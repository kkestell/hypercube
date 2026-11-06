// Package mcp gives agents the Hypercube tools. Each agent's harness runs
// `hypercube mcp`, a stdio MCP server that forwards every request, with the
// session ID, to the app's /mcp route. The tools run in the app, beside
// the browser's actions.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type sendMessageArgs struct {
	To   string `json:"to"`
	Text string `json:"text"`
}

type setTitleArgs struct {
	Title string `json:"title"`
}

// result wraps a tool's text; an error becomes an error result the model sees.
func result[In any](run func(context.Context, In) (string, error)) mcp.ToolHandlerFor[In, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		text, err := run(ctx, in)
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
	}
}

// newServer returns the server for one session; browser drives its browser,
// and is nil without the Hypercube window.
func newServer(db *store.DB, rt *session.Runtime, browser Browser, self string) *mcp.Server {
	// An unreadable database or configuration fails the tool calls that need
	// it, so it needs no separate error here.
	var caller store.Session
	var w store.Workspace
	_ = db.Read(func(tx *store.Tx) error {
		var err error
		caller, err = tx.Session(self)
		if err != nil {
			return err
		}
		w, err = tx.Workspace(caller.Workspace)
		return err
	})

	sendDescription := "Send a message to one of your child sessions by its name, or to any other top-level session across all workspaces in this app by its ID. A recipient that is not running starts to receive it. Ended recipients are refused. This is the only way to reach other Hypercube sessions."
	if caller.Parent != "" {
		sendDescription = fmt.Sprintf("Send a message to your parent session at %s. This is the only session you can message.", caller.Parent)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "hypercube", Version: "0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "send_message", Description: sendDescription}, result(func(_ context.Context, a sendMessageArgs) (string, error) { return sendMessage(rt, self, a.To, a.Text) }))
	mcp.AddTool(server, &mcp.Tool{Name: "open_file", Description: "Open a file in a tab beside your chat, which the user sees, and select it. path is relative to your directory, absolute, or starts with ~ for the user's home; it may be anywhere the user can read. lines highlights ranges of lines, each {start, end} with end optional, and scrolls to the first; a Markdown file with lines shows its source, otherwise its preview. The tab opens now if the user is viewing your session, or the next time they do."}, result(func(_ context.Context, a openFileArgs) (string, error) { return openFile(rt, caller.Dir, self, a) }))
	addBrowserTools(server, browser, self)
	mcp.AddTool(server, &mcp.Tool{Name: "list_sessions", Description: "List the sessions you can message: to, title, harness, activity, directory, branch, workspace path, and parent ID. Children are addressed by name; top-level sessions by ID."}, result(func(context.Context, struct{}) (string, error) { return listSessions(db, self) }))
	if caller.Parent != "" {
		return server
	}
	mcp.AddTool(server, &mcp.Tool{Name: "read_session", Description: readSessionDescription}, result(func(_ context.Context, a readSessionArgs) (string, error) { return readSession(db, rt, self, a) }))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_title",
		Description: "Set your session's title: a short display title for the user. It does not change how sessions are addressed.",
	}, result(func(_ context.Context, a setTitleArgs) (string, error) {
		if err := rt.Rename(self, a.Title); err != nil {
			return "", err
		}
		return "Title set.", nil
	}))
	addChildTools(server, rt, self, w)
	return server
}

// SessionHeader names the session a request comes from.
const SessionHeader = "X-Hypercube-Session"

// Bridge answers MCP requests on stdin until it closes, by forwarding each to
// the app. HYPERCUBE_URL is the app's address and HYPERCUBE_SESSION_ID its session.
// The SDK has no stdio-to-HTTP forwarder, so Bridge moves messages between the
// SDK's stdio and streamable HTTP transports itself, one request at a time.
func Bridge(stdin io.Reader, stdout io.Writer) error {
	id, base := os.Getenv("HYPERCUBE_SESSION_ID"), os.Getenv("HYPERCUBE_URL")
	if id == "" || base == "" {
		return errors.New("HYPERCUBE_SESSION_ID and HYPERCUBE_URL must be set")
	}
	ctx := context.Background()
	agent, err := (&mcp.StreamableClientTransport{
		Endpoint:             base + "/mcp",
		HTTPClient:           &http.Client{Transport: identity(id)},
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

// identity adds the session ID that the app's /mcp route requires.
type identity string

func (i identity) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set(SessionHeader, string(i))
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

// Handler is the app's /mcp route: it serves the tools to the session named in
// SessionHeader, with browser as in newServer. Each request is answered on its own, so the tools follow the
// current workspace and configuration.
func Handler(db *store.DB, rt *session.Runtime, browser Browser) http.Handler {
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return newServer(db, rt, browser, r.Header.Get(SessionHeader))
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := db.Read(func(tx *store.Tx) error { _, err := tx.Session(r.Header.Get(SessionHeader)); return err }); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	})
}

// listSessions lists exactly the sessions in the caller's messaging scope.
func listSessions(db *store.DB, self string) (string, error) {
	type entry struct {
		To        string `json:"to"`
		Title     string `json:"title"`
		Harness   string `json:"harness"`
		Activity  string `json:"activity"`
		Dir       string `json:"directory"`
		Branch    string `json:"branch"`
		Workspace string `json:"workspace"`
		Parent    string `json:"parent"`
	}
	listed := []entry{}
	err := db.Read(func(tx *store.Tx) error {
		caller, err := tx.Session(self)
		if err != nil {
			return err
		}
		var sessions []store.Session
		if caller.Parent != "" {
			parent, err := tx.Session(caller.Parent)
			if err != nil {
				return err
			}
			sessions = []store.Session{parent}
		} else {
			sessions, err = tx.Children(caller.ID)
			if err != nil {
				return err
			}
			peers, err := tx.Sessions("")
			if err != nil {
				return err
			}
			sessions = append(sessions, peers...)
		}
		for _, s := range sessions {
			if s.ID == self || s.Ended {
				continue
			}
			w, err := tx.Workspace(s.Workspace)
			if err != nil {
				return err
			}
			to := s.ID
			if s.Parent == caller.ID {
				to = s.Name
			}
			listed = append(listed, entry{To: to, Title: s.Title, Harness: s.Harness, Activity: s.Activity, Dir: s.Dir, Branch: s.Branch, Workspace: w.Path, Parent: s.Parent})
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(listed, "", "  ")
	return string(b), err
}

func sendMessage(rt *session.Runtime, self, to, text string) (string, error) {
	if _, err := rt.SendMessage(self, to, text); err != nil {
		return "", err
	}
	return fmt.Sprintf("Message queued for %s.", to), nil
}
