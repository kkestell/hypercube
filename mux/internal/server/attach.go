package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/kkestell/hypercube/mux/client"
	"go.mitchellh.com/libghostty"
)

// attachment is a client viewing a session. Its draw goroutine is the only
// writer to the connection while the stream runs. The server's mutex guards
// session, cols, rows, drag, and clipboard.
type attachment struct {
	session    *session
	cols, rows int // the client's terminal size
	conn       net.Conn
	wake       chan struct{} // buffer of one; a signal means "redraw"
	prefix     bool          // the last key was the prefix
	drag       *drag         // the mouse button held down, if any
	clipboard  []byte        // text for the next frame to copy to the client's clipboard
}

// window is the window the client sees.
func (a *attachment) window() *window { return a.session.active }

// focus is the pane that receives input.
func (a *attachment) focus() *pane { return a.window().active }

// wake signals the attachments whose shown window contains p. It must be called
// with the mutex held.
func (s *Server) wake(p *pane) {
	for a := range s.attached {
		if slices.Contains(a.window().panes, p) {
			a.signal()
		}
	}
}

// signalAll redraws every attachment. A label change in one session shows in
// every client's session bar. It must be called with the mutex held.
func (s *Server) signalAll() {
	for a := range s.attached {
		a.signal()
	}
}

func (a *attachment) signal() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

// layoutSession sizes the panes of every window in sess to the client attached
// to it, or to the default size if none is, and redraws the client. Panes that
// do not fit keep their size. It must be called with the mutex held.
func (s *Server) layoutSession(sess *session) {
	var shown *attachment
	for a := range s.attached {
		if a.session == sess {
			shown = a
			break
		}
	}
	// The layout never gives a zero size, and a PTY master accepts a size after
	// its process has exited, so a failure here is a bug.
	resize := func(p *pane, cols, rows int) {
		if err := p.resize(uint16(cols), uint16(rows)); err != nil {
			panic(err)
		}
	}
	for _, w := range sess.windows {
		if shown == nil {
			for _, p := range w.panes {
				resize(p, defaultCols, defaultRows)
			}
			continue
		}
		for i, r := range layout(w.panes, shown.cols, shown.rows) {
			if r.rows > 0 { // a collapsed pane keeps its size
				resize(w.panes[i], r.cols, r.rows)
			}
		}
	}
	if shown != nil {
		shown.signal()
	}
}

// attach runs a client's stream until it detaches or the connection ends.
func (s *Server) attach(conn net.Conn, dec *json.Decoder, enc *json.Encoder, req client.Request) {
	s.mu.Lock()
	sess := s.findSession(req.Session)
	if sess == nil {
		s.mu.Unlock()
		enc.Encode(client.Reply{Error: fmt.Sprintf("unknown session %q", req.Session)})
		return
	}
	a := &attachment{session: sess, cols: req.Cols, rows: req.Rows, conn: conn, wake: make(chan struct{}, 1)}
	s.attached[a] = struct{}{}
	s.show(a, sess)
	s.mu.Unlock()
	enc.Encode(client.Reply{OK: true})

	done := make(chan struct{})
	var drawing sync.WaitGroup
	drawing.Add(1)
	go func() {
		defer drawing.Done()
		s.draw(a, enc, done)
	}()
	a.signal()

	detached := s.stream(dec, a)

	close(done)
	drawing.Wait()
	s.mu.Lock()
	delete(s.attached, a)
	a.endDrag()
	// A kill that ended the session has already removed it.
	if slices.Contains(s.sessions, a.session) {
		s.layoutSession(a.session)
	}
	s.mu.Unlock()
	if detached {
		enc.Encode(client.Output{Detached: true})
	}
}

