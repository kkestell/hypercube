// Package mcp serves the Hypercube tools to agents at the backend's /mcp
// route. Each agent's harness runs hypercube-mcp (internal/mcpbridge), which
// forwards requests here with the session ID in SessionHeader. The tools run
// in the backend, beside the browser's actions.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
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

// newServer returns the server for one session.
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

	sendDescription := "Send a message to one of your child sessions by its name, or to any other top-level session across all workspaces in this app by its ID. A recipient that is not running starts to receive it. Ended recipients are refused. This is the only way to reach other Hypercube sessions."
	if caller.Parent != "" {
		sendDescription = fmt.Sprintf("Send a message to your parent session at %s. This is the only session you can message.", caller.Parent)
	}
	sendDescription += " Check list_sessions first: messaging a parked session wakes its agent. Do not message parked sessions unless you have a serious reason or the user explicitly requests it."
	server := mcp.NewServer(&mcp.Implementation{Name: "hypercube", Version: "0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "send_message", Description: sendDescription}, result(func(_ context.Context, a sendMessageArgs) (string, error) { return sendMessage(rt, self, a.To, a.Text) }))
	mcp.AddTool(server, &mcp.Tool{Name: "open_file", Description: "Open a file in a tab beside your chat, which the user sees, and select it. Use it to show relevant code at its lines or a completed deliverable when that helps the user review the result. path is relative to your directory, absolute, or starts with ~ for the user's home; it may be anywhere the user can read. lines highlights ranges of lines, each {start, end} with end optional, and scrolls to the first; a Markdown file with lines shows its source, otherwise its preview. The tab opens now if the user is viewing your session, or the next time they do."}, result(func(_ context.Context, a openFileArgs) (string, error) { return openFile(rt, caller.Dir, self, a) }))
	mcp.AddTool(server, &mcp.Tool{Name: "list_sessions", Description: "List the sessions you can message: to, title, harness, model, effort (empty for a model without efforts), activity, parked, directory, branch, workspace path, and parent ID. parked is true when the session has no current agent launch; messaging it starts its agent. Activity is separate: exited also covers startup and does not imply parked. Ended sessions are excluded. Children are addressed by name; top-level sessions by ID."}, result(func(context.Context, struct{}) (string, error) { return listSessions(db, self) }))
	if caller.Parent != "" {
		return server
	}
	mcp.AddTool(server, &mcp.Tool{Name: "read_session", Description: readSessionDescription}, result(func(_ context.Context, a readSessionArgs) (string, error) { return readSession(db, rt, self, a) }))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_title",
		Description: "Set your session's title: a short display title for the user, 48 characters or fewer. It does not change how sessions are addressed.",
	}, result(func(_ context.Context, a setTitleArgs) (string, error) {
		if err := rt.Rename(self, a.Title); err != nil {
			return "", err
		}
		return "Title set.", nil
	}))
	mcp.AddTool(server, &mcp.Tool{Name: "list_models", Description: listModelsDescription}, result(func(ctx context.Context, a listModelsArgs) (string, error) {
		return listModels(ctx, rt, caller.Dir, a)
	}))
	addChildTools(server, rt, self, w)
	return server
}

// SessionHeader names the session a request comes from.
const SessionHeader = "X-Hypercube-Session"

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
		Model     string `json:"model"`
		Effort    string `json:"effort"`
		Activity  string `json:"activity"`
		Parked    bool   `json:"parked"`
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
			listed = append(listed, entry{To: to, Title: s.Title, Harness: s.Harness, Model: s.Model, Effort: s.Effort, Activity: s.Activity, Parked: s.Launch == "", Dir: s.Dir, Branch: s.Branch, Workspace: w.Path, Parent: s.Parent})
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
