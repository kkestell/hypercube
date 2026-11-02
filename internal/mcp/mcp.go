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

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const dirArg = "an existing directory given as an absolute path, a path starting with ~/, or a path relative to your directory, the child session works directly in it"

const profileArg = "The profile sets the child session's harness, model, and effort. An optional message is queued for the child session before it starts."

type sendMessageArgs struct {
	To   string `json:"to"`
	Text string `json:"text"`
}

type setStatusArgs struct {
	Text string `json:"text"`
}

type createSessionArgs struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Profile string `json:"profile"`
	Dir     string `json:"dir,omitempty"`
	Message string `json:"message,omitempty"`
}

type setTitleArgs struct {
	Title string `json:"title"`
}

type endSessionArgs struct {
	Name string `json:"name"`
}

// result wraps a tool's text; an error becomes an error result the model sees.
func result[In any](run func(In) (string, error)) mcp.ToolHandlerFor[In, any] {
	return func(_ context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		text, err := run(in)
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
	}
}

// newServer returns the server for one session. Outside Git, child sessions cannot
// have worktrees, so create_session requires dir. Profiles restrict profile to
// the configured profiles.
func newServer(db *store.DB, rt *session.Runtime, self string) *mcp.Server {
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

	sendDescription := "Send a message to one of your running child sessions by its name, or to any other running top-level session across all workspaces in this app by its ID. Stopped and ended recipients are refused. This is the only way to reach other Hypercube sessions."
	if caller.Parent != "" {
		sendDescription = fmt.Sprintf("Send a message to your parent session at %s. This is the only session you can message, and it must be running.", caller.Parent)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "hypercube", Version: "0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "send_message", Description: sendDescription}, result(func(a sendMessageArgs) (string, error) { return sendMessage(rt, self, a.To, a.Text) }))
	mcp.AddTool(server, &mcp.Tool{Name: "list_sessions", Description: "List the running sessions you can message: to, title, harness, activity, directory, branch, workspace path, and parent ID. Children are addressed by name; top-level sessions by ID."}, result(func(struct{}) (string, error) { return listSessions(db, self) }))
	if caller.Parent != "" {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "set_status",
			Description: "Tell the user in a sentence or two what you are doing now. It appears in your parent session's transcript for the user; your parent's agent does not see it.",
		}, result(func(a setStatusArgs) (string, error) { return setStatus(rt, self, a.Text) }))
		return server
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_title",
		Description: "Set your session's title: a short display title for the user. It does not change how sessions are addressed.",
	}, result(func(a setTitleArgs) (string, error) {
		if err := rt.Rename(self, a.Title); err != nil {
			return "", err
		}
		return "Title set.", nil
	}))
	cfg, cfgErr := harness.LoadConfig()

	createSchema, err := jsonschema.For[createSessionArgs](nil)
	if err != nil {
		panic(err)
	}
	createDescription := "Create a child session and launch its agent. The title is a short, meaningful display title for the user; the name stays its address. Without dir, the child session gets its own branch and worktree from HEAD. With dir, " + dirArg + ", and Hypercube creates no branch or worktree. " + profileArg
	if w.NoGit {
		createDescription = "Create a child session and launch its agent. The title is a short, meaningful display title for the user; the name stays its address. Given dir, " + dirArg + ". " + profileArg
		createSchema.Required = append(createSchema.Required, "dir")
	}
	for _, p := range cfg.Profiles {
		createSchema.Properties["profile"].Enum = append(createSchema.Properties["profile"].Enum, p.Name)
	}
	mcp.AddTool(server, &mcp.Tool{Name: "create_session", Description: createDescription, InputSchema: createSchema}, result(func(a createSessionArgs) (string, error) {
		if cfgErr != nil {
			return "", cfgErr
		}
		return createSession(rt, self, cfg, a)
	}))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "end_session",
		Description: "End a child session: its agent stops for good, and a child with a worktree also has its worktree and branch removed; Git refuses dirty worktrees and unmerged branches. The ended child stays listed for the user and cannot be messaged.",
	}, result(func(a endSessionArgs) (string, error) { return endSession(rt, self, a.Name) }))
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
// SessionHeader. Each request is answered on its own, so the tools follow the
// current workspace and configuration.
func Handler(db *store.DB, rt *session.Runtime) http.Handler {
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return newServer(db, rt, r.Header.Get(SessionHeader))
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
			running, err := tx.Running(s)
			if err != nil {
				return err
			}
			if !running {
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

func setStatus(rt *session.Runtime, self, text string) (string, error) {
	if err := rt.SetStatus(self, text); err != nil {
		return "", err
	}
	return "Status set.", nil
}

func createSession(rt *session.Runtime, self string, cfg harness.Config, args createSessionArgs) (string, error) {
	profile, err := cfg.Profile(args.Profile)
	if err != nil {
		return "", err
	}
	child, err := rt.CreateChild(self, args.Name, args.Title, profile, args.Dir, args.Message)
	if err != nil {
		return "", err
	}
	if child.Branch == "" {
		return fmt.Sprintf("Created %s in %s.", child.Name, child.Dir), nil
	}
	return fmt.Sprintf("Created %s on branch %s in %s.", child.Name, child.Branch, child.Dir), nil
}

func endSession(rt *session.Runtime, self, name string) (string, error) {
	child, err := rt.EndChild(self, name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Ended %s.", child.Name), nil
}
