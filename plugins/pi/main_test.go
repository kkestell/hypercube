package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
)

// The fixtures in testdata are pi's own output, recorded from
// `pi --mode rpc` and trimmed of its system prompt, the messages of agent_end,
// signatures of reasoning, and the updates of tools and tool call arguments.

func fixture(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// piUnderTest is an agent whose commands go to a list instead of a process.
// Pi accepts every command unless fail names an error for its type.
type piUnderTest struct {
	*agent
	mu     sync.Mutex
	sent   []map[string]any
	events []harness.Event
	fail   map[string]string
}

func newPi() *piUnderTest {
	p := &piUnderTest{fail: map[string]string{}}
	p.agent = &agent{
		stdin: p, emit: func(ev harness.Event) {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.events = append(p.events, ev)
		},
		ended: make(chan struct{}), calls: map[string]chan response{},
		blocks: map[string]*block{}, tools: map[string]harness.Item{}, requests: map[string]*request{},
		session: "session-1", window: 1000,
	}
	p.agent.ctx, p.agent.cancel = context.WithCancel(context.Background())
	return p
}

func (p *piUnderTest) Write(b []byte) (int, error) {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return 0, err
	}
	p.mu.Lock()
	p.sent = append(p.sent, m)
	p.mu.Unlock()
	if id, ok := m["id"].(string); ok && m["type"] != "extension_ui_response" {
		r := map[string]any{"id": id, "type": "response", "command": m["type"], "success": true}
		if e := p.fail[m["type"].(string)]; e != "" {
			r["success"], r["error"] = false, e
		}
		b, _ := json.Marshal(r)
		go p.line(b) // Pi's output is read on its own goroutine
	}
	return len(b), nil
}

func (p *piUnderTest) feed(lines ...string) {
	for _, line := range lines {
		p.line([]byte(line))
	}
}

// until returns the lines up to and including the first that contains want,
// and the lines after it.
func until(t *testing.T, lines []string, want string) (head, rest []string) {
	t.Helper()
	for i, line := range lines {
		if strings.Contains(line, want) {
			return lines[:i+1], lines[i+1:]
		}
	}
	t.Fatalf("no line contains %s", want)
	return
}

func (p *piUnderTest) snapshot() []harness.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.events)
}

// transcript applies events the way Hypercube does: a later item with the same
// ID replaces the earlier one, or extends its text.
func (p *piUnderTest) transcript() []harness.Item {
	var items []harness.Item
	for _, ev := range p.snapshot() {
		for _, it := range ev.Items {
			i := slices.IndexFunc(items, func(o harness.Item) bool { return o.ID == it.ID })
			switch {
			case i < 0:
				items = append(items, it)
			case it.Append && !items[i].Done:
				it.Text, it.Append = items[i].Text+it.Text, false
				items[i] = it
			default:
				items[i] = it
			}
		}
	}
	return items
}

func (p *piUnderTest) item(t *testing.T, kind string) harness.Item {
	t.Helper()
	for _, it := range p.transcript() {
		if it.Kind == kind {
			return it
		}
	}
	t.Fatalf("no %s item in %+v", kind, p.transcript())
	return harness.Item{}
}

func (p *piUnderTest) states() []string {
	var states []string
	for _, ev := range p.snapshot() {
		if ev.State != "" {
			states = append(states, ev.State)
		}
	}
	return states
}

// finals are the response items reported with each idle state.
func (p *piUnderTest) finals() [][]string {
	var finals [][]string
	for _, ev := range p.snapshot() {
		if ev.State == harness.Idle {
			finals = append(finals, ev.Final)
		}
	}
	return finals
}

func (p *piUnderTest) conversations() []string {
	var ids []string
	for _, ev := range p.snapshot() {
		if ev.Conversation != "" {
			ids = append(ids, ev.Conversation)
		}
	}
	return ids
}

func (p *piUnderTest) lastUsage() harness.Usage {
	for _, ev := range slices.Backward(p.snapshot()) {
		if ev.Usage != nil {
			return *ev.Usage
		}
	}
	return harness.Usage{}
}

