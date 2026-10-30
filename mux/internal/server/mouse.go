package server

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.mitchellh.com/libghostty"
)

const (
	wheelStep = 3 // lines per wheel click

	mouseShift  = 4
	mouseAlt    = 8
	mouseCtrl   = 16
	mouseMotion = 32
)

// mouseEvent is one SGR mouse report from the outer terminal.
type mouseEvent struct {
	button libghostty.MouseButton // left, middle, right, four (wheel up), five (wheel down), and so on
	mods   libghostty.Mods
	action libghostty.MouseAction // press, release, or motion
	x, y   int                    // 0-based terminal cell
}

// parseMouse decodes an SGR mouse report, CSI < b ; x ; y M or m, and reports
// whether raw is one.
func parseMouse(raw []byte) (mouseEvent, bool) {
	body, ok := strings.CutPrefix(string(raw), "\x1b[<")
	if !ok || len(body) == 0 || (body[len(body)-1] != 'M' && body[len(body)-1] != 'm') {
		return mouseEvent{}, false
	}
	fields := strings.Split(body[:len(body)-1], ";")
	if len(fields) != 3 {
		return mouseEvent{}, false
	}
	var n [3]int
	for i, f := range fields {
		var err error
		if n[i], err = strconv.Atoi(f); err != nil {
			return mouseEvent{}, false
		}
	}
	b := n[0]

	ev := mouseEvent{x: n[1] - 1, y: n[2] - 1}
	switch {
	case b&64 != 0: // wheel: buttons four to seven
		ev.button = libghostty.MouseButtonFour + libghostty.MouseButton(b&3)
	case b&128 != 0: // buttons eight to eleven
		ev.button = libghostty.MouseButtonEight + libghostty.MouseButton(b&3)
	default:
		// The outer terminal reports motion only while a button is held, so b&3
		// is never 3, which means no button.
		ev.button = [...]libghostty.MouseButton{libghostty.MouseButtonLeft, libghostty.MouseButtonMiddle, libghostty.MouseButtonRight, libghostty.MouseButtonUnknown}[b&3]
	}
	if b&mouseShift != 0 {
		ev.mods |= libghostty.ModShift
	}
	if b&mouseAlt != 0 {
		ev.mods |= libghostty.ModAlt
	}
	if b&mouseCtrl != 0 {
		ev.mods |= libghostty.ModCtrl
	}
	switch {
	case b&mouseMotion != 0:
		ev.action = libghostty.MouseActionMotion
	case body[len(body)-1] == 'm':
		ev.action = libghostty.MouseActionRelease
	default:
		ev.action = libghostty.MouseActionPress
	}
	return ev, true
}

func (ev mouseEvent) isWheel() bool {
	return ev.button >= libghostty.MouseButtonFour && ev.button <= libghostty.MouseButtonSeven
}

// isDragButton reports whether the button can start a drag.
func (ev mouseEvent) isDragButton() bool {
	return ev.button == libghostty.MouseButtonLeft || ev.button == libghostty.MouseButtonMiddle ||
		ev.button == libghostty.MouseButtonRight
}

// drag is a button held down in a pane. Motion and the release go to the
// pane, with positions clamped to its screen.
type drag struct {
	pane   *pane
	anchor *libghostty.TrackedGridRef // the selection's start; nil when the pane gets the mouse
}

// endDrag drops the drag. It must be called with the server's mutex held. A
// selection left on the pane stays highlighted until the pane's next press;
// that happens only when the pane leaves the shown window mid-drag, which is
// unlikely.
func (a *attachment) endDrag() {
	if a.drag != nil && a.drag.anchor != nil {
		a.drag.anchor.Close()
	}
	a.drag = nil
}

// mouse routes a mouse report from the client's terminal.
func (s *Server) mouse(a *attachment, ev mouseEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// A kill that ended the session closed the connection.
	if _, ok := s.attached[a]; !ok {
		return
	}
	panes := a.window().panes
	rects := layout(panes, a.cols, a.rows)

	if a.drag != nil {
		// The pane may have been killed, respawned, hidden by another window, or
		// left without room.
		if i := slices.Index(panes, a.drag.pane); i < 0 || i >= len(rects) {
			a.endDrag()
		}
	}
	if ev.action != libghostty.MouseActionPress {
		if a.drag != nil {
			s.toPane(a, a.drag.pane, rects[slices.Index(panes, a.drag.pane)], ev)
			if ev.action == libghostty.MouseActionRelease {
				a.endDrag()
			}
		}
		return
	}

	left := ev.button == libghostty.MouseButtonLeft
	switch {
	case ev.y == 0:
		if i := barItem(windowLabels(a.session), a.cols, ev.x); left && i >= 0 {
			a.session.active = a.session.windows[i]
			a.signal()
		}
	case ev.y == a.rows-1:
		if i := barItem(sessionLabels(s.sessions), a.cols, ev.x); left && i >= 0 {
			s.show(a, s.sessions[i])
		}
	default:
		for i, r := range rects {
			if ev.x < r.x || ev.x >= r.x+r.cols {
				continue
			}
			switch {
			case ev.y == r.y-titleRows && left && i == 0:
				a.window().active = panes[i]
				a.signal()
			case ev.y == r.y-titleRows && left:
				// A stacked pane's title toggles it. An opened pane takes the
				// focus, and a collapsed one gives it to the main pane.
				p := panes[i]
				p.collapsed = !p.collapsed
				if !p.collapsed {
					a.window().active = p
				} else if a.window().active == p {
					a.window().active = panes[0]
				}
				s.layoutSession(a.session)
			case ev.y >= r.y && ev.y < r.y+r.rows:
				if ev.isWheel() {
					s.toPane(a, panes[i], r, ev)
				} else if ev.isDragButton() {
					a.window().active = panes[i]
					a.endDrag()
					a.drag = &drag{pane: panes[i]}
					s.toPane(a, panes[i], r, ev)
					a.signal()
				}
			}
		}
	}
}

