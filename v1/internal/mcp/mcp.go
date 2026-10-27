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

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/tmux"
	"github.com/kkestell/hypercube/internal/workspace"
)

// Nudge is typed into an idle agent's pane to make it run a turn.
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

const dirArg = "an existing directory given as an absolute path, a path starting with ~/, or a path relative to the orchestrator's directory, the worker works directly in it"

const profileArg = "The profile sets the worker's harness, model, and effort. An optional message is queued for the worker before it starts."

// toolsFor returns the tools for the workspace's mode and the configured
// profiles. Outside Git, workers cannot have worktrees, so create_agent
// requires dir.
func toolsFor(s store.State, profiles []harness.Profile) []tool {
	create := tool{"create_agent", "Orchestrator only. Create a worker and launch it. Without dir, the worker gets its own branch and worktree from HEAD. With dir, " + dirArg + ", and Hypercube creates no branch or worktree. " + profileArg,
		schema([]string{"name", "profile"}, "name", "profile", "dir", "message")}
	remove := tool{"remove_agent", "Orchestrator only. Close a worker's pane and remove it from the team. A worker with a worktree also has its worktree and branch removed, and Git refuses dirty worktrees and unmerged branches. A worker without one has its directory left as it is.", schema([]string{"id"}, "id")}
	if s.NoGit {
		create = tool{create.Name, "Orchestrator only. Create a worker and launch it. Given dir, " + dirArg + ". " + profileArg,
			schema([]string{"name", "profile", "dir"}, "name", "profile", "dir", "message")}
		remove.Description = "Orchestrator only. Close a worker's pane and remove it from the team. Its directory is left as it is."
	}
	if len(profiles) > 0 {
		var names []string
		for _, p := range profiles {
			names = append(names, p.Name)
		}
		create.InputSchema["properties"].(map[string]any)["profile"] = map[string]any{"type": "string", "enum": names}
	}
	return []tool{
		{"send_message", "Send a message to another agent by ID or name.", schema([]string{"to", "text"}, "to", "text")},
		{"list_agents", "List the team: id, name, role, harness, activity, directory, and branch.", schema([]string{})},
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
			json.Unmarshal(req.Params, &p)
			text, err := call(id, path, p.Name, p.Arguments)
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
		type entry struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Role     string `json:"role"`
			Harness  string `json:"harness"`
			Activity string `json:"activity"`
			Dir      string `json:"directory"`
			Branch   string `json:"branch"`
		}
		var team []entry
		for _, a := range s.Agents {
			team = append(team, entry{a.ID, a.Name, a.Role, a.Harness, a.Activity, a.Dir, a.Branch})
		}
		b, err := json.MarshalIndent(team, "", "  ")
		return string(b), err
	case "create_agent":
		return createAgent(self, path, args)
	case "remove_agent":
		return removeAgent(self, path, args["id"])
	}
	return "", fmt.Errorf("unknown tool %q", name)
}

func sendMessage(self, path, to, text string) (string, error) {
	var target string
	var recipient store.Agent
	err := store.Update(path, func(s *store.State) error {
		a, err := s.Agent(to)
		if err != nil {
			return err
		}
		if a.ID == self {
			return errors.New("cannot send a message to yourself")
		}
		recipient = *a
		s.Messages = append(s.Messages, store.Message{From: self, To: a.ID, Text: text})
		if a.Activity == store.Idle {
			target = a.Target
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if target != "" {
		if err := tmux.SendKeys(target, Nudge); err != nil {
			return "", fmt.Errorf("message queued, but waking %s failed: %w", recipient.Name, err)
		}
	}
	return fmt.Sprintf("Message queued for %s (%s).", recipient.Name, recipient.ID), nil
}

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func orchestrator(s *store.State, self string) (*store.Agent, error) {
	a, err := s.Agent(self)
	if err == nil && a.Role != store.Orchestrator {
		err = errors.New("only the orchestrator can do this")
	}
	return a, err
}

func createAgent(self, path string, args map[string]string) (string, error) {
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
		orch, err := orchestrator(s, self)
		if err != nil {
			return err
		}
		if _, err := s.Agent(name); err == nil {
			return fmt.Errorf("an agent named %s already exists", name)
		}
		root = orch.Dir
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
			a.Dir, a.Branch = workspace.WorktreeDir(root, name), workspace.WorkerBranch(name)
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
	if a.Branch != "" {
		err = workspace.AddWorktree(root, a.Dir, a.Branch)
	}
	if err == nil {
		if a.Target, err = harness.Launch(a, path, Nudge); err != nil && a.Branch != "" {
			workspace.RemoveWorktree(root, a.Dir, a.Branch)
		}
	}
	if err != nil {
		store.Update(path, func(s *store.State) error { s.Remove(a.ID); return nil })
		return "", err
	}
	err = store.Update(path, func(s *store.State) error {
		w, err := s.Agent(a.ID)
		if err == nil {
			w.Target = a.Target
		}
		return err
	})
	if err != nil {
		return "", err
	}
	if a.Branch == "" {
		return fmt.Sprintf("Created %s (%s) in %s.", a.Name, a.ID, a.Dir), nil
	}
	return fmt.Sprintf("Created %s (%s) on branch %s in %s.", a.Name, a.ID, a.Branch, a.Dir), nil
}

func removeAgent(self, path, id string) (string, error) {
	s, err := store.Load(path)
	if err != nil {
		return "", err
	}
	orch, err := orchestrator(&s, self)
	if err != nil {
		return "", err
	}
	a, err := s.Agent(id)
	if err != nil {
		return "", err
	}
	if a.Role != store.Worker {
		return "", errors.New("only workers can be removed")
	}
	if a.Branch != "" {
		if err := workspace.RemoveWorktree(orch.Dir, a.Dir, a.Branch); err != nil {
			return "", err
		}
	}
	tmux.KillPane(store.Workspace(path), a.Target)
	err = store.Update(path, func(s *store.State) error { s.Remove(a.ID); return nil })
	return fmt.Sprintf("Removed %s (%s).", a.Name, a.ID), err
}