// commands returns what was sent to pi with the given type.
func (p *piUnderTest) commands(kind string) []map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []map[string]any
	for _, m := range p.sent {
		if m["type"] == kind {
			out = append(out, m)
		}
	}
	return out
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for range 300 {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestTextAndToolCall(t *testing.T) {
	p := newPi()
	p.feed(fixture(t, "tool")...)
	var kinds []string
	for _, it := range p.transcript() {
		kinds = append(kinds, it.Kind)
		if !it.Done {
			t.Errorf("item %s is not done", it.ID)
		}
	}
	// The whitespace before the tool call is not shown.
	if !slices.Equal(kinds, []string{harness.KindTool, harness.KindText}) {
		t.Fatalf("kinds %v in %+v", kinds, p.transcript())
	}
	tool := p.item(t, harness.KindTool)
	if tool.Tool != "bash" || !strings.Contains(string(tool.Input), "echo hi") || tool.Output != "hi\n" || tool.Failed {
		t.Errorf("tool %+v", tool)
	}
	if text := p.item(t, harness.KindText); text.Text != "done" {
		t.Errorf("text %q", text.Text)
	}
	if got := p.states(); !slices.Equal(got, []string{harness.Working, harness.Idle}) {
		t.Errorf("states %v", got)
	}
	// Pi saves the session from its first message, and the ID is reported then.
	if got := p.conversations(); !slices.Equal(got, []string{"session-1"}) {
		t.Errorf("conversations %v", got)
	}
	if u := p.lastUsage(); u.Context != 3621 || u.Window != 1000 || u.Cost <= 0.00005 {
		t.Errorf("usage %+v", u)
	}
}

func TestWhitespaceBlockIsNotShown(t *testing.T) {
	p := newPi()
	// The model's first message streams a block of whitespace before its tool call.
	head, _ := until(t, fixture(t, "tool"), `"role":"assistant","content":[{"type":"text","text":"\n\n"}`)
	p.feed(head...)
	for _, ev := range p.snapshot() {
		for _, it := range ev.Items {
			if it.Kind == harness.KindText {
				t.Errorf("whitespace was shown: %+v", it)
			}
		}
	}
	if got := p.states(); !slices.Equal(got, []string{harness.Working}) {
		t.Errorf("states %v", got)
	}
}

func TestReasoningStreamsAndCompletes(t *testing.T) {
	p := newPi()
	p.feed(fixture(t, "editor")...)
	var reasoning harness.Item
	for _, it := range p.transcript() {
		if it.Kind == harness.KindReasoning {
			reasoning = it
		}
	}
	if reasoning.ID == "" || !reasoning.Done || reasoning.Text == "" {
		t.Fatalf("reasoning %+v", reasoning)
	}
	// Each streamed delta extends one item.
	var parts int
	for _, ev := range p.snapshot() {
		for _, it := range ev.Items {
			if it.ID == reasoning.ID && it.Append && !it.Done {
				parts++
			}
		}
	}
	if parts == 0 {
		t.Errorf("reasoning did not stream: %+v", p.snapshot())
	}
}

func TestFailedRunIsAnError(t *testing.T) {
	p := newPi()
	p.feed(fixture(t, "error")...)
	e := p.item(t, harness.KindError)
	if !strings.Contains(e.Text, "401") || !e.Done {
		t.Errorf("error %+v", e)
	}
	if got := p.states(); !slices.Equal(got, []string{harness.Working, harness.Idle}) {
		t.Errorf("states %v", got)
	}
}

func TestInterruptedRunIsNotAnError(t *testing.T) {
	p := newPi()
	p.feed(fixture(t, "interrupt")...)
	for _, it := range p.transcript() {
		if it.Kind == harness.KindError {
			t.Errorf("error %+v", it)
		}
	}
	if tool := p.item(t, harness.KindTool); !tool.Failed || tool.Output != "Command aborted" {
		t.Errorf("tool %+v", tool)
	}
	if got := p.states(); !slices.Equal(got, []string{harness.Working, harness.Idle}) {
		t.Errorf("states %v", got)
	}
}

func TestRetriedErrorIsNotShown(t *testing.T) {
	p := newPi()
	failed, _ := until(t, fixture(t, "error"), `"turn_end"`)
	_, retry := until(t, fixture(t, "tool"), `"turn_end"`)
	// Pi retries a failed request within the same run.
	p.feed(failed...)
	p.feed(retry...)
	for _, it := range p.transcript() {
		if it.Kind == harness.KindError {
			t.Errorf("error %+v", it)
		}
	}
	if got := p.states(); !slices.Equal(got, []string{harness.Working, harness.Idle}) {
		t.Errorf("states %v", got)
	}
}

func TestSelectDialog(t *testing.T) {
	p := newPi()
	head, _ := until(t, fixture(t, "select"), `"extension_ui_request"`)
	p.feed(head...)
	a := p.item(t, harness.KindAttention)
	if a.Text != "Pick a color" || !slices.Equal(a.Choices, []string{"red", "green"}) || a.Free || a.Done {
		t.Fatalf("attention %+v", a)
	}
	if err := p.answer(a.ID, "purple", ""); err == nil {
		t.Error("an unknown choice was accepted")
	}
	if err := p.answer(a.ID, "green", ""); err != nil {
		t.Fatal(err)
	}
	r := p.commands("extension_ui_response")
	if len(r) != 1 || r[0]["id"] != a.ID || r[0]["value"] != "green" {
		t.Errorf("responses %v", r)
	}
	if done := p.item(t, harness.KindAttention); !done.Done || done.Answer != "green" {
		t.Errorf("attention %+v", done)
	}
	if err := p.answer(a.ID, "green", ""); err == nil {
		t.Error("an answer to a finished request succeeded")
	}
}

func TestConfirmDialog(t *testing.T) {
	for _, tc := range []struct {
		choice string
		want   bool
	}{{"Yes", true}, {"No", false}} {
		p := newPi()
		head, _ := until(t, fixture(t, "confirm"), `"extension_ui_request"`)
		p.feed(head...)
		a := p.item(t, harness.KindAttention)
		if a.Text != "Delete it?\n\nThis cannot be undone." || !slices.Equal(a.Choices, []string{"Yes", "No"}) {
			t.Fatalf("attention %+v", a)
		}
		if err := p.answer(a.ID, tc.choice, ""); err != nil {
			t.Fatal(err)
		}
		if r := p.commands("extension_ui_response"); len(r) != 1 || r[0]["confirmed"] != tc.want {
			t.Errorf("%s: responses %v", tc.choice, r)
		}
	}
}

func TestInputAndEditorTakeText(t *testing.T) {
	for _, tc := range []struct{ name, text string }{{"input", "Your name?"}, {"editor", "Edit this\n\nLine 1\nLine 2"}} {
		p := newPi()
		head, _ := until(t, fixture(t, tc.name), `"extension_ui_request"`)
		p.feed(head...)
		a := p.item(t, harness.KindAttention)
		if a.Text != tc.text || !a.Free || len(a.Choices) != 0 {
			t.Fatalf("%s: attention %+v", tc.name, a)
		}
		if err := p.answer(a.ID, "", ""); err == nil {
			t.Errorf("%s: an empty answer was accepted", tc.name)
		}
		if err := p.answer(a.ID, "", "Kyle"); err != nil {
			t.Fatal(err)
		}
		if r := p.commands("extension_ui_response"); len(r) != 1 || r[0]["value"] != "Kyle" {
			t.Errorf("%s: responses %v", tc.name, r)
		}
	}
}

func TestDialogEndsWhenPiGivesUp(t *testing.T) {
	p := newPi()
	head, _ := until(t, fixture(t, "timeout"), `"extension_ui_request"`)
	p.feed(head...)
	id := p.item(t, harness.KindAttention).ID
	eventually(t, "the dialog to end", func() bool {
		p.agent.mu.Lock()
		defer p.agent.mu.Unlock()
		return len(p.agent.requests) == 0
	})
	if a := p.item(t, harness.KindAttention); a.ID != id || !a.Done {
		t.Errorf("attention %+v", a)
	}
	if err := p.answer(id, "red", ""); err == nil {
		t.Error("an answer to an expired dialog succeeded")
	}
}

func TestErrorNotificationIsShown(t *testing.T) {
	p := newPi()
	p.feed(fixture(t, "notify")...)
	if e := p.item(t, harness.KindError); e.Text != "Disk is full" {
		t.Errorf("error %+v", e)
	}
	for _, it := range p.transcript() {
		if it.Kind == harness.KindAttention {
			t.Errorf("attention %+v", it)
		}
	}
}

func TestOtherInterfaceRequestsAreIgnored(t *testing.T) {
	p := newPi()
	p.feed(
		`{"type":"extension_ui_request","id":"u1","method":"notify","message":"fyi","notifyType":"info"}`,
		`{"type":"extension_ui_request","id":"u2","method":"setStatus","statusKey":"k","statusText":"busy"}`,
		`{"type":"extension_ui_request","id":"u3","method":"setWidget","widgetKey":"k","widgetLines":["a"]}`,
	)
	if got := p.snapshot(); len(got) != 0 {
		t.Errorf("events %+v", got)
	}
}

func TestExtensionErrorIsShown(t *testing.T) {
	p := newPi()
	p.feed(`{"type":"extension_error","extensionPath":"/x.ts","event":"tool_call","error":"boom"}`)
	if e := p.item(t, harness.KindError); !strings.Contains(e.Text, "boom") {
		t.Errorf("error %+v", e)
	}
}

// Stopping the agent gives up a call that pi has not answered.
func TestStopCancelsPendingCall(t *testing.T) {
	pending := filepath.Join(t.TempDir(), "pending")
	fakePi(t, map[string]string{"FAKE_PENDING": pending})
	ag, _ := start(t, startArgs(t))
	done := make(chan error, 1)
	go func() { done <- ag.input("hello", nil) }()
	eventually(t, "Pi to receive the unanswered prompt", func() bool {
		_, err := os.Stat(pending)
		return err == nil
	})
	stopped := make(chan struct{})
	go func() { ag.stop(); close(stopped) }()
	defer func() { os.Remove(pending); <-stopped }()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("input returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("input is still waiting after stop")
	}
}

func TestInterruptGivesUpDialogsAndAborts(t *testing.T) {
	p := newPi()
	head, _ := until(t, fixture(t, "select"), `"extension_ui_request"`)
	p.feed(head...)
	id := p.item(t, harness.KindAttention).ID
	if err := p.interrupt(); err != nil {
		t.Fatal(err)
	}
	r := p.commands("extension_ui_response")
	if len(r) != 1 || r[0]["id"] != id || r[0]["cancelled"] != true {
		t.Errorf("responses %v", r)
	}
	if len(p.commands("abort")) != 1 {
		t.Errorf("sent %v", p.sent)
	}
	if a := p.item(t, harness.KindAttention); !a.Done {
		t.Errorf("attention %+v", a)
	}
	if err := p.answer(id, "red", ""); err == nil {
		t.Error("an answer to a cancelled dialog succeeded")
	}
}

func TestInputPromptsAndSteers(t *testing.T) {
	p := newPi()
	if err := p.input("hello", nil); err != nil {
		t.Fatal(err)
	}
	c := p.commands("prompt")
	// Pi starts a turn, or steers the one in progress, as it finds itself.
	if len(c) != 1 || c[0]["message"] != "hello" || c[0]["streamingBehavior"] != "steer" {
		t.Errorf("sent %v", c)
	}
}

func TestInputSendsImages(t *testing.T) {
	p := newPi()
	if err := p.input("", []harness.Image{{MIMEType: "image/png", Data: "AAAA"}}); err != nil {
		t.Fatal(err)
	}
	c := p.commands("prompt")
	if len(c) != 1 || c[0]["message"] != "" || c[0]["streamingBehavior"] != "steer" {
		t.Fatalf("sent %v", c)
	}
	want := []any{map[string]any{"type": "image", "data": "AAAA", "mimeType": "image/png"}}
	if !reflect.DeepEqual(c[0]["images"], want) {
		t.Errorf("images %v", c[0]["images"])
	}
	if err := p.input("hello", nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.commands("prompt")[1]["images"]; ok {
		t.Error("sent images for a text input")
	}
}

func TestToolResultImages(t *testing.T) {
	p := newPi()
	p.feed(
		`{"type":"tool_execution_start","toolCallId":"t1","toolName":"read","args":{}}`,
		`{"type":"tool_execution_end","toolCallId":"t1","toolName":"read","result":{"content":[{"type":"text","text":"shown"},{"type":"image","data":"AAAA","mimeType":"image/png"}]},"isError":false}`,
	)
	tool := p.item(t, harness.KindTool)
	if tool.Output != "shown" || !tool.Done || len(tool.Images) != 1 || tool.Images[0] != (harness.Image{MIMEType: "image/png", Data: "AAAA"}) {
		t.Errorf("tool %+v", tool)
	}
}

func TestRejectedInputIsAnError(t *testing.T) {
	p := newPi()
	p.fail["prompt"] = "No API key found"
	if err := p.input("hello", nil); err == nil || !strings.Contains(err.Error(), "No API key found") {
		t.Errorf("error %v", err)
	}
}

func TestUnreadableLineIsSkipped(t *testing.T) {
	p := newPi()
	p.feed("not json", `{"type":"agent_start"}`)
	if got := p.states(); !slices.Equal(got, []string{harness.Working}) {
		t.Errorf("states %v", got)
	}
}

func TestInputWhenNotRunning(t *testing.T) {
	p := newPi()
	p.stdin = failingStdin{}
	if err := p.input("hello", nil); err == nil || !strings.Contains(err.Error(), "pi is not running") {
		t.Errorf("error %v", err)
	}
}

type failingStdin struct{}

func (failingStdin) Write([]byte) (int, error) { return 0, os.ErrClosed }

// A call waiting for its response ends with the reason pi's output ended.
func TestPendingCallEndsWithTheExitReason(t *testing.T) {
	p := newPi()
	p.stdin = io.Discard
	done := make(chan error, 1)
	go func() { done <- p.call(context.Background(), "abort", nil, nil) }()
	p.end(errors.New("pi exited: exit status 3: boom"))
	if err := <-done; err == nil || err.Error() != "abort: pi exited: exit status 3: boom" {
		t.Fatal(err)
	}
}

// fakePi installs a `pi` that records its arguments in $FAKE_ARGS and the MCP
// description in $FAKE_ARGS.mcp, answers get_state and get_session_stats, and
// plays $FAKE_STREAM for each prompt. With $FAKE_EXIT it exits at a prompt.
// With $FAKE_PENDING it records an unanswered prompt there and waits for the
// file to be removed, so a test can cancel the call before the process exits.
func fakePi(t *testing.T, env map[string]string) (args string) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
echo "$@" > "$FAKE_ARGS"
echo "$HYPERCUBE_MCP" > "$FAKE_ARGS.mcp"
while IFS= read -r line; do
  id=$(printf '%s' "$line" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
  case "$line" in
    *'"type":"get_state"'*) printf '{"id":"%s","type":"response","command":"get_state","success":true,"data":{"model":{"provider":"%s","id":"%s","contextWindow":1000},"sessionFile":"%s","sessionId":"session-1"}}\n' "$id" "$FAKE_PROVIDER" "$FAKE_MODEL" "$FAKE_SESSION_FILE";;
    *'"type":"get_session_stats"'*) printf '{"id":"%s","type":"response","command":"get_session_stats","success":true,"data":{"cost":0.5,"contextUsage":{"tokens":7,"contextWindow":1000}}}\n' "$id";;
    *'"type":"prompt"'*)
      if [ -n "$FAKE_EXIT" ]; then echo boom >&2; exit 3; fi
      if [ -n "$FAKE_PENDING" ]; then
        echo pending > "$FAKE_PENDING"
        while [ -f "$FAKE_PENDING" ]; do sleep 0.01; done
        continue
      fi
      printf '{"id":"%s","type":"response","command":"prompt","success":true,"data":{"disposition":"started"}}\n' "$id"
      cat "$FAKE_STREAM";;
  esac
