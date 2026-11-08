// Package harnessclient runs harness plugins for the backend: it finds them
// in the plugin directory, runs their discovery commands, and starts and
// attaches to their serve command under a holder.
package harnessclient

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/hold"
)

// Client attaches to one plugin's holder.
type Client struct {
	holder *hold.Holder
	mu     sync.Mutex
	output *hold.Output
	closed bool
}

// Launch starts the plugin's serve command under a detached holder.
func Launch(holder, dir, plugin string) (*Client, error) {
	h, err := hold.Start(holder, dir, []string{plugin, "serve"}, os.Environ(), "", nil)
	if err != nil {
		return nil, err
	}
	return &Client{holder: h}, nil
}
func Attach(dir string) (*Client, error) {
	h, err := hold.Open(dir)
	if err != nil {
		return nil, err
	}
	return &Client{holder: h}, nil
}

// Send writes one command line to the plugin's input.
func (c *Client) Send(line []byte) error {
	_, err := c.holder.Write(line)
	return err
}

// Read applies complete records in output order, passing their end position.
// A handler error is a protocol error and stops the plugin.
func (c *Client) Read(from int64, handle func(harness.Record, int64) error) error {
	output, err := c.holder.Output(from)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.output = output
	if c.closed {
		output.Close()
	}
	c.mu.Unlock()
	defer output.Close()
	lines := bufio.NewReader(output)
	for {
		line, err := lines.ReadBytes('\n')
		if len(line) > 0 {
			if err != nil && err != io.EOF {
				return err
			}
			from += int64(len(line))
			if len(bytes.TrimSpace(line)) > 0 {
				rec, decodeErr := harness.DecodeRecord(line)
				if decodeErr == nil {
					decodeErr = handle(rec, from)
				}
				if decodeErr != nil {
					_ = c.Stop()
					return decodeErr
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				return err
			}
			break
		}
	}
	e, err := c.holder.Exit()
	if err != nil {
		return err
	}
	reason := "plugin ended"
	if e.Status != "" {
		reason += ": " + e.Status
	}
	if detail := strings.TrimSpace(e.Stderr); detail != "" {
		parts := strings.Split(detail, "\n")
		reason += ": " + strings.TrimSpace(parts[len(parts)-1])
	}
	return errors.New(reason)
}
func (c *Client) Stop() error                { return c.holder.Stop() }
func (c *Client) Discard(before int64) error { return c.holder.Discard(before) }
func (c *Client) Remove() error              { return c.holder.Remove() }
func (c *Client) Close() error {
	c.mu.Lock()
	c.closed = true
	if c.output != nil {
		c.output.Close()
	}
	c.mu.Unlock()
	return c.holder.Close()
}