// show makes sess the session the client sees. A session has one client, so
// closing the earlier client's connection ends its stream, the same way kill
// ends an attachment. The session the client left goes back to the default
// size. It must be called with the mutex held.
func (s *Server) show(a *attachment, sess *session) {
	for old := range s.attached {
		if old != a && old.session == sess {
			delete(s.attached, old)
			old.conn.Close()
		}
	}
	left := a.session
	a.session = sess
	a.endDrag()
	if left != sess {
		s.layoutSession(left)
	}
	s.layoutSession(sess)
}

// stream handles input and resize requests. It reports whether the client
// asked to detach, as opposed to the connection ending.
func (s *Server) stream(dec *json.Decoder, a *attachment) bool {
	for {
		var req client.Request
		if err := dec.Decode(&req); err != nil {
			return false
		}
		switch req.Type {
		case "input":
			// A command may move focus, so the rest of the input goes to the
			// pane focused after it.
			for data := req.Data; len(data) > 0; {
				var pass []byte
				var cmd rune
				var mouse *mouseEvent
				pass, cmd, mouse, data = a.filter(data)
				s.mu.Lock()
				// A kill that ended the session closed the connection.
				_, ok := s.attached[a]
				var ptmx *os.File
				if ok {
					p := a.focus()
					ptmx = p.ptmx
					// Typing returns a scrolled pane to the bottom, so the
					// prompt shows.
					if len(pass) > 0 {
						active, err := p.term.ViewportActive()
						if err != nil {
							panic(err) // The terminal is broken; fail loudly.
						}
						if !active {
							p.term.ScrollViewportBottom()
							a.signal()
						}
					}
				}
				s.mu.Unlock()
				if !ok {
					return false
				}
				ptmx.Write(pass)
				if mouse != nil {
					s.mouse(a, *mouse)
				}
				if cmd != 0 && s.command(a, cmd) {
					return true
				}
			}
		case "resize":
			s.mu.Lock()
			// A kill that ended the session closed the connection.
			if _, ok := s.attached[a]; ok {
				a.cols, a.rows = req.Cols, req.Rows
				s.layoutSession(a.session)
			}
			s.mu.Unlock()
		}
	}
}

// command runs the built-in key after the prefix. It reports whether the client
// asked to detach. Switching windows needs no layout, because every window of
// the session is already sized.
func (s *Server) command(a *attachment, cmd rune) (detach bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// A kill that ended the session closed the connection.
	if _, ok := s.attached[a]; !ok {
		return false
	}
	sess, w := a.session, a.window()
	switch {
	case cmd == 'd':
		return true
	case cmd == 'n':
		sess.active = sess.windows[(slices.Index(sess.windows, w)+1)%len(sess.windows)]
	case cmd == 'p':
		sess.active = sess.windows[(slices.Index(sess.windows, w)+len(sess.windows)-1)%len(sess.windows)]
	case cmd >= '0' && cmd <= '9':
		if i := int(cmd - '0'); i < len(sess.windows) {
			sess.active = sess.windows[i]
		}
	case cmd == 'o':
		// The next pane may not fit the terminal, so it is not drawn and the
		// client types into a pane it cannot see; that is unlikely.
		i := slices.Index(w.panes, w.active)
		for {
			i = (i + 1) % len(w.panes)
			if !w.panes[i].collapsed {
				break
			}
		}
		w.active = w.panes[i]
	}
	a.signal()
	return false
}

// draw sends a full frame each time the attachment is woken. Output that
// arrives while a frame is being written folds into the next frame.
func (s *Server) draw(a *attachment, enc *json.Encoder, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case <-a.wake:
		}
		s.mu.Lock()
		if _, ok := s.attached[a]; !ok {
			s.mu.Unlock()
			return
		}
		frame, err := drawFrame(a, s.sessions)
		if err != nil {
			panic(err) // The render state is broken; fail loudly.
		}
		if a.clipboard != nil {
			frame = append(fmt.Appendf(nil, "\x1b]52;c;%s\x07", base64.StdEncoding.EncodeToString(a.clipboard)), frame...)
			a.clipboard = nil
		}
		s.mu.Unlock()
		if enc.Encode(client.Output{Frame: frame}) != nil {
			return
		}
	}
}