// toPane delivers the event to the pane at rect r: encoded for the pane if it
// asked for the mouse, otherwise as scrollback and selection. It must be called
// with the server's mutex held.
func (s *Server) toPane(a *attachment, p *pane, r rect, ev mouseEvent) {
	tracking, err := p.term.MouseTracking()
	if err != nil {
		panic(err) // The terminal is broken; fail loudly.
	}
	x := min(max(ev.x-r.x, 0), r.cols-1)
	y := min(max(ev.y-r.y, 0), r.rows-1)
	if tracking {
		out, err := encodeMouse(p, r, ev, x, y, a.drag != nil)
		if err != nil {
			panic(err)
		}
		// Like the terminal's write callback, this writes with the mutex held, so
		// a pane that stops reading input could block the server once its PTY
		// input buffer fills; that is unlikely.
		p.ptmx.Write(out)
		return
	}

	switch {
	case ev.isWheel():
		// A pane on the alternate screen has no scrollback, so the wheel does
		// nothing there.
		switch ev.button {
		case libghostty.MouseButtonFour:
			p.term.ScrollViewportDelta(-wheelStep)
		case libghostty.MouseButtonFive:
			p.term.ScrollViewportDelta(wheelStep)
		}
		a.signal()
	case ev.action == libghostty.MouseActionPress && ev.button == libghostty.MouseButtonLeft:
		p.term.SetSelection(nil)
		anchor, err := p.term.TrackGridRef(libghostty.Point{Tag: libghostty.PointTagViewport, X: uint16(x), Y: uint32(y)})
		if err != nil {
			panic(err)
		}
		a.drag.anchor = anchor
	case a.drag.anchor == nil: // a middle or right button does nothing
	case ev.action == libghostty.MouseActionMotion:
		// Dragging past the top or bottom does not scroll; the wheel scrolls
		// during a drag, and the next motion extends the selection.
		start, err := a.drag.anchor.Snapshot()
		if err != nil {
			return // scrollback pruning discarded the anchor's cell; that is unlikely
		}
		end, err := p.term.GridRef(libghostty.Point{Tag: libghostty.PointTagViewport, X: uint16(x), Y: uint32(y)})
		if err != nil {
			panic(err)
		}
		p.term.SetSelection(&libghostty.Selection{Start: *start, End: *end})
		a.signal()
	case ev.action == libghostty.MouseActionRelease:
		sel, err := p.term.Selection()
		if err != nil {
			panic(err)
		}
		if sel != nil {
			text, err := p.term.SelectionFormatString()
			if err != nil {
				panic(err)
			}
			if text != "" {
				a.clipboard = []byte(text)
			}
			p.term.SetSelection(nil)
			a.signal()
		}
	}
}

// encodeMouse encodes the event at cell (x, y) of the pane in the mode and
// format the pane asked for.
func encodeMouse(p *pane, r rect, ev mouseEvent, x, y int, dragging bool) ([]byte, error) {
	enc, err := libghostty.NewMouseEncoder()
	if err != nil {
		return nil, err
	}
	defer enc.Close()
	enc.SetOptFromTerminal(p.term)
	enc.SetOptSize(libghostty.MouseEncoderSize{ScreenWidth: uint32(r.cols), ScreenHeight: uint32(r.rows), CellWidth: 1, CellHeight: 1})
	enc.SetOptAnyButtonPressed(dragging)
	me, err := libghostty.NewMouseEvent()
	if err != nil {
		return nil, err
	}
	defer me.Close()
	me.SetAction(ev.action)
	me.SetButton(ev.button)
	me.SetMods(ev.mods)
	me.SetPosition(libghostty.MousePosition{X: float32(x) + 0.5, Y: float32(y) + 0.5})
	return enc.Encode(me)
}

// barItem returns the item at column x of a bar width columns wide, using the
// widths appendBar draws, or -1.
func barItem(items []string, width, x int) int {
	start := 0
	for i, item := range items {
		w := min(utf8.RuneCountInString(item)+2, width-start)
		if w <= 0 {
			break
		}
		if x < start+w {
			return i
		}
		start += w
	}
	return -1
}
