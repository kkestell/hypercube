// Command hypercube-harness-scripted plays a scripted scene for the Linux
// desktop tests. They install it under a real harness name, so the app runs
// it in place of the native harness. It reads the scene file named by
// HYPERCUBE_SCENE and answers each input with the turn whose prompt the input
// contains: a child's brief arrives wrapped in its parent's message. A turn
// may start child sessions and offer tasks through Hypercube's create_session
// tool, and a parent ends the turn that delivers its children's reports at
// once.
//
// Its `describe` and `models` commands answer from the scene: the harness is
// the one the plugin's file is installed as, and its models are those the
// scene's sessions use with it.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/report"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Ends of a turn: it finishes, stays open, or waits on its last item, an open
// attention request.
const (
	endIdle    = "idle"
	endWorking = "working"
	endAsk     = "ask"
)

// The parts of a scene file the plugin plays.
type scene struct {
	Workspaces []struct {
		Sessions []session `json:"sessions"`
	} `json:"workspaces"`
}

type session struct {
	Title    string           `json:"title"`
	Name     string           `json:"name"`
	Settings harness.Settings `json:"settings"`
	Dir      string           `json:"dir"`
	Turns    []turn           `json:"turns"`
}

type turn struct {
	Prompt   string         `json:"prompt"`
	Items    []harness.Item `json:"items"`
	End      string         `json:"end"`
	Usage    *harness.Usage `json:"usage"`
	Limits   []limit        `json:"limits"`
	Children []session      `json:"children"`
	// Offers are the titles of tasks the turn offers the user as new sessions.
	Offers      []string  `json:"offers"`
	Suggestions []session `json:"suggestions"`
	Git         []struct {
		Args    []string `json:"args"`
		File    string   `json:"file"`
		Content string   `json:"content"`
	} `json:"git"`
}

// limit is a usage window's reading, reported as the turn ends: Left is the
// minutes until the window resets.
type limit struct {
	Minutes int `json:"minutes"`
	Used    int `json:"used"`
	Left    int `json:"left"`
}

func main() {
	var err error
	switch {
	case len(os.Args) == 2 && os.Args[1] == "serve":
		err = serve(os.Stdin, os.Stdout)
	case len(os.Args) == 2 && os.Args[1] == "describe":
		err = json.NewEncoder(os.Stdout).Encode(harness.Description{Name: names[harnessID()], Installed: true})
	case len(os.Args) == 2 && os.Args[1] == "models":
		err = models(os.Stdin, os.Stdout)
	default:
		err = fmt.Errorf("usage: %s serve | describe | models", os.Args[0])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hypercube-harness-scripted:", err)
		os.Exit(1)
	}
}

// names are the display names of the harnesses the scenes use.
var names = map[string]string{"claude": "Claude Code", "opencode": "OpenCode", "pi": "Pi"}

// harnessID is the harness the plugin is installed as.
func harnessID() string {
	return strings.TrimPrefix(filepath.Base(os.Args[0]), "hypercube-harness-")
}

// models lists the models the scene's sessions use with this harness, the
// first as the default, and a requested model's efforts.
func models(in io.Reader, out io.Writer) error {
	var req harness.ModelsRequest
	if err := json.NewDecoder(in).Decode(&req); err != nil {
		return err
	}
	sessions, _, err := load(os.Getenv("HYPERCUBE_SCENE"))
	if err != nil {
		return err
	}
	c := harness.Catalog{Model: req.Model}
	for _, s := range sessions {
		if s.Harness != harnessID() {
			continue
		}
		if !slices.ContainsFunc(c.Models, func(m harness.Model) bool { return m.ID == s.Model }) {
			c.Models = append(c.Models, harness.Model{ID: s.Model})
		}
		if s.Model == req.Model && s.Effort != "" && !slices.Contains(c.Efforts, s.Effort) {
			c.Efforts = append(c.Efforts, s.Effort)
		}
	}
	if len(c.Models) > 0 {
		c.Default = c.Models[0].ID
	}
	return json.NewEncoder(out).Encode(c)
}

func serve(in io.Reader, out io.Writer) error {
	_, turns, err := load(os.Getenv("HYPERCUBE_SCENE"))
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	send := func(r harness.Record) error { return enc.Encode(r) }
	reply := func(id int64) error { return send(harness.Record{Reply: &harness.Reply{ID: id}}) }
	event := func(ev harness.Event) error { return send(harness.Record{Event: &ev}) }

	var start harness.StartArgs
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1<<20), 64<<20)
	var last int64
	for scanner.Scan() {
		var cmd harness.Command
		if err := json.Unmarshal(scanner.Bytes(), &cmd); err != nil {
			return err
		}
		if cmd.ID <= last {
			continue
		}
		last = cmd.ID
		if err := reply(cmd.ID); err != nil {
			return err
		}
		if cmd.Command == harness.CmdStart {
			start = *cmd.Start
		}
		if cmd.Command != harness.CmdInput {
			continue
		}
		t, ok := match(turns, cmd.Text)
		switch {
		case strings.HasPrefix(cmd.Text, report.ReportsReady):
			// An idle parent of idle children receives their reports; a
			// scene's parent has nothing to say about them.
			if err = event(harness.Event{State: harness.Working}); err == nil {
				err = event(harness.Event{State: harness.Idle})
			}
		case !ok:
			err = event(harness.Event{State: harness.Idle, Items: []harness.Item{{ID: fmt.Sprintf("unmatched-%d", cmd.ID),
				Kind: harness.KindError, Text: "The scene has no turn for this input.", Done: true}}})
		default:
			err = play(t, start, event)
		}
		if err != nil {
			return err
		}
	}
	return scanner.Err()
}