// drawFrame renders the bars, the shown window, and each pane's title row as
// one synchronized update. Clearing the screen first blanks any space nothing
// uses, and the focused pane's keyboard modes follow it so the cursor stays
// last. It must be called with the server's mutex held.
func drawFrame(a *attachment, sessions []*session) ([]byte, error) {
	rows, err := libghostty.NewRenderStateRowIterator()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cells, err := libghostty.NewRenderStateRowCells()
	if err != nil {
		return nil, err
	}
	defer cells.Close()

	panes := a.window().panes
	rects := layout(panes, a.cols, a.rows)
	b := []byte("\x1b[?2026h\x1b[0m\x1b[H\x1b[2J")
	if b, err = appendKeyboardModes(b, a.focus()); err != nil {
		return nil, err
	}

	// The chrome comes before the cells, and each item ends with a reset, so
	// the cell loop's SGR tracking starts from a known state.
	b = appendBar(b, 1, 1, a.cols, windowLabels(a.session), slices.Index(a.session.windows, a.window()))
	b = appendBar(b, a.rows, 1, a.cols, sessionLabels(sessions), slices.Index(sessions, a.session))
	for i, r := range rects {
		label := panes[i].label()
		if !panes[i].alive {
			label = fmt.Sprintf("%s (exited %d)", label, panes[i].status)
		}
		b = appendTitle(b, r.y, r.x+1, r.cols, label, panes[i] == a.focus())
	}
	if len(rects) > 1 {
		// The divider joins the title lines: ┬ where the main pane's and the
		// first stacked pane's titles meet, ├ at each other stacked title.
		for y := topRows; y < a.rows-bottomRows; y++ {
			c := "│"
			if y == topRows {
				c = "┬"
			} else if slices.ContainsFunc(rects[2:], func(r rect) bool { return r.y-1 == y }) {
				c = "├"
			}
			b = append(b, fmt.Sprintf("\x1b[%d;%dH%s", y+1, rects[0].cols+1, c)...)
		}
	}
	var sgr, prev []byte
	var graphemes []uint32
	var cursor *libghostty.RenderStateCursor
	var cursorRect rect
	for i, r := range rects {
		if r.rows == 0 {
			continue // collapsed
		}
		rs := panes[i].rs
		if err := rs.Update(panes[i].term); err != nil {
			return nil, err
		}
		if err := rs.RowIterator(rows); err != nil {
			return nil, err
		}
		for y := r.y + 1; rows.Next(); y++ {
			if err := rows.Cells(cells); err != nil {
				return nil, err
			}
			b = append(b, fmt.Sprintf("\x1b[%d;%dH", y, r.x+1)...)
			for cells.Next() {
				raw, err := cells.Raw()
				if err != nil {
					return nil, err
				}
				wide, err := raw.Wide()
				if err != nil {
					return nil, err
				}
				if wide == libghostty.CellWideSpacerTail {
					continue
				}
				style, err := cells.Style()
				if err != nil {
					return nil, err
				}
				selected, err := cells.Selected()
				if err != nil {
					return nil, err
				}
				if sgr = appendSGR(sgr[:0], style, selected); !bytes.Equal(sgr, prev) {
					b = append(b, sgr...)
					prev = append(prev[:0], sgr...)
				}
				if graphemes, err = cells.GraphemesInto(graphemes[:0]); err != nil {
					return nil, err
				}
				if len(graphemes) == 0 {
					b = append(b, ' ')
				}
				for _, g := range graphemes {
					b = utf8.AppendRune(b, rune(g))
				}
			}
		}
		if panes[i] == a.focus() {
			cursorRect = r
			if cursor, err = rs.Cursor(); err != nil {
				return nil, err
			}
		}
	}

	b = append(b, "\x1b[0m"...)
	if cursor != nil && cursor.Visible && cursor.ViewportHasValue {
		b = append(b, fmt.Sprintf("\x1b[%d;%dH\x1b[?25h", cursorRect.y+int(cursor.ViewportY)+1, cursorRect.x+int(cursor.ViewportX)+1)...)
	} else {
		b = append(b, "\x1b[?25l"...)
	}
	return append(b, "\x1b[?2026l"...), nil
}

