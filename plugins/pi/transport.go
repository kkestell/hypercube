package main

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"strconv"
)

// statsID identifies the usage refresh sent without waiting on the output reader.
const statsID = "session-stats"

// response is Pi's answer to a command.
type response struct {
	err  string
	data json.RawMessage
}

// end fails the calls waiting on pi, and later ones, with the reason its
// output ended.
func (ag *agent) end(err error) {
	ag.exit = err
	close(ag.ended)
}

// write sends one record to pi.
func (ag *agent) write(v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	ag.writing.Lock()
	defer ag.writing.Unlock()
	if _, err := ag.stdin.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("pi is not running: %w", err)
	}
	return nil
}

// call sends a command and decodes the data of its response. A failed write
// returns its error. A process that ends before it responds returns why it
// ended.
func (ag *agent) call(ctx context.Context, command string, params map[string]any, result any) error {
	reply := make(chan response, 1)
	ag.callMu.Lock()
	ag.next++
	id := strconv.Itoa(ag.next)
	ag.calls[id] = reply
	ag.callMu.Unlock()
	forget := func() {
		ag.callMu.Lock()
		delete(ag.calls, id)
		ag.callMu.Unlock()
	}
	rec := map[string]any{"id": id, "type": command}
	maps.Copy(rec, params)
	if err := ag.write(rec); err != nil {
		forget()
		return err
	}
	select {
	case r := <-reply:
		if r.err != "" {
			return fmt.Errorf("%s: %s", command, r.err)
		}
		if result == nil {
			return nil
		}
		return json.Unmarshal(r.data, result)
	case <-ag.ended:
		forget()
		return fmt.Errorf("%s: %w", command, ag.exit)
	case <-ctx.Done():
		forget()
		return fmt.Errorf("%s: %w", command, ctx.Err())
	}
}

// response hands a command's response to its caller.
func (ag *agent) response(line []byte, id string) {
	var m struct {
		Success bool            `json:"success"`
		Error   string          `json:"error"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(line, &m); err != nil {
		return
	}
	r := response{data: m.Data}
	if !m.Success {
		r.err = m.Error
		if r.err == "" {
			r.err = "failed"
		}
	}
	if id == statsID {
		ag.stats(r)
		return
	}
	ag.callMu.Lock()
	reply := ag.calls[id]
	delete(ag.calls, id)
	ag.callMu.Unlock()
	if reply == nil {
		// A parse failure has no ID; a caller that gave up has no channel.
		if !m.Success {
			fmt.Fprintf(os.Stderr, "hypercube-harness-pi: pi rejected a command: %s\n", m.Error)
		}
		return
	}
	reply <- r
}
