package hold

import (
	"errors"
	"io"
	"os"
	"sync"
	"time"
)

// Output follows the retained segments from a byte position.
type Output struct {
	h        *Holder
	position int64
	closed   chan struct{}
	once     sync.Once
}

func (h *Holder) Output(from int64) (*Output, error) {
	if _, err := os.Stat(h.dir); err != nil {
		return nil, err
	}
	return &Output{h: h, position: from, closed: make(chan struct{})}, nil
}
func (o *Output) Close() error { o.once.Do(func() { close(o.closed) }); return nil }
func (o *Output) Read(p []byte) (int, error) {
	var ended error
	for {
		select {
		case <-o.closed:
			return 0, ErrClosed
		default:
		}
		positions, err := o.h.segments()
		if err != nil {
			return 0, err
		}
		if len(positions) > 0 && o.position < positions[0] {
			o.position = positions[0]
		}
		var start int64 = -1
		for _, pos := range positions {
			if pos <= o.position {
				start = pos
			}
		}
		if start >= 0 {
			f, err := os.Open(o.h.segment(start))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return 0, err
			}
			n, err := f.ReadAt(p, o.position-start)
			f.Close()
			o.position += int64(n)
			if n > 0 {
				return n, nil
			}
			if err != io.EOF {
				return 0, err
			}
		}
		if ended != nil {
			return 0, ended
		}
		// The holder writes the exit record after all output and before it
		// releases the lock, so a dead holder without one was lost. Re-read
		// after either so output appended before the end cannot be missed.
		alive := o.h.Alive()
		if _, err := o.h.Exit(); err == nil {
			ended = io.EOF
			continue
		}
		if !alive {
			ended = ErrLost
			continue
		}
		select {
		case <-o.closed:
			return 0, ErrClosed
		case <-time.After(25 * time.Millisecond):
		}
	}
}