// appendBar moves to row and col (both 1-based) and writes each item as " text
// ", the current one in inverse video, cut off at width columns. Pass current -1
// for none. Width is counted in runes, so a title with wide or combining
// characters may draw a little off; that is unlikely.
func appendBar(b []byte, row, col, width int, items []string, current int) []byte {
	b = fmt.Appendf(b, "\x1b[%d;%dH", row, col)
	for i, item := range items {
		if width <= 0 {
			break
		}
		text := []rune(" " + item + " ")
		text = text[:min(len(text), width)]
		width -= len(text)
		if i == current {
			b = append(b, "\x1b[7m"...)
		}
		b = append(b, string(text)...)
		if i == current {
			b = append(b, "\x1b[0m"...)
		}
	}
	return b
}

// appendTitle moves to row and col (both 1-based) and writes a pane's title
// row as a line, "─ label ───", filled out to width columns. The label is in
// inverse video when the pane is focused. Width is counted in runes, as in
// appendBar.
func appendTitle(b []byte, row, col, width int, label string, focused bool) []byte {
	b = fmt.Appendf(b, "\x1b[%d;%dH", row, col)
	text := []rune(" " + label + " ")
	text = text[:min(len(text), max(width-1, 0))]
	if width > 0 {
		b = append(b, "─"...)
	}
	if focused {
		b = append(b, "\x1b[7m"...)
	}
	b = append(b, string(text)...)
	if focused {
		b = append(b, "\x1b[0m"...)
	}
	return append(b, strings.Repeat("─", max(width-1-len(text), 0))...)
}

// appendSGR resets, then sets the cell's attributes; a selected cell has
// inverse video flipped. Palette colors stay
// palette colors so the outer terminal's theme shows, and SGR 39 and 49 leave
// its default colors in place. A palette the program changes with OSC 4 is not
// shown.
func appendSGR(b []byte, s *libghostty.Style, selected bool) []byte {
	b = append(b, "\x1b[0"...)
	for _, a := range []struct {
		on   bool
		code string
	}{
		{s.Bold(), ";1"}, {s.Faint(), ";2"}, {s.Italic(), ";3"},
		{s.Underline() != libghostty.UnderlineNone, ";4"}, {s.Inverse() != selected, ";7"}, {s.Strikethrough(), ";9"},
	} {
		if a.on {
			b = append(b, a.code...)
		}
	}
	b = appendColor(b, s.FgColor(), 30, 90, 38)
	b = appendColor(b, s.BgColor(), 40, 100, 48)
	return append(b, 'm')
}

// appendColor appends one color: base+n for the first 8 palette entries,
// bright+n for the next 8, extended;5;n for the rest, and extended;2;r;g;b for
// RGB. No color is the default, base+9.
func appendColor(b []byte, c libghostty.StyleColor, base, bright, extended int) []byte {
	switch {
	case c.Tag == libghostty.StyleColorRGB:
		return fmt.Appendf(b, ";%d;2;%d;%d;%d", extended, c.RGB.R, c.RGB.G, c.RGB.B)
	case c.Tag == libghostty.StyleColorPalette && c.Palette < 8:
		return fmt.Appendf(b, ";%d", base+int(c.Palette))
	case c.Tag == libghostty.StyleColorPalette && c.Palette < 16:
		return fmt.Appendf(b, ";%d", bright+int(c.Palette)-8)
	case c.Tag == libghostty.StyleColorPalette:
		return fmt.Appendf(b, ";%d;5;%d", extended, c.Palette)
	}
	return fmt.Appendf(b, ";%d", base+9)
}
