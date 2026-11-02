// Package mcp gives agents the Hypercube tools. Each agent's harness runs
// `hypercube mcp`, a stdio MCP server that forwards every request, with the
// agent's identity, to the app's /mcp route. The tools run in the app, beside
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
	"regexp"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/report"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const dirArg = "an existing directory given as an absolute path, a path starting with ~/, or a path relative to the lead's directory, the worker works directly in it"

const profileArg = "The profile sets the worker's harness, model, and effort. An optional message is queued for the worker before it starts."

type sendMessageArgs struct {
	To   string `json:"to"`
	Text string `json:"text"`
}

type setStatusArgs struct {
	Text string `json:"text"`
}

type createAgentArgs struct {
	Name    string `json:"name"`
	Profile string `json:"profile"`
	Dir     string `json:"dir,omitempty"`
	Message string `json:"message,omitempty"`
}

type removeAgentArgs struct {
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

// newServer returns the server for one agent. Outside Git, workers cannot
// have worktrees, so create_agent requires dir. Profiles restrict profile to
// the configured worker profiles.
func newServer(db *store.DB, rt *session.Runtime, self string) *mcp.Server {
	// An unreadable database or configuration fails the tool calls that need
	// it, so it needs no separate error here.
	w, _ := workspaceOf(db, self)
	cfg, cfgErr := harness.LoadConfig()

	createSchema, err := jsonschema.For[createAgentArgs](nil)
	if err != nil {
		panic(err)
	}
	createDescription := "Lead only. Create a worker and launch it. Without dir, the worker gets its own branch and worktree from HEAD. With dir, " + dirArg + ", and Hypercube creates no branch or worktree. " + profileArg
	removeDescription := "Lead only. End a worker and remove it from the team. A worker with a worktree also has its worktree and branch removed, and Git refuses dirty worktrees and unmerged branches. A worker without one has its directory left as it is."
	if w.NoGit {
		createDescription = "Lead only. Create a worker and launch it. Given dir, " + dirArg + ". " + profileArg
		removeDescription = "Lead only. End a worker and remove it from the team. Its directory is left as it is."
		createSchema.Required = append(createSchema.Required, "dir")
	}
	for _, p := range cfg.Workers() {
		createSchema.Properties["profile"].Enum = append(createSchema.Properties["profile"].Enum, p.Name)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "hypercube", Version: "0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "send_message",
		Description: "Send a message to a Hypercube agent in your session, by name or session/name address. This is the only way to reach Hypercube agents. A lead can also message another session's lead at session/lead.",
	}, result(func(a sendMessageArgs) (string, error) { return sendMessage(rt, self, a.To, a.Text) }))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_agents",
		Description: "List the Hypercube agents in your session and, for a lead, other sessions' leads: address, session title, role, harness, activity, directory, and branch.",
	}, result(func(struct{}) (string, error) { return listAgents(db, self) }))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_status",
		Description: "Worker only. Tell the user in a sentence or two what you are doing now. The user's view shows it on your card in place of your previous status.",
	}, result(func(a setStatusArgs) (string, error) { return setStatus(rt, self, a.Text) }))
	mcp.AddTool(server, &mcp.Tool{Name: "create_agent", Description: createDescription, InputSchema: createSchema},
		result(func(a createAgentArgs) (string, error) {
			if cfgErr != nil {
				return "", cfgErr
			}
			return createAgent(rt, self, cfg, a)
		}))
	mcp.AddTool(server, &mcp.Tool{Name: "remove_agent", Description: removeDescription},
		result(func(a removeAgentArgs) (string, error) { return removeAgent(rt, self, a.Name) }))
	return server
}

// AgentHeader names the agent a request comes from.
const AgentHeader = "X-Hypercube-Agent"