done
`
	if err := os.WriteFile(filepath.Join(dir, "pi"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	args = filepath.Join(dir, "args")
	t.Setenv("FAKE_ARGS", args)
	t.Setenv("FAKE_PROVIDER", "openrouter")
	t.Setenv("FAKE_MODEL", "x/m")
	t.Setenv("FAKE_SESSION_FILE", filepath.Join(dir, "session.jsonl"))
	stream, err := filepath.Abs(filepath.Join("testdata", "tool.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_STREAM", stream)
	t.Setenv("FAKE_EXIT", "")
	t.Setenv("FAKE_PENDING", "")
	for k, v := range env {
		t.Setenv(k, v)
	}
	return args
}

// emitted collects the events of a started agent.
type emitted struct {
	mu     sync.Mutex
	events []harness.Event
}

func (e *emitted) emit(ev harness.Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, ev)
}

func (e *emitted) all() []harness.Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.events)
}

// idle waits until the agent has reported itself idle and returns the events
// so far.
func (e *emitted) idle(t *testing.T) []harness.Event {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		events := e.all()
		if slices.ContainsFunc(events, func(ev harness.Event) bool { return ev.State == harness.Idle }) {
			return events
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out; saw %+v", events)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func start(t *testing.T, a harness.StartArgs) (*agent, *emitted) {
	t.Helper()
	e := &emitted{}
	ag, err := startAgent(a, e.emit)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ag.proc.Stop)
	return ag, e
}

// startFails starts pi and returns the error, checking that nothing was
// emitted.
func startFails(t *testing.T, a harness.StartArgs) error {
	t.Helper()
	e := &emitted{}
	ag, err := startAgent(a, e.emit)
	if err == nil {
		ag.proc.Stop()
		t.Fatal("start succeeded")
	}
	if events := e.all(); len(events) != 0 {
		t.Errorf("events %+v", events)
	}
	return err
}

func startArgs(t *testing.T) harness.StartArgs {
	return harness.StartArgs{
		Hypercube: "/bin/hc", Env: map[string]string{"HYPERCUBE_SESSION_ID": "a1"}, Session: "a1",
		Model: "openrouter/x/m", Effort: "high", Dir: t.TempDir(), Instructions: "You work in a team.", StateDir: t.TempDir(),
	}
}

func TestStart(t *testing.T) {
	argsFile := fakePi(t, nil)
	a := startArgs(t)
	ag, e := start(t, a)
	if c := ag.conversation(); c != "" {
		t.Errorf("a new session %s was reported before it exists", c)
	}
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	extension := filepath.Join(a.StateDir, "pi-extension", "index.js")
	for _, want := range []string{"--mode rpc", "--provider openrouter", "--model x/m", "--thinking high", "--append-system-prompt You work in a team.", "-e " + extension} {
		if !strings.Contains(string(data), want) {
			t.Errorf("arguments lack %s: %s", want, data)
		}
	}
	if strings.Contains(string(data), "--session-id") {
		t.Error(string(data))
	}
	mcp, _ := os.ReadFile(argsFile + ".mcp")
	if strings.TrimSpace(string(mcp)) != `{"args":["mcp"],"command":"/bin/hc","env":{"HYPERCUBE_SESSION_ID":"a1"},"exposure":"direct"}` {
		t.Errorf("MCP description %s", mcp)
	}
	if text, err := os.ReadFile(extension); err != nil || !strings.Contains(string(text), `registerMcpServer("hypercube"`) {
		t.Errorf("extension %v %s", err, text)
	}

	if err := ag.input("hello", nil); err != nil {
		t.Fatal(err)
	}
	var texts []string
	var reported []string
	for _, ev := range e.idle(t) {
		if ev.Conversation != "" {
			reported = append(reported, ev.Conversation)
		}
		for _, it := range ev.Items {
			if it.Kind == harness.KindText && it.Done {
				texts = append(texts, it.Text)
			}
		}
	}
	if !slices.Equal(texts, []string{"done"}) || !slices.Equal(reported, []string{"session-1"}) {
		t.Errorf("texts %v, conversations %v", texts, reported)
	}
	if err := ag.answer("nothing", "", ""); err == nil {
		t.Error("an answer to no request succeeded")
	}
}

func TestStartResumesTheSession(t *testing.T) {
	argsFile := fakePi(t, nil)
	if err := os.WriteFile(os.Getenv("FAKE_SESSION_FILE"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := startArgs(t)
	a.Conversation = "session-1"
	ag, e := start(t, a)
	if c := ag.conversation(); c != "session-1" {
		t.Errorf("conversation %q", c)
	}
	data, _ := os.ReadFile(argsFile)
	if !strings.Contains(string(data), "--session-id session-1") {
		t.Error(string(data))
	}
	// The cost of the resumed session carries on.
	if err := ag.input("hello", nil); err != nil {
		t.Fatal(err)
	}
	var usage harness.Usage
	for _, ev := range e.idle(t) {
		if ev.Usage != nil {
			usage = *ev.Usage
		}
	}
	if usage.Cost <= 0.5 || usage.Window != 1000 {
		t.Errorf("usage %+v", usage)
	}
}

func TestStartFailedResume(t *testing.T) {
	// The session file is missing: Pi would start a new session under the ID.
	fakePi(t, nil)
	a := startArgs(t)
	a.Conversation = "gone"
	if err := startFails(t, a); !strings.Contains(err.Error(), "no saved session gone") {
		t.Errorf("error %v", err)
	}
}

func TestStartRejectsAnotherModel(t *testing.T) {
	fakePi(t, map[string]string{"FAKE_MODEL": "other"})
	err := startFails(t, startArgs(t))
	if !strings.Contains(err.Error(), "openrouter/other") || !strings.Contains(err.Error(), "openrouter/x/m") {
		t.Errorf("error %v", err)
	}
}

func TestStartRejectsAModelWithoutProvider(t *testing.T) {
	fakePi(t, nil)
	a := startArgs(t)
	a.Model = "m"
	if _, err := startAgent(a, func(harness.Event) {}); err == nil || !strings.Contains(err.Error(), "provider/id") {
		t.Errorf("error %v", err)
	}
}

func TestStartWithoutPi(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := startAgent(startArgs(t), func(harness.Event) {}); err == nil || !strings.Contains(err.Error(), "Pi is missing") {
		t.Errorf("error %v", err)
	}
}

func TestStartReportsAProcessThatEnds(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'No API key for openrouter' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "pi"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, err := startAgent(startArgs(t), func(harness.Event) {})
	if err == nil || !strings.Contains(err.Error(), "exit status 1") || !strings.Contains(err.Error(), "No API key for openrouter") {
		t.Errorf("error %v", err)
	}
}

func TestExitEndsTheProcess(t *testing.T) {
	fakePi(t, map[string]string{"FAKE_EXIT": "1"})
	ag, _ := start(t, startArgs(t))
	_ = ag.input("hello", nil) // fails with the exit reason or a broken pipe
	select {
	case <-ag.proc.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("pi did not end")
	}
	if err := ag.proc.Err(); !strings.Contains(err.Error(), "exit status 3") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("error %v", err)
	}
}

func TestMCPToolNamesItsServer(t *testing.T) {
	p := newPi()
	p.feed(`{"type":"tool_execution_start","toolCallId":"c1","toolName":"mcp__hypercube__end_session","args":{"name":"auditor"}}`)
	tool := p.item(t, harness.KindTool)
	if tool.Server != "hypercube" || tool.Tool != "end_session" {
		t.Fatalf("%+v", tool)
	}
}

// piMessage is the lines of one assistant message whose text blocks are texts.
func piMessage(stop string, texts ...string) []string {
	lines := []string{`{"type":"message_start","message":{"role":"assistant"}}`}
	for i, text := range texts {
		lines = append(lines,
			fmt.Sprintf(`{"type":"message_update","assistantMessageEvent":{"type":"text_start","contentIndex":%d}}`, i),
			fmt.Sprintf(`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","contentIndex":%d,"delta":%q}}`, i, text),
			fmt.Sprintf(`{"type":"message_update","assistantMessageEvent":{"type":"text_end","contentIndex":%d,"content":%q}}`, i, text))
	}
	return append(lines, fmt.Sprintf(`{"type":"message_end","message":{"role":"assistant","stopReason":%q}}`, stop))
}

