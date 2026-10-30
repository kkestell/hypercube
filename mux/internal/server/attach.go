package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"sync"
	"unicode/utf8"

	"github.com/kkestell/hypercube/mux/client"
	"go.mitchellh.com/libghostty"
)

const prefixKey = 0x02 // Ctrl-b

// attachment is a client viewing a session's pane. Its draw goroutine is the
// only writer to the connection while the stream runs.
type attachment struct {
	pane   *pane
	conn   net.Conn
	wake   chan struct{} // buffer of one; a signal means "redraw"
	prefix bool          // the last input byte was the prefix key
}

// wake must be called with the mutex held.
func (s *Server) wake(p *pane) {
	for a := range s.attached {
		if a.pane == p {
			a.signal()
		}
	}
}

func (a *attachment) signal() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

// attach runs a client's stream until it detaches or the connection ends.
func (s *Server) attach(conn net.Conn, dec *json.Decoder, enc *json.Encoder, req client.Request) {
	s.mu.Lock()
	var p *pane
	for _, sess := range s.sessions {
		if sess.id == req.Session {
			p = sess.windows[0].panes[0]
		}
	}
	if p == nil {
		s.mu.Unlock()
		enc.Encode(client.Reply{Error: fmt.Sprintf("unknown session %q", req.Session)})
		return
	}
	err := p.resize(uint16(req.Cols), uint16(req.Rows))
	a := &attachment{pane: p, conn: conn, wake: make(chan struct{}, 1)}
	if err == nil {
		s.attached[a] = struct{}{}
	}
	s.mu.Unlock()
	if err != nil {
		enc.Encode(client.Reply{Error: err.Error()})
		return
	}
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
	s.mu.Unlock()
	if detached {
		enc.Encode(client.Output{Detached: true})
	}
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
			pass, detach := a.filter(req.Data)
			s.mu.Lock()
			ptmx := a.pane.ptmx
			s.mu.Unlock()
			ptmx.Write(pass)
			if detach {
				return true
			}
		case "resize":
			s.mu.Lock()
			// A killed pane's terminal is closed.
			if _, ok := s.attached[a]; ok {
				a.pane.resize(uint16(req.Cols), uint16(req.Rows))
			}
			s.mu.Unlock()
			a.signal()
		}
	}
}

// filter takes the prefix key out of input, one byte at a time across reads. It
// returns the bytes for the pane and whether the client asked to detach.
func (a *attachment) filter(data []byte) (pass []byte, detach bool) {
	for _, b := range data {
		switch {
		case a.prefix:
			a.prefix = false
			// Only d is bound in task 1; any other key is dropped.
			if b == 'd' {
				return pass, true
			}
		case b == prefixKey:
			a.prefix = true
		default:
			pass = append(pass, b)
		}
	}
	return pass, false
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
		frame, err := drawFrame(a.pane.term, a.pane.rs)
		s.mu.Unlock()
		if err != nil {
			panic(err) // The render state is broken; fail loudly.
		}
		if enc.Encode(client.Output{Frame: frame}) != nil {
			return
		}
	}
}

// drawFrame renders the terminal's screen as one synchronized update. It must
// be called with the server's mutex held.
func drawFrame(term *libghostty.Terminal, rs *libghostty.RenderState) ([]byte, error) {
	if err := rs.Update(term); err != nil {
		return nil, err
	}
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
	if err := rs.RowIterator(rows); err != nil {
		return nil, err
	}

	b := []byte("\x1b[?2026h")
	var sgr, prev []byte
	var graphemes []uint32
	for y := 1; rows.Next(); y++ {
		if err := rows.Cells(cells); err != nil {
			return nil, err
		}
		b = append(b, "\x1b["+strconv.Itoa(y)+";1H"...)
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
			if sgr = appendSGR(sgr[:0], style); !bytes.Equal(sgr, prev) {
				b = append(b, sgr...)
				prev = append(prev[:0], sgr...)
			}
			if graphemes, err = cells.GraphemesInto(graphemes[:0]); err != nil {
				return nil, err
			}
			if len(graphemes) == 0 {
				b = append(b, ' ')
			}
			for _, r := range graphemes {
				b = utf8.AppendRune(b, rune(r))
			}
		}
	}

	cursor, err := rs.Cursor()
	if err != nil {
		return nil, err
	}
	b = append(b, "\x1b[0m"...)
	if cursor.Visible && cursor.ViewportHasValue {
		b = append(b, fmt.Sprintf("\x1b[%d;%dH\x1b[?25h", cursor.ViewportY+1, cursor.ViewportX+1)...)
	} else {
		b = append(b, "\x1b[?25l"...)
	}
	return append(b, "\x1b[?2026l"...), nil
}

// appendSGR resets, then sets the cell's attributes. Palette colors stay
// palette colors so the outer terminal's theme shows, and SGR 39 and 49 leave
// its default colors in place. A palette the program changes with OSC 4 is not
// shown.
func appendSGR(b []byte, s *libghostty.Style) []byte {
	b = append(b, "\x1b[0"...)
	for _, a := range []struct {
		on   bool
		code string
	}{
		{s.Bold(), ";1"}, {s.Faint(), ";2"}, {s.Italic(), ";3"},
		{s.Underline() != libghostty.UnderlineNone, ";4"}, {s.Inverse(), ";7"}, {s.Strikethrough(), ";9"},
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