// Bridge answers MCP requests on stdin until it closes, by forwarding each to
// the app. HYPERCUBE_URL is the app's address and HYPERCUBE_AGENT_ID the agent.
// The SDK has no stdio-to-HTTP forwarder, so Bridge moves messages between the
// SDK's stdio and streamable HTTP transports itself, one request at a time.
func Bridge(stdin io.Reader, stdout io.Writer) error {
	id, base := os.Getenv("HYPERCUBE_AGENT_ID"), os.Getenv("HYPERCUBE_URL")
	if id == "" || base == "" {
		return errors.New("HYPERCUBE_AGENT_ID and HYPERCUBE_URL must be set")
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

// identity adds the agent ID that the app's /mcp route requires.
type identity string

func (i identity) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set(AgentHeader, string(i))
	return http.DefaultTransport.RoundTrip(r)
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

// Handler is the app's /mcp route: it serves the tools to the agent named in
// AgentHeader. Each request is answered on its own, so the tools follow the
// current workspace and configuration.
func Handler(db *store.DB, rt *session.Runtime) http.Handler {
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return newServer(db, rt, r.Header.Get(AgentHeader))
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := db.Read(func(tx *store.Tx) error { _, err := tx.Agent(r.Header.Get(AgentHeader)); return err }); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	})
}

func workspaceOf(db *store.DB, id string) (store.Workspace, error) {
	var w store.Workspace
	err := db.Read(func(tx *store.Tx) error {
		a, err := tx.Agent(id)
		if err != nil {
			return err
		}
		s, err := tx.Session(a.Session)
		if err != nil {
			return err
		}
		w, err = tx.Workspace(s.Workspace)
		return err
	})
	return w, err
}

// listAgents lists the caller's session and, for a lead, the other leads in
// its workspace.
func listAgents(db *store.DB, self string) (string, error) {
	type entry struct {
		Address  string `json:"address"`
		Title    string `json:"session_title"`
		Role     string `json:"role"`
		Harness  string `json:"harness"`
		Activity string `json:"activity"`
		Dir      string `json:"directory"`
		Branch   string `json:"branch"`
	}
	var listed []entry
	err := db.Read(func(tx *store.Tx) error {
		caller, err := tx.Agent(self)
		if err != nil {
			return err
		}
		s, err := tx.Session(caller.Session)
		if err != nil {
			return err
		}
		agents, err := tx.Agents(caller.Session)
		if err != nil {
			return err
		}
		leads, titles, err := report.Leads(tx, s.Workspace, s.ID)
		if err != nil {
			return err
		}
		if caller.Role == store.Lead {
			agents = append(agents, leads...)
		}
		for _, a := range agents {
			listed = append(listed, entry{a.Address(), titles[a.Session], a.Role, a.Harness, a.Activity, a.Dir, a.Branch})
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
	recipient, running, err := rt.SendMessage(self, to, text)
	if err != nil {
		return "", err
	}
	reply := fmt.Sprintf("Message queued for %s.", recipient.Address())
	if !running {
		reply += " Its session is stopped; the message will be delivered when the session starts."
	}
	return reply, nil
}

func setStatus(rt *session.Runtime, self, text string) (string, error) {
	if err := rt.SetStatus(self, text); err != nil {
		return "", err
	}
	return "Status set.", nil
}

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func createAgent(rt *session.Runtime, self string, cfg harness.Config, args createAgentArgs) (string, error) {
	if !validName.MatchString(args.Name) {
		return "", errors.New("name must be lowercase letters, digits, and hyphens")
	}
	profile, err := cfg.Worker(args.Profile)
	if err != nil {
		return "", err
	}
	a, err := rt.AddWorker(self, args.Name, profile, args.Dir, args.Message)
	if err != nil {
		return "", err
	}
	if a.Branch == "" {
		return fmt.Sprintf("Created %s in %s.", a.Address(), a.Dir), nil
	}
	return fmt.Sprintf("Created %s on branch %s in %s.", a.Address(), a.Branch, a.Dir), nil
}

func removeAgent(rt *session.Runtime, self, name string) (string, error) {
	a, err := rt.RemoveWorker(self, name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Removed %s.", a.Address()), nil
}
