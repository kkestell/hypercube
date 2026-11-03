package session

// LineRange highlights inclusive, one-based source lines.
type LineRange struct {
	Start int `json:"start"`
	End   int `json:"end,omitempty"`
}

// FileOpen is a request to open a file relative to a session's directory.
type FileOpen struct {
	Path  string      `json:"path"`
	Lines []LineRange `json:"lines"`
}

// OpenFile queues a request until a page views the session.
func (r *Runtime) OpenFile(id string, open FileOpen) {
	r.mu.Lock()
	r.opens[id] = append(r.opens[id], open)
	r.mu.Unlock()
	r.feed.publish(Change{})
}

// TakeFileOpens removes and returns the session's waiting requests.
func (r *Runtime) TakeFileOpens(id string) []FileOpen {
	r.mu.Lock()
	defer r.mu.Unlock()
	opens := r.opens[id]
	delete(r.opens, id)
	return opens
}
