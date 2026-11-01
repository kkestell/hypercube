// Package mcp gives agents the Hypercube tools. Each agent's harness runs
// `hypercube mcp`, a stdio MCP server that forwards every request, with the
// agent's identity, to the app's /mcp route. The tools run in the app, beside
// the browser's actions.
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/report"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
)

type request struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func schema(required []string, props ...string) map[string]any {
	p := map[string]any{}
	for _, name := range props {
		p[name] = map[string]any{"type": "string"}
	}
	return map[string]any{"type": "object", "properties": p, "required": required}
}

const dirArg = "an existing directory given as an absolute path, a path starting with ~/, or a path relative to the lead's directory, the worker works directly in it"

const profileArg = "The profile sets the worker's harness, model, and effort. An optional message is queued for the worker before it starts."

// toolsFor returns the tools for the workspace's mode and the configured
// profiles. Outside Git, workers cannot have worktrees, so create_agent
// requires dir.
func toolsFor(noGit bool, profiles []harness.Profile) []tool {
	description := "Lead only. Create a worker and launch it. Without dir, the worker gets its own branch and worktree from HEAD. With dir, " + dirArg + ", and Hypercube creates no branch or worktree. " + profileArg
	required := []string{"name", "profile"}
	if noGit {
		description = "Lead only. Create a worker and launch it. Given dir, " + dirArg + ". " + profileArg
		required = append(required, "dir")
	}
	create := tool{"create_agent", description, schema(required, "name", "profile", "dir", "message")}
	remove := tool{"remove_agent", "Lead only. End a worker and remove it from the team. A worker with a worktree also has its worktree and branch removed, and Git refuses dirty worktrees and unmerged branches. A worker without one has its directory left as it is.", schema([]string{"name"}, "name")}
	if noGit {
		remove.Description = "Lead only. End a worker and remove it from the team. Its directory is left as it is."
	}
	if len(profiles) > 0 {
		var names []string
		for _, p := range profiles {
			names = append(names, p.Name)
		}
		create.InputSchema["properties"].(map[string]any)["profile"] = map[string]any{"type": "string", "enum": names}
	}
	return []tool{
		{"send_message", "Send a message to a Hypercube agent in your session, by name or session/name address. This is the only way to reach Hypercube agents. A lead can also message another session's lead at session/lead.", schema([]string{"to", "text"}, "to", "text")},
		{"list_agents", "List the Hypercube agents in your session and, for a lead, other sessions' leads: address, session title, role, harness, activity, directory, and branch.", schema([]string{})},
		{"set_status", "Worker only. Tell the user in a sentence or two what you are doing now. The user's view shows it on your card in place of your previous status.", schema([]string{"text"}, "text")},
		create,
		remove,
	}
}

// AgentHeader names the agent a forwarded request comes from.
const AgentHeader = "X-Hypercube-Agent"

// Bridge answers MCP requests on stdin until it closes, by forwarding each to
// the app. HYPERCUBE_URL is the app's address and HYPERCUBE_AGENT_ID the agent.
func Bridge(stdin io.Reader, stdout io.Writer) error {
	id, base := os.Getenv("HYPERCUBE_AGENT_ID"), os.Getenv("HYPERCUBE_URL")
	if id == "" || base == "" {
		return errors.New("HYPERCUBE_AGENT_ID and HYPERCUBE_URL must be set")
	}
	out := json.NewEncoder(stdout)
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 1<<20), 64<<20)
	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil || req.ID == nil {
			continue
		}
		answer, err := forward(base, id, scanner.Bytes())
		if err != nil {
			answer, _ = json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID,
				"error": map[string]any{"code": -32603, "message": err.Error()}})
		}
		if err := out.Encode(json.RawMessage(answer)); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// forward posts one request to the app. The app accepts only requests whose
// Origin is its own address.
func forward(base, id string, body []byte) ([]byte, error) {
	req, err := http.NewRequest("POST", base+"/mcp", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set(AgentHeader, id)
	req.Header.Set("Origin", base)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Hypercube is not reachable: %w", err)
	}
	defer res.Body.Close()
	answer, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Hypercube refused the request: %s", strings.TrimSpace(string(answer)))
	}
	return answer, nil
}

// Handler is the app's /mcp route: it runs one forwarded request for the agent
// named in AgentHeader and answers with the JSON-RPC response.
func Handler(db *store.DB, rt *session.Runtime) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(AgentHeader)
		if err := db.Read(func(tx *store.Tx) error { _, err := tx.Agent(id); return err }); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		var req request
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<20)).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.ID == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": handle(db, rt, id, req)})
	})
}

func handle(db *store.DB, rt *session.Runtime, id string, req request) any {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		json.Unmarshal(req.Params, &p)
		return map[string]any{
			"protocolVersion": p.ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "hypercube", "version": "0.1.0"},
		}
	case "tools/list":
		// An unreadable database or configuration fails the tool calls that
		// need it, so it needs no separate error here.
		w, _ := workspaceOf(db, id)
		cfg, _ := harness.LoadConfig()
		return map[string]any{"tools": toolsFor(w.NoGit, cfg.Workers())}
	case "tools/call":
		var p struct {
			Name      string            `json:"name"`
			Arguments map[string]string `json:"arguments"`
		}
		err := json.Unmarshal(req.Params, &p)
		var text string
		if err == nil {
			text, err = call(db, rt, id, p.Name, p.Arguments)
		}
		if err != nil {
			text = err.Error()
		}
		return map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}, "isError": err != nil}
	}
	return map[string]any{}
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

func call(db *store.DB, rt *session.Runtime, self, name string, args map[string]string) (string, error) {
	switch name {
	case "send_message":
		return sendMessage(db, rt, self, args["to"], args["text"])
	case "list_agents":
		return listAgents(db, self)
	case "set_status":
		return setStatus(db, self, args["text"])
	case "create_agent":
		return createAgent(rt, self, args)
	case "remove_agent":
		return removeAgent(rt, self, args["name"])
	}
	return "", fmt.Errorf("unknown tool %q", name)
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

func sendMessage(db *store.DB, rt *session.Runtime, self, to, text string) (string, error) {
	// A misnamed argument arrives as empty text; failing lets the model retry.
	if strings.TrimSpace(text) == "" {
		return "", errors.New("text is required")
	}
	var recipient store.Agent
	running := false
	err := db.Tx(func(tx *store.Tx) error {
		caller, err := tx.Agent(self)
		if err != nil {
			return err
		}
		a, err := tx.Find(caller.Session, to)
		if err != nil {
			return err
		}
		if a.ID == self {
			return errors.New("cannot send a message to yourself")
		}
		from, err := tx.Session(caller.Session)
		if err != nil {
			return err
		}
		s, err := tx.Session(a.Session)
		if err != nil {
			return err
		}
		if a.Session != caller.Session && (caller.Role != store.Lead || a.Role != store.Lead || s.Workspace != from.Workspace) {
			return errors.New("across sessions, only leads in the same workspace can message each other")
		}
		recipient, running = a, s.Running
		return tx.Queue(store.Message{From: self, To: a.ID, Text: text})
	})
	if err != nil {
		return "", err
	}
	rt.Deliver(recipient.ID)
	reply := fmt.Sprintf("Message queued for %s.", recipient.Address())
	if !running {
		reply += " Its session is stopped; the message will be delivered when the session starts."
	}
	return reply, nil
}

// setStatus records a worker's status on one line.
func setStatus(db *store.DB, self, text string) (string, error) {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "", errors.New("text is required")
	}
	err := db.Tx(func(tx *store.Tx) error {
		a, err := tx.Agent(self)
		if err != nil {
			return err
		}
		if a.Role != store.Worker {
			return errors.New("only workers have a status")
		}
		a.Status, a.StatusAt = text, time.Now().UnixMilli()
		return tx.SaveAgent(a)
	})
	if err != nil {
		return "", err
	}
	return "Status set.", nil
}

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func createAgent(rt *session.Runtime, self string, args map[string]string) (string, error) {
	name := args["name"]
	if !validName.MatchString(name) {
		return "", errors.New("name must be lowercase letters, digits, and hyphens")
	}
	cfg, err := harness.LoadConfig()
	if err != nil {
		return "", err
	}
	profile, err := cfg.Worker(args["profile"])
	if err != nil {
		return "", err
	}
	a, err := rt.AddWorker(self, name, profile, args["dir"], args["message"])
	if err != nil {
		return "", err
	}
	if a.Branch == "" {
		return fmt.Sprintf("Created %s in %s.", a.Address(), a.Dir), nil
	}
	return fmt.Sprintf("Created %s on branch %s in %s.", a.Address(), a.Branch, a.Dir), nil
}

func removeAgent(rt *session.Runtime, self, name string) (string, error) {
	// A misnamed argument arrives as an empty name; failing lets the model retry.
	if strings.TrimSpace(name) == "" {
		return "", errors.New("name is required")
	}
	a, err := rt.RemoveWorker(self, name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Removed %s.", a.Address()), nil
}
