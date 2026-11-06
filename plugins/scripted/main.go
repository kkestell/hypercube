// Command hypercube-harness-scripted plays a documentation screenshot scene.
// `make screenshots` installs it under each real harness name, so the app runs
// it in place of the native harnesses. It reads the scene file named by
// HYPERCUBE_SCENE and answers each input with the turn whose prompt the input
// contains: a child's brief arrives wrapped in its parent's message. A turn
// may start child sessions and offer tasks through Hypercube's create_session
// tool, and a parent ends the turn that delivers its children's reports at
// once.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

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

// The parts of a scene file the plugin plays. The screenshot driver reads the
// rest.
type scene struct {
	Workspaces []struct {
		Sessions []session `json:"sessions"`
	} `json:"workspaces"`
}

type session struct {
	Title   string `json:"title"`
	Name    string `json:"name"`
	Profile string `json:"profile"`
	Dir     string `json:"dir"`
	Turns   []turn `json:"turns"`
}

type turn struct {
	Prompt   string         `json:"prompt"`
	Items    []harness.Item `json:"items"`
	End      string         `json:"end"`
	Usage    *harness.Usage `json:"usage"`
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

func main() {
	if len(os.Args) != 2 || os.Args[1] != "serve" {
		fmt.Fprintln(os.Stderr, "usage: hypercube-harness-scripted serve")
		os.Exit(1)
	}
	if err := serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "hypercube-harness-scripted:", err)
		os.Exit(1)
	}
}

func serve(in io.Reader, out io.Writer) error {
	turns, err := load(os.Getenv("HYPERCUBE_SCENE"))
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

// load returns every turn of the scene, of top-level sessions and children.
func load(path string) ([]turn, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s scene
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var turns []turn
	var add func([]session)
	add = func(sessions []session) {
		for _, s := range sessions {
			for _, t := range s.Turns {
				turns = append(turns, t)
				add(t.Children)
			}
		}
	}
	for _, w := range s.Workspaces {
		add(w.Sessions)
	}
	return turns, nil
}

func match(turns []turn, text string) (turn, bool) {
	for _, t := range turns {
		if strings.Contains(text, t.Prompt) {
			return t, true
		}
	}
	return turn{}, false
}

// play sends a turn: working, its children and offers, its items, and its end. The last
// item arrives with the end, so its arrival means the turn has played. A
// child that cannot start ends the turn with the reason.
func play(t turn, start harness.StartArgs, event func(harness.Event) error) error {
	if err := event(harness.Event{State: harness.Working}); err != nil {
		return err
	}
	for _, step := range t.Git {
		if step.File != "" {
			if err := os.WriteFile(start.Dir+"/"+step.File, []byte(step.Content), 0644); err != nil {
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

// createSessions starts a turn's children and makes its offers through the
// session's own Hypercube tools.
func createSessions(start harness.StartArgs, t turn) error {
	ctx := context.Background()
	cmd := exec.Command(start.Hypercube, "mcp")
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
		calls = append(calls, map[string]any{"title": c.Title, "name": c.Name, "profile": c.Profile, "dir": c.Dir, "initiation": "user_asked", "message": c.Turns[0].Prompt})
	}
	for _, title := range t.Offers {
		calls = append(calls, map[string]any{"title": title, "initiation": "offer"})
	}
	for _, c := range t.Suggestions {
		calls = append(calls, map[string]any{"title": c.Title, "name": c.Name, "profile": c.Profile, "dir": c.Dir, "initiation": "own_initiative", "message": c.Turns[0].Prompt})
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
