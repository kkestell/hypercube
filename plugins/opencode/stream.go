package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
)

func (ag *agent) subscribe(ctx context.Context) error {
	ready := make(chan error, 1)
	ag.streamDone = make(chan struct{})
	go func() {
		defer close(ag.streamDone)
		err := ag.stream(func() { ready <- nil })
		select {
		case ready <- err:
		default:
		}
		if ag.ctx.Err() == nil {
			ag.mu.Lock()
			ag.emit(harness.Event{Items: []harness.Item{ag.errors.Item(fmt.Sprintf("OpenCode event stream: %v", err))}})
			ag.mu.Unlock()
			ag.cancel()
			ag.server.stop()
		}
	}()
	select {
	case err := <-ready:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (ag *agent) stream(ready func()) error {
	res, err := ag.server.request(ag.ctx, "GET", "/api/event", nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	first := true
	return readEvents(res.Body, func(e nativeEvent) error {
		if first {
			if e.Type != "server.connected" {
				return errors.New("missing server.connected marker")
			}
			first = false
			ready()
			return nil
		}
		return ag.handle(e)
	})
}

// readEvents handles SSE frames, including comments and multiline data. A
// disconnected stream fails the plugin; continuing would hide missing output.
func readEvents(r io.Reader, handle func(nativeEvent) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 64<<20)
	var data []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if len(data) == 0 {
				continue
			}
			var e nativeEvent
			if err := json.Unmarshal([]byte(strings.Join(data, "\n")), &e); err != nil {
				return err
			}
			if err := handle(e); err != nil {
				return err
			}
			data = nil
		} else if value, ok := strings.CutPrefix(line, "data:"); ok {
			data = append(data, strings.TrimPrefix(value, " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}
