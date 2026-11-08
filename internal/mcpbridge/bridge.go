// Package mcpbridge is the stdio MCP server each agent's harness runs. It
// forwards every message to the Hypercube backend's /mcp route with the
// agent's session ID and writes the backend's answers back. The SDK has no
// stdio-to-HTTP forwarder, so Run moves messages between the SDK's stdio and
// streamable HTTP transports itself, one request at a time.
package mcpbridge

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// sessionHeader names the session a request comes from. The backend's /mcp
// handler (internal/mcp.SessionHeader) reads the same header.
const sessionHeader = "X-Hypercube-Session"

// Main runs the bridge for the session in HYPERCUBE_SESSION_ID against the
// backend at HYPERCUBE_URL until stdin closes.
func Main(stdin io.Reader, stdout io.Writer) error {
	id, base := os.Getenv("HYPERCUBE_SESSION_ID"), os.Getenv("HYPERCUBE_URL")
	if id == "" || base == "" {
		return errors.New("HYPERCUBE_SESSION_ID and HYPERCUBE_URL must be set")
	}
	return Run(context.Background(), base, id, stdin, stdout)
}

// Run forwards MCP messages from stdin to the backend at url for session
// until stdin closes or ctx ends.
func Run(ctx context.Context, url, session string, stdin io.Reader, stdout io.Writer) error {
	agent, err := (&mcp.StreamableClientTransport{
		Endpoint:             url + "/mcp",
		HTTPClient:           &http.Client{Transport: identity(session)},
		DisableStandaloneSSE: true,
	}).Connect(ctx)
	if err != nil {
		return err
	}
	defer agent.Close()
	stdio, err := (&mcp.IOTransport{Reader: io.NopCloser(stdin), Writer: nopCloser{stdout}}).Connect(ctx)
	if err != nil {
		return err
	}
	defer stdio.Close()
	for {
		msg, err := stdio.Read(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		req, isRequest := msg.(*jsonrpc.Request)
		if err := agent.Write(ctx, msg); err != nil {
			if !isRequest || !req.IsCall() {
				continue
			}
			failure := &jsonrpc.Response{ID: req.ID, Error: &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: err.Error()}}
			if err := stdio.Write(ctx, failure); err != nil {
				return err
			}
			continue
		}
		if !isRequest || !req.IsCall() {
			continue
		}
		for {
			answer, err := agent.Read(ctx)
			if err != nil {
				return err
			}
			if err := stdio.Write(ctx, answer); err != nil {
				return err
			}
			if r, ok := answer.(*jsonrpc.Response); ok && r.ID == req.ID {
				break
			}
		}
	}
}

// identity adds the session ID that the backend's /mcp route requires.
type identity string

func (i identity) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set(sessionHeader, string(i))
	deadline := time.Now().Add(30 * time.Second)
	for {
		response, err := http.DefaultTransport.RoundTrip(r)
		if !errors.Is(err, syscall.ECONNREFUSED) || time.Now().After(deadline) {
			return response, err
		}
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		case <-time.After(250 * time.Millisecond):
		}
		// A refused connection sent no bytes, but the transport closed its body.
		if r.GetBody != nil {
			r.Body, err = r.GetBody()
			if err != nil {
				return nil, err
			}
		}
	}
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }
