// Package mcp is the stdio MCP server that gives agents the Hypercube tools.
package mcp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/mux"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/workspace"
	"github.com/kkestell/hypercube/mux/client"
)

// Nudge is the initial prompt for a worker; later messages use harness wake-ups.
const Nudge = "Check your Hypercube messages."

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
func toolsFor(s store.State, profiles []harness.Profile) []tool {
	create := tool{"create_agent", "Lead only. Create a worker and launch it. Without dir, the worker gets its own branch and worktree from HEAD. With dir, " + dirArg + ", and Hypercube creates no branch or worktree. " + profileArg,
		schema([]string{"name", "profile"}, "name", "profile", "dir", "message")}
	remove := tool{"remove_agent", "Lead only. Close a worker's pane and remove it from the team. A worker with a worktree also has its worktree and branch removed, and Git refuses dirty worktrees and unmerged branches. A worker without one has its directory left as it is.", schema([]string{"name"}, "name")}
	if s.NoGit {
		create = tool{create.Name, "Lead only. Create a worker and launch it. Given dir, " + dirArg + ". " + profileArg,
			schema([]string{"name", "profile", "dir"}, "name", "profile", "dir", "message")}
		remove.Description = "Lead only. Close a worker's pane and remove it from the team. Its directory is left as it is."
	}
	if len(profiles) > 0 {
		var names []string
		for _, p := range profiles {
			names = append(names, p.Name)
		}
		create.InputSchema["properties"].(map[string]any)["profile"] = map[string]any{"type": "string", "enum": names}
	}
	return []tool{
		{"send_message", "Send a message to an agent in your session, by name or session/name address. A lead can also message another session's lead at session/lead.", schema([]string{"to", "text"}, "to", "text")},
		{"list_agents", "List your session's agents and, for a lead, other sessions' leads: address, role, harness, activity, directory, and branch.", schema([]string{})},
		create,
		remove,
	}
}

// Serve answers MCP requests on stdin until it closes.
func Serve(stdin io.Reader, stdout io.Writer) error {
	id, path := os.Getenv("HYPERCUBE_AGENT_ID"), os.Getenv("HYPERCUBE_STATE")
	if id == "" || path == "" {
		return errors.New("HYPERCUBE_AGENT_ID and HYPERCUBE_STATE must be set")
	}
	out := json.NewEncoder(stdout)
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 1<<20), 64<<20)
	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil || req.ID == nil {
			continue
		}
		var result any
		switch req.Method {
		case "initialize":
			var p struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			json.Unmarshal(req.Params, &p)
			result = map[string]any{
				"protocolVersion": p.ProtocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "hypercube", "version": "0.1.0"},
			}
		case "tools/list":
			// An unreadable state or configuration fails the tool calls that
			// need it, so it needs no separate error here.
			s, _ := store.Load(path)
			cfg, _ := harness.LoadConfig()
			result = map[string]any{"tools": toolsFor(s, cfg.Profiles)}
		case "tools/call":
			var p struct {
				Name      string            `json:"name"`
				Arguments map[string]string `json:"arguments"`
			}
			err := json.Unmarshal(req.Params, &p)
			var text string
			if err == nil {
				text, err = call(id, path, p.Name, p.Arguments)
			}
			if err != nil {
				text = err.Error()
			}
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}, "isError": err != nil}
		default:
			result = map[string]any{}
		}
		if err := out.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result}); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func call(self, path, name string, args map[string]string) (string, error) {
	switch name {
	case "send_message":
		return sendMessage(self, path, args["to"], args["text"])
	case "list_agents":
		s, err := store.Load(path)
		if err != nil {
			return "", err
		}
		caller, err := s.Agent(self)
		if err != nil {
			return "", err
		}
		type entry struct {
			Address  string `json:"address"`
			Role     string `json:"role"`
			Harness  string `json:"harness"`
			Activity string `json:"activity"`
			Dir      string `json:"directory"`
			Branch   string `json:"branch"`
		}
		var listed []entry
		for _, a := range s.Agents {
			if a.Team != caller.Team && !(caller.Role == store.Lead && a.Role == store.Lead) {
				continue
			}
			listed = append(listed, entry{a.Address(), a.Role, a.Harness, a.Activity, a.Dir, a.Branch})
		}
		b, err := json.MarshalIndent(listed, "", "  ")
		return string(b), err
	case "create_agent":
		return createAgent(self, path, args)
	case "remove_agent":
		return removeAgent(self, path, args["name"])
	}
	return "", fmt.Errorf("unknown tool %q", name)
}

func sendMessage(self, path, to, text string) (string, error) {
	// A misnamed argument arrives as empty text; failing lets the model retry.
	if strings.TrimSpace(text) == "" {
		return "", errors.New("text is required")
	}
	var recipient store.Agent
	err := store.Update(path, func(s *store.State) error {
		caller, err := s.Agent(self)
		if err != nil {
			return err
		}
		a, err := s.Find(caller.Team, to)
		if err != nil {
			return err
		}
		if a.ID == self {
			return errors.New("cannot send a message to yourself")
		}
		if a.Team != caller.Team && (caller.Role != store.Lead || a.Role != store.Lead) {
			return errors.New("across sessions, only leads can message each other")
		}
		recipient = *a
		s.Messages = append(s.Messages, store.Message{From: self, To: a.ID, Text: text})
		return nil
	})
	if err != nil {
		return "", err
	}
	reply := fmt.Sprintf("Message queued for %s.", recipient.Address())
	if recipient.Activity == store.Exited {
		reply += " It is not running; the message will be delivered when the workspace starts again."
	}
	return reply, nil
}

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func requireLead(s *store.State, self string) (*store.Agent, error) {
	a, err := s.Agent(self)
	if err == nil && a.Role != store.Lead {
		err = errors.New("only the lead can do this")
	}
	return a, err
}

func createAgent(self, path string, args map[string]string) (string, error) {
	lock, err := store.LockFile(path + ".session.lock")
	if err != nil {
		return "", err
	}
	defer lock.Close()
	name := args["name"]
	if !validName.MatchString(name) {
		return "", errors.New("name must be lowercase letters, digits, and hyphens")
	}
	cfg, err := harness.LoadConfig()
	if err != nil {
		return "", err
	}
	profile, err := cfg.Profile(args["profile"])
	if err != nil {
		return "", err
	}

	var root string
	a := store.Agent{
		ID: store.NewID(), Name: name, Role: store.Worker,
		Harness: profile.Harness, Model: profile.Model, Effort: profile.Effort,
		Activity: store.Busy,
	}
	err = store.Update(path, func(s *store.State) error {
		lead, err := requireLead(s, self)
		if err != nil {
			return err
		}
		if _, err := s.Find(lead.Team, name); err == nil {
			return fmt.Errorf("an agent named %s already exists", name)
		}
		root = lead.Dir
		a.Team = lead.Team
		switch {
		case args["dir"] != "":
			dir, err := workspace.Resolve(root, args["dir"])
			if err != nil {
				return err
			}
			a.Dir = dir
		case s.NoGit:
			return errors.New("dir is required: this workspace is not a Git checkout, so workers cannot have worktrees")
		default:
			workerName := a.Team + "/" + name
			a.Dir, a.Branch = workspace.WorktreeDir(root, workerName), workspace.WorkerBranch(workerName)
		}
		s.Agents = append(s.Agents, a)
		if args["message"] != "" {
			s.Messages = append(s.Messages, store.Message{From: self, To: a.ID, Text: args["message"]})
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	worktreeCreated := false
	var reply client.Reply
	window, err := teamWindow(path, a.Team)
	if err == nil && a.Branch != "" {
		err = workspace.AddWorktree(root, a.Dir, a.Branch)
		worktreeCreated = err == nil
	}
	if err == nil {
		reply, err = harness.Launch(a, path, Nudge, "", window)
	}
	if err != nil {
		cause := err
		if closeErr := mux.Kill(reply.Pane); closeErr != nil {
			return "", fmt.Errorf("%w; closing the pane failed: %v; agent %s was kept for cleanup", cause, closeErr, a.Name)
		}
		if worktreeCreated {
			if cleanup := workspace.RemoveWorktree(root, a.Dir, a.Branch); cleanup != nil {
				return "", fmt.Errorf("%w; worktree cleanup failed: %v; agent %s was kept for cleanup", cause, cleanup, a.Name)
			}
		}
		if cleanup := store.Update(path, func(s *store.State) error { s.Remove(a.ID); return nil }); cleanup != nil {
			return "", fmt.Errorf("%w; removing the agent failed: %v", cause, cleanup)
		}
		return "", cause
	}

	if a.Branch == "" {
		return fmt.Sprintf("Created %s in %s.", a.Address(), a.Dir), nil
	}
	return fmt.Sprintf("Created %s on branch %s in %s.", a.Address(), a.Branch, a.Dir), nil
}

// teamWindow returns the mux window of a running team.
func teamWindow(path, team string) (string, error) {
	s, err := store.Load(path)
	if err != nil {
		return "", err
	}
	window, err := mux.Team(s, team)
	if err == nil && window == "" {
		err = errors.New("the session is not running")
	}
	return window, err
}

func removeAgent(self, path, name string) (string, error) {
	// A misnamed argument arrives as an empty name; failing lets the model retry.
	if strings.TrimSpace(name) == "" {
		return "", errors.New("name is required")
	}
	lock, err := store.LockFile(path + ".session.lock")
	if err != nil {
		return "", err
	}
	defer lock.Close()
	s, err := store.Load(path)
	if err != nil {
		return "", err
	}
	lead, err := requireLead(&s, self)
	if err != nil {
		return "", err
	}
	a, err := s.Find(lead.Team, name)
	if err != nil {
		return "", err
	}
	if a.Team != lead.Team {
		return "", errors.New("only workers in your session can be removed")
	}
	if a.Role != store.Worker {
		return "", errors.New("only workers can be removed")
	}
	if a.Branch != "" {
		if err := workspace.RemoveWorktree(lead.Dir, a.Dir, a.Branch); err != nil {
			return "", err
		}
	}
	if err := mux.Kill(a.Target); err != nil {
		return "", fmt.Errorf("closing %s: %w", a.Name, err)
	}
	err = store.Update(path, func(s *store.State) error { s.Remove(a.ID); return nil })
	return fmt.Sprintf("Removed %s.", a.Address()), err
}