func TestIdleReportsTheResponseOfASuccessfulRun(t *testing.T) {
	run := func(settled string, messages ...[]string) []string {
		lines := []string{`{"type":"agent_start"}`}
		for _, m := range messages {
			lines = append(lines, m...)
		}
		return append(lines, settled)
	}
	for _, tc := range []struct {
		name  string
		lines []string
		final []string
	}{
		{"progress then response", run(`{"type":"agent_settled","aborted":false}`, piMessage("toolUse", "Looking."), piMessage("stop", "Done.")), []string{"m2:0"}},
		{"every text block", run(`{"type":"agent_settled","aborted":false}`, piMessage("stop", "One.", "Two.")), []string{"m1:0", "m1:1"}},
		{"tool use without a response", run(`{"type":"agent_settled","aborted":false}`, piMessage("toolUse", "Looking.")), nil},
		{"error", run(`{"type":"agent_settled","aborted":false}`, piMessage("error", "Partial.")), nil},
		{"aborted", run(`{"type":"agent_settled","aborted":true}`, piMessage("stop", "Done.")), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newPi()
			p.feed(tc.lines...)
			if got := p.finals(); len(got) != 1 || !slices.Equal(got[0], tc.final) {
				t.Errorf("final %v, want %v", got, tc.final)
			}
		})
	}
}

func TestResponseDoesNotCarryToTheNextRun(t *testing.T) {
	p := newPi()
	p.feed(`{"type":"agent_start"}`)
	p.feed(piMessage("stop", "One.")...)
	p.feed(`{"type":"agent_settled","aborted":false}`, `{"type":"agent_start"}`, `{"type":"agent_settled","aborted":false}`)
	if got := p.finals(); len(got) != 2 || len(got[0]) != 1 || got[1] != nil {
		t.Errorf("finals %v", got)
	}
}