// load returns the settings of every session in the scene, suggestions
// included, and every turn of top-level sessions and children.
func load(path string) ([]harness.Settings, []turn, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var s scene
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	var settings []harness.Settings
	var turns []turn
	var add func([]session)
	add = func(sessions []session) {
		for _, s := range sessions {
			settings = append(settings, s.Settings)
			for _, t := range s.Turns {
				turns = append(turns, t)
				add(t.Children)
				for _, suggested := range t.Suggestions {
					settings = append(settings, suggested.Settings)
				}
			}
		}
	}
	for _, w := range s.Workspaces {
		add(w.Sessions)
	}
	return settings, turns, nil
}

func match(turns []turn, text string) (turn, bool) {
	for _, t := range turns {
		if strings.Contains(text, t.Prompt) {
			return t, true
		}
	}
	return turn{}, false
}

// play sends a turn: working, its children and offers, its items, and its end
// with its usage and limits. The last item arrives with the end, so its arrival
// means the turn has played. A
// child that cannot start ends the turn with the reason.
func play(t turn, start harness.StartArgs, event func(harness.Event) error) error {
	if err := event(harness.Event{State: harness.Working}); err != nil {
		return err
	}
	for _, step := range t.Git {
		if step.File != "" {
			if err := os.WriteFile(filepath.Join(start.Dir, step.File), []byte(step.Content), 0644); err != nil {
				return err
			}
		}
		if len(step.Args) > 0 {
			cmd := exec.Command("git", append([]string{"-c", "user.name=Hypercube", "-c", "user.email=hypercube@example.com", "-c", "commit.gpgsign=false"}, step.Args...)...)
			cmd.Dir = start.Dir
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("git: %s: %w", out, err)
			}
		}
	}
	if len(t.Children) > 0 || len(t.Offers) > 0 || len(t.Suggestions) > 0 {
		if err := createSessions(start, t); err != nil {
			return event(harness.Event{State: harness.Idle, Items: []harness.Item{{ID: "children", Kind: harness.KindError, Text: err.Error(), Done: true}}})
		}
	}
	n := len(t.Items) - 1
	if n > 0 {
		if err := event(harness.Event{Items: t.Items[:n]}); err != nil {
			return err
		}
	}
	end := harness.Event{Items: t.Items[n:], Usage: t.Usage}
	now := time.Now()
	for _, l := range t.Limits {
		end.Limits = append(end.Limits, harness.Limit{Minutes: l.Minutes, Used: l.Used, Resets: now.Add(time.Duration(l.Left) * time.Minute).Unix(), At: now.UnixMilli()})
	}
	if t.End == endIdle {
		end.State = harness.Idle
		for i := n; i >= 0; i-- {
			if t.Items[i].Kind == harness.KindText {
				end.Final = []string{t.Items[i].ID}
				break
			}
		}
	}
	return event(end)
}

// childCall is the create_session arguments of a child or suggestion.
func childCall(c session, initiation string) map[string]any {
	return map[string]any{"title": c.Title, "name": c.Name, "harness": c.Settings.Harness, "model": c.Settings.Model, "effort": c.Settings.Effort,
		"dir": c.Dir, "initiation": initiation, "message": c.Turns[0].Prompt}
}

// createSessions starts a turn's children and makes its offers through the
// session's own Hypercube tools.
func createSessions(start harness.StartArgs, t turn) error {
	ctx := context.Background()
	if start.MCP == "" {
		return errors.New("the start names no MCP server")
	}
	cmd := exec.Command(start.MCP)
	cmd.Env = os.Environ()
	for k, v := range start.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "hypercube-harness-scripted", Version: "0.1.0"}, nil)
	conn, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	var calls []map[string]any
	for _, c := range t.Children {
		calls = append(calls, childCall(c, "user_asked"))
	}
	for _, title := range t.Offers {
		calls = append(calls, map[string]any{"title": title, "initiation": "offer"})
	}
	for _, c := range t.Suggestions {
		calls = append(calls, childCall(c, "own_initiative"))
	}
	for _, args := range calls {
		result, err := conn.CallTool(ctx, &mcp.CallToolParams{Name: "create_session", Arguments: args})
		if err != nil {
			return err
		}
		if result.IsError {
			return fmt.Errorf("creating %s: %v", args["title"], result.Content)
		}
	}
	return nil
}
