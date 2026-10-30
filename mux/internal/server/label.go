package server

import (
	"fmt"
	"strings"

	"go.mitchellh.com/libghostty"
)

// The icons are Nerd Font Material Design symbols, which take one cell. They
// show only if the attached terminal's font has them.
const (
	bellIcon  = "\U000F009A" // nf-md-bell
	waitIcon  = "\U000F051F" // nf-md-timer_sand
	errorIcon = "\U000F0028" // nf-md-alert_circle
	pauseIcon = "\U000F03E5" // nf-md-pause_circle
	sliceIcon = 0xF0A9E      // nf-md-circle_slice_1; slices 2 to 8 follow
)

// bell marks a label whose session, window, or pane needs attention.
func bell(on bool) string {
	if on {
		return bellIcon + " "
	}
	return ""
}

// progressIcon shows the progress a pane's program last reported with OSC 9;4:
// a circle filled in eighths and the percentage, or an icon for a report
// without one.
func progressIcon(r libghostty.TerminalProgressReport) string {
	switch {
	case r.State == libghostty.TerminalProgressStateSet && r.Progress >= 0:
		eighths := min(max((int(r.Progress)*8+99)/100, 1), 8)
		return fmt.Sprintf("%c %d%% ", sliceIcon+eighths-1, r.Progress)
	case r.State == libghostty.TerminalProgressStateSet, r.State == libghostty.TerminalProgressStateIndeterminate:
		return waitIcon + " "
	case r.State == libghostty.TerminalProgressStateError:
		return errorIcon + " "
	case r.State == libghostty.TerminalProgressStatePause:
		return pauseIcon + " "
	}
	return ""
}

// fit returns the longest prefix of s that fits in width terminal columns, and
// the columns it takes. Widths are per code point, so a grapheme joined from
// several emoji may draw a little off; that is unlikely.
func fit(s string, width int) (string, int) {
	used := 0
	for i, r := range s {
		w := int(libghostty.UnicodeCodepointWidth(uint32(r)))
		if used+w > width {
			return s[:i], used
		}
		used += w
	}
	return s, used
}

// text is the label a client set, and whether it set one.
func (m marks) text() (string, bool) {
	if m.spans == nil {
		return "", false
	}
	var b strings.Builder
	for _, span := range m.spans {
		b.WriteString(span.Text)
	}
	return b.String(), true
}

// label is the label a client set, or else the title the pane's program set
// with OSC 0 or OSC 2, or else the pane's command name. It must be called with
// the server's mutex held.
func (p *pane) label() string {
	if text, ok := p.text(); ok {
		return text
	}
	title, err := p.term.Title()
	if err != nil {
		panic(err) // The terminal is broken; fail loudly.
	}
	if title == "" {
		return p.name
	}
	return title
}

// label is the label a client set, or else the first pane's label. A window is
// never empty: kill removes it with its last pane.
func (w *window) label() string {
	if text, ok := w.text(); ok {
		return text
	}
	return w.panes[0].label()
}

// label is the label a client set, or else the first window's label. A session
// is never empty: kill removes it with its last window.
func (sess *session) label() string {
	if text, ok := sess.text(); ok {
		return text
	}
	return sess.windows[0].label()
}

// sessionsAt is the 0-based column where the status bar's sessions start, so
// that they end at the right edge. When the bar is too narrow for both, the
// sessions cover the windows; that is unlikely.
func sessionsAt(cols int, labels []string) int {
	width := 0
	for _, label := range labels {
		_, w := fit(" "+label+" ", cols)
		width += w
	}
	return max(cols-width, 0)
}

// sessionLabels is the status bar's sessions, on the right. It must be called with the server's
// mutex held.
func sessionLabels(sessions []*session) []string {
	var labels []string
	for _, sess := range sessions {
		labels = append(labels, bell(sess.attention)+sess.label())
	}
	return labels
}

// windowLabels is the status bar's windows, on the left. It must be called with the server's
// mutex held.
func windowLabels(sess *session) []string {
	var labels []string
	for i, w := range sess.windows {
		labels = append(labels, fmt.Sprintf("%d %s%s", i, bell(w.attention), w.label()))
	}
	return labels
}
