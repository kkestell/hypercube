package server

import "fmt"

// label is the title the pane's program set with OSC 0 or OSC 2, or else the
// pane's command name. It must be called with the server's mutex held.
func (p *pane) label() string {
	title, err := p.term.Title()
	if err != nil {
		panic(err) // The terminal is broken; fail loudly.
	}
	if title == "" {
		return p.name
	}
	return title
}

// label is the first pane's label. A window is never empty: kill removes it
// with its last pane.
func (w *window) label() string { return w.panes[0].label() }

// label is the first window's label. A session is never empty: kill removes it
// with its last window.
func (sess *session) label() string { return sess.windows[0].label() }

// sessionLabels is the session bar's items. It must be called with the server's
// mutex held.
func sessionLabels(sessions []*session) []string {
	var labels []string
	for _, sess := range sessions {
		labels = append(labels, sess.label())
	}
	return labels
}

// windowLabels is the window bar's items. It must be called with the server's
// mutex held.
func windowLabels(sess *session) []string {
	var labels []string
	for i, w := range sess.windows {
		labels = append(labels, fmt.Sprintf("%d %s", i, w.label()))
	}
	return labels
}
