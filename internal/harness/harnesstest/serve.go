package harnesstest

import (
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
)

// Serving is a test's connection to a plugin's public serve entry point.
type Serving struct {
	t       testing.TB
	in      *io.PipeWriter
	records chan harness.Record
}

// Serve starts a plugin and closes its connection when the test ends.
func Serve(t testing.TB, serve func(io.Reader, io.Writer) error) *Serving {
	t.Helper()
	in, input := io.Pipe()
	output, out := io.Pipe()
	c := &Serving{t: t, in: input, records: make(chan harness.Record, 64)}
	done := make(chan error, 1)
	go func() {
		err := serve(in, out)
		_ = out.Close()
		done <- err
	}()
	go func() {
		defer close(c.records)
		dec := json.NewDecoder(output)
		for {
			var r harness.Record
			if err := dec.Decode(&r); err != nil {
				return
			}
			c.records <- r
		}
	}()
	t.Cleanup(func() {
		_ = input.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(10 * time.Second):
			t.Error("plugin did not stop")
		}
		_ = output.Close()
		_ = in.Close()
	})
	return c
}

func (c *Serving) Send(cmd harness.Command) {
	c.t.Helper()
	line, err := cmd.Line()
	if err != nil {
		c.t.Fatal(err)
	}
	if _, err := c.in.Write(line); err != nil {
		c.t.Fatal(err)
	}
}

// Until reads through the first matching record, preserving their order.
func (c *Serving) Until(match func(harness.Record) bool) harness.Record {
	c.t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for {
		select {
		case r, ok := <-c.records:
			if !ok {
				c.t.Fatal("plugin connection closed")
			}
			if match(r) {
				return r
			}
		case <-timer.C:
			c.t.Fatal("plugin did not send the expected record")
		}
	}
}

func (c *Serving) Reply(id int64) harness.Reply {
	c.t.Helper()
	return *c.Until(func(r harness.Record) bool { return r.Reply != nil && r.Reply.ID == id }).Reply
}
