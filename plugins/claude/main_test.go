package main

import (
	"bytes"
	"encoding/json"
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

// The fixtures in testdata are claude's own output, recorded from
// `claude -p --input-format stream-json --output-format stream-json
// --include-partial-messages --permission-prompt-tool stdio` and trimmed of
// fields the plugin does not read.

func fixture(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// harnessUnderTest is a claude that reads fixture lines and writes to a
// buffer instead of a process.
type harnessUnderTest struct {
	*claude
	events []harness.Event
	sent   *bytes.Buffer
}

func newClaude() *harnessUnderTest {
	h := &harnessUnderTest{sent: &bytes.Buffer{}}
	h.claude = &claude{
		stdin:  h.sent,
		blocks: map[string]int{}, open: map[string]harness.Item{}, tools: map[string]harness.Item{},
	}
	h.emit = func(ev harness.Event) { h.events = append(h.events, ev) }
	return h
}

func (h *harnessUnderTest) feed(lines ...string) {
	for _, line := range lines {
		h.line([]byte(line))
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

// transcript applies events the way Hypercube does: a later item with the same
// ID replaces the earlier one, or extends its text.
func (h *harnessUnderTest) transcript() []harness.Item {
	var items []harness.Item
	for _, ev := range h.events {
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

func (h *harnessUnderTest) states() []string {
	var states []string
	for _, ev := range h.events {
		if ev.State != "" {
			states = append(states, ev.State)
		}
	}
	return states
}

// idleLine is claude's report that the turn is over.
const idleLine = `{"type":"system","subtype":"session_state_changed","state":"idle"}`

// finals are the response items reported with each idle state.
func (h *harnessUnderTest) finals() [][]string {
	var finals [][]string
	for _, ev := range h.events {
		if ev.State == harness.Idle {
			finals = append(finals, ev.Final)
		}
	}
	return finals
}

func (h *harnessUnderTest) lastUsage() harness.Usage {
	for _, ev := range slices.Backward(h.events) {
		if ev.Usage != nil {
			return *ev.Usage
		}
	}
	return harness.Usage{}
}

// sentFrames decodes what was written to claude and clears it.
func (h *harnessUnderTest) sentFrames(t *testing.T) []map[string]any {
	t.Helper()
	var frames []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(h.sent.String()), "\n") {
		if line == "" {
			continue
		}
		var f map[string]any
		if err := json.Unmarshal([]byte(line), &f); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, f)
	}
	h.sent.Reset()
	return frames
}

// reply returns the response body of the one control response written.
func (h *harnessUnderTest) reply(t *testing.T) (id string, body map[string]any) {
	t.Helper()
	frames := h.sentFrames(t)
	if len(frames) != 1 || frames[0]["type"] != "control_response" {
		t.Fatalf("sent %v", frames)
	}
	r := frames[0]["response"].(map[string]any)
	return r["request_id"].(string), r["response"].(map[string]any)
}

func TestStreamedAndFinalTextAppearOnce(t *testing.T) {
	h := newClaude()
	h.feed(fixture(t, "text")...)
	items := h.transcript()
	if len(items) != 1 || items[0].Kind != harness.KindText || items[0].Text != "Hello there." || !items[0].Done {
		t.Fatalf("%+v", items)
	}
	var streamed int
	for _, ev := range h.events {
		for _, it := range ev.Items {
			if it.Append && it.Text == "Hello there." {
				streamed++
			}
		}
	}
	if streamed != 1 {
		t.Errorf("text streamed %d times", streamed)
	}
	if got := h.events[len(h.events)-1].Conversation; got != "98dec42e-cafa-48e3-a4a4-b76fa5c35b3a" || h.events[0].Conversation != "" {
		t.Errorf("conversation %q, first event %+v", got, h.events[0])
	}
	if got := h.states(); !slices.Equal(got, []string{harness.Working}) {
		t.Errorf("states %v", got)
	}
	if u := h.lastUsage(); u.Context != requestContext(t) || u.Window != 1000000 || u.Cost < 0.002 || u.Cost > 0.0022 {
		t.Errorf("usage %+v", u)
	}
}

// requestContext is the tokens of the fixture's one request.
func requestContext(t *testing.T) int {
	t.Helper()
	for _, line := range fixture(t, "text") {
		var m message
		if json.Unmarshal([]byte(line), &m) == nil && m.Type == "assistant" {
			u := m.Message.Usage
			return u.Input + u.CacheRead + u.CacheCreation
		}
	}
	t.Fatal("no assistant line")
	return 0
}

func TestRateLimitEventReportsLimits(t *testing.T) {
	h := newClaude()
	h.feed(fixture(t, "limits")...)
	if len(h.events) != 1 {
		t.Fatalf("events = %+v", h.events)
	}
	got := h.events[0].Limits
	for i := range got {
		if got[i].At <= 0 {
			t.Fatalf("limit without an observation time: %+v", got[i])
		}
		got[i].At = 0
	}
	want := []harness.Limit{{Minutes: 300, Used: 1, Resets: 1793767800}, {Minutes: 10080, Used: 39, Resets: 1794193200}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("limits = %+v, want %+v", got, want)
	}
}

func TestSessionStateReportsWorkAndIdle(t *testing.T) {
	h := newClaude()
	h.feed(fixture(t, "interrupt")...)
	if got := h.states(); !slices.Equal(got, []string{harness.Working, harness.Idle}) {
		t.Errorf("states %v", got)
	}
}

func TestToolCallAndResult(t *testing.T) {
	h := newClaude()
	h.feed(fixture(t, "tool")...)
	items := h.transcript()
	if len(items) != 2 {
		t.Fatalf("%+v", items)
	}
	tool := items[0]
	if tool.Kind != harness.KindTool || tool.Tool != "Bash" || !tool.Done || tool.Failed || tool.Output != "fixture" || !strings.Contains(string(tool.Input), "echo fixture") {
		t.Errorf("%+v", tool)
	}
	if items[1].Kind != harness.KindText || !items[1].Done || !strings.HasPrefix(items[1].Text, "The command ran") {
		t.Errorf("%+v", items[1])
	}
}

func TestNativeSubagentItemsAreNested(t *testing.T) {
	h := newClaude()
	h.feed(fixture(t, "nested")...)
	var agent harness.Item
	var children []harness.Item
	for _, it := range h.transcript() {
		switch {
		case it.Tool == "Agent":
			agent = it
		case it.Parent != "":
			children = append(children, it)
		}
	}
	if agent.ID == "" || agent.Parent != "" || len(children) != 2 {
		t.Fatalf("agent %+v, children %+v", agent, children)
	}
	for _, c := range children {
		if c.Parent != agent.ID || c.Kind != harness.KindTool || !c.Done {
			t.Errorf("%+v", c)
		}
	}
	if children[0].Tool != "Read" || children[0].Output != "1\thi\n2\t" {
		t.Errorf("%+v", children[0])
	}
	// Only the main conversation changes work state.
	if got := h.states(); !slices.Equal(got, []string{harness.Working, harness.Idle}) {
		t.Errorf("states %v", got)
	}
	// Empty reasoning is not shown.
	for _, it := range h.transcript() {
		if it.Kind == harness.KindReasoning {
			t.Errorf("%+v", it)
		}
	}
}

func TestMainResultPreservesBackgroundMessageBlocks(t *testing.T) {
	h := newClaude()
	h.feed(
		`{"type":"assistant","parent_tool_use_id":"agent1","message":{"id":"child1","content":[{"type":"thinking","thinking":"considering"}]}}`,
		`{"type":"result","subtype":"success","modelUsage":{}}`,
		`{"type":"assistant","parent_tool_use_id":"agent1","message":{"id":"child1","content":[{"type":"text","text":"answer"}]}}`)
	items := h.transcript()
	if len(items) != 2 || items[0].Kind != harness.KindReasoning || items[0].Text != "considering" ||
		items[1].Kind != harness.KindText || items[1].Text != "answer" || items[0].ID == items[1].ID {
		t.Fatalf("background message blocks: %+v", items)
	}
}

func TestMainResultPreservesBackgroundTool(t *testing.T) {
	h := newClaude()
	h.feed(
		`{"type":"assistant","parent_tool_use_id":"agent1","message":{"id":"child1","content":[{"type":"tool_use","id":"read1","name":"Read","input":{}}]}}`,
		`{"type":"result","subtype":"success","modelUsage":{}}`,
		`{"type":"user","parent_tool_use_id":"agent1","message":{"content":[{"type":"tool_result","tool_use_id":"read1","content":"contents"}]}}`)
	items := h.transcript()
	if len(items) != 1 || items[0].Parent != "agent1" || !items[0].Done || items[0].Output != "contents" {
		t.Fatalf("background tool result: %+v", items)
	}
}

func TestQuestionBecomesAttentionAndAnswerReachesClaude(t *testing.T) {
	h := newClaude()
	head, rest := until(t, fixture(t, "question"), `"control_request"`)
	h.feed(head...)
	items := h.transcript()
	if len(items) != 1 || items[0].Kind != harness.KindAttention || items[0].Done || !items[0].Free ||
		items[0].Grant != "" || !slices.Equal(items[0].Choices, []string{"Red", "Blue"}) || items[0].Text != "Which color do you like, Red or Blue?" || !slices.Equal(items[0].Notes, []string{"Choose Red", "Choose Blue"}) {
		t.Fatalf("%+v", items)
	}
	// The question is not also a tool call.
	h.feed(rest...)
	for _, it := range h.transcript() {
		if it.Kind == harness.KindTool {
			t.Errorf("%+v", it)
		}
	}
}

func TestQuestionAnswer(t *testing.T) {
	h := newClaude()
	head, _ := until(t, fixture(t, "question"), `"control_request"`)
	h.feed(head...)
	item := h.transcript()[0]
	if err := h.answer(item.ID, nil, ""); err == nil {
		t.Error("an empty answer succeeded")
	}
	if err := h.answer(item.ID, []string{"Red"}, ""); err != nil {
		t.Fatal(err)
	}
	id, body := h.reply(t)
	if !strings.HasPrefix(item.ID, id+"/") || body["behavior"] != "allow" {
		t.Fatalf("%s %v", id, body)
	}
	in := body["updatedInput"].(map[string]any)
	if answers := in["answers"].(map[string]any); answers["Which color do you like, Red or Blue?"] != "Red" {
		t.Errorf("%v", in)
	}
	if _, ok := in["questions"]; !ok {
		t.Errorf("the question is not in %v", in)
	}
	done := h.transcript()[0]
	if !done.Done || done.Answer != "Red" {
		t.Errorf("%+v", done)
	}
	if err := h.answer(item.ID, []string{"Red"}, ""); err == nil {
		t.Error("a finished request took an answer")
	}
}

func TestQuestionTypedAnswer(t *testing.T) {
	h := newClaude()
	head, _ := until(t, fixture(t, "question"), `"control_request"`)
	h.feed(head...)
	if err := h.answer(h.transcript()[0].ID, nil, "Green"); err != nil {
		t.Fatal(err)
	}
	_, body := h.reply(t)
	answers := body["updatedInput"].(map[string]any)["answers"].(map[string]any)
	if answers["Which color do you like, Red or Blue?"] != "Green" {
		t.Errorf("%v", answers)
	}
}

func TestQuestionInputIsKept(t *testing.T) {
	h := newClaude()
	h.feed(`{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"AskUserQuestion","input":{` +
		`"questions":[{"question":"One?","options":[{"label":"a","description":""}]}],"source":{"id":7,"tags":["x"]}}}}`)
	if err := h.answer("r1/0", []string{"a"}, ""); err != nil {
		t.Fatal(err)
	}
	_, body := h.reply(t)
	in := body["updatedInput"].(map[string]any)
	if !reflect.DeepEqual(in["source"], map[string]any{"id": 7.0, "tags": []any{"x"}}) {
		t.Errorf("the input's other fields were lost: %v", in)
	}
	if answers := in["answers"].(map[string]any); answers["One?"] != "a" {
		t.Errorf("%v", in)
	}
}

func TestSeveralQuestionsAnswerTogether(t *testing.T) {
	h := newClaude()
	h.feed(`{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"AskUserQuestion","input":{"questions":[` +
		`{"question":"One?","options":[{"label":"a","description":""},{"label":"b","description":""}]},` +
		`{"question":"Two?","options":[{"label":"c","description":""}]}]}}}`)
	items := h.transcript()
	if len(items) != 2 || items[0].ID != "r1/0" || items[1].ID != "r1/1" || items[0].Text != "One?" {
		t.Fatalf("%+v", items)
	}
	if err := h.answer("r1/1", []string{"c"}, ""); err != nil {
		t.Fatal(err)
	}
	if len(h.sentFrames(t)) != 0 {
		t.Fatal("claude was answered before every question was")
	}
	if err := h.answer("r1/0", []string{"b"}, ""); err != nil {
		t.Fatal(err)
	}
	_, body := h.reply(t)
	answers := body["updatedInput"].(map[string]any)["answers"].(map[string]any)
	if answers["One?"] != "b" || answers["Two?"] != "c" {
		t.Errorf("%v", answers)
	}
}

func TestToolPermissionPrompt(t *testing.T) {
	h := newClaude()
	head, _ := until(t, fixture(t, "permission"), `"control_request"`)
	h.feed(head...)
	items := h.transcript()
	if len(items) != 2 || items[0].Tool != "Bash" || items[0].Done {
		t.Fatalf("%+v", items)
	}
	prompt := items[1]
	if prompt.Kind != harness.KindAttention || prompt.Grant != "Bash" || !slices.Equal(prompt.Choices, []string{"Allow", "Deny"}) || !prompt.Free ||
		!strings.HasPrefix(prompt.Text, "Allow Bash?") {
		t.Fatalf("%+v", prompt)
	}
	if err := h.answer(prompt.ID, []string{"Maybe"}, ""); err == nil {
		t.Error("an unknown choice succeeded")
	}
	if err := h.answer(prompt.ID, []string{"Allow"}, ""); err != nil {
		t.Fatal(err)
	}
	id, body := h.reply(t)
	in, _ := body["updatedInput"].(map[string]any)
	if id != prompt.ID || body["behavior"] != "allow" || !strings.Contains(in["command"].(string), "echo hi") {
		t.Errorf("%s %v", id, body)
	}
}

func TestDeniedToolShowsAsFailed(t *testing.T) {
	h := newClaude()
	head, rest := until(t, fixture(t, "permission"), `"control_request"`)
	h.feed(head...)
	prompt := h.transcript()[1]
	if err := h.answer(prompt.ID, []string{"Deny"}, "not now"); err != nil {
		t.Fatal(err)
	}
	_, body := h.reply(t)
	if body["behavior"] != "deny" || body["message"] != "not now" {
		t.Errorf("%v", body)
	}
	if got := h.transcript()[1]; !got.Done || got.Answer != "Deny: not now" {
		t.Errorf("%+v", got)
	}
	h.feed(rest...)
	items := h.transcript()
	if tool := items[0]; !tool.Done || !tool.Failed || tool.Output != "User declined" {
		t.Errorf("%+v", tool)
	}
	// Streamed text follows empty reasoning, so its block index is 1.
	last := items[len(items)-1]
	if last.Kind != harness.KindText || !last.Done || !strings.HasPrefix(last.Text, "The command did not run.") {
		t.Errorf("%+v", last)
	}
	var texts int
	for _, it := range items {
		if it.Kind == harness.KindText {
			texts++
		}
	}
	if texts != 1 {
		t.Errorf("%d text items", texts)
	}
}

func TestTypedTextAloneDenies(t *testing.T) {
	h := newClaude()
	head, _ := until(t, fixture(t, "permission"), `"control_request"`)
	h.feed(head...)
	if err := h.answer(h.transcript()[1].ID, nil, "use a different file"); err != nil {
		t.Fatal(err)
	}
	if _, body := h.reply(t); body["behavior"] != "deny" || body["message"] != "use a different file" {
		t.Errorf("%v", body)
	}
}

func TestCancelledRequestFinishes(t *testing.T) {
	h := newClaude()
	head, _ := until(t, fixture(t, "permission"), `"control_request"`)
	h.feed(head...)
	prompt := h.transcript()[1]
	h.feed(`{"type":"control_cancel_request","request_id":"` + prompt.ID + `"}`)
	if got := h.transcript()[1]; !got.Done || got.Answer != "" {
		t.Errorf("%+v", got)
	}
	if err := h.answer(prompt.ID, []string{"Allow"}, ""); err == nil {
		t.Error("a cancelled request took an answer")
	}
}

func TestInterruptedTurnIsNotAnError(t *testing.T) {
	h := newClaude()
	h.feed(fixture(t, "interrupt")...)
	for _, it := range h.transcript() {
		if it.Kind == harness.KindError {
			t.Errorf("%+v", it)
		}
	}
	if tool := h.transcript()[0]; !tool.Done || !tool.Failed {
		t.Errorf("%+v", tool)
	}
	if err := h.interrupt(); err != nil {
		t.Fatal(err)
	}
	frames := h.sentFrames(t)
	if len(frames) != 1 || frames[0]["type"] != "control_request" || frames[0]["request"].(map[string]any)["subtype"] != "interrupt" {
		t.Errorf("%v", frames)
	}
}

func TestResultEndsWhatIsStillOpen(t *testing.T) {
	h := newClaude()
	h.feed(`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"cut off"}},"parent_tool_use_id":null,"api_message_id":"m1"}`,
		`{"type":"control_request","request_id":"r1","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{}}}`,
		`{"type":"result","subtype":"error_during_execution","is_error":true,"terminal_reason":"aborted_streaming","modelUsage":{}}`)
	items := h.transcript()
	if len(items) != 2 || !items[0].Done || items[0].Text != "cut off" || !items[1].Done {
		t.Fatalf("%+v", items)
	}
}

func TestFailedTurnIsAnError(t *testing.T) {
	h := newClaude()
	h.feed(`{"type":"result","subtype":"success","is_error":true,"result":"API Error: 529 overloaded","terminal_reason":"model_error","total_cost_usd":0.5,"modelUsage":{}}`,
		`{"type":"result","subtype":"error_during_execution","is_error":true,"errors":["no budget"],"terminal_reason":"completed","modelUsage":{}}`)
	items := h.transcript()
	if len(items) != 2 || items[0].Kind != harness.KindError || items[0].Text != "API Error: 529 overloaded" || !items[0].Done || items[0].ID == items[1].ID || items[1].Text != "no budget" {
		t.Fatalf("%+v", items)
	}
}

func TestUnreadableLineIsSkipped(t *testing.T) {
	h := newClaude()
	lines := fixture(t, "text")
	h.feed("not json", lines[len(lines)-1])
	if len(h.events) != 1 || h.events[0].Conversation != "98dec42e-cafa-48e3-a4a4-b76fa5c35b3a" {
		t.Errorf("the line after it was not reported: %+v", h.events)
	}
}

// fakeClaude installs a `claude` that answers initialize, plays $FAKE_STREAM
// for each user message, and records its arguments in $FAKE_ARGS. It plays
// $FAKE_BEFORE first, and with $FAKE_FAIL it then exits, as claude does when
// it cannot resume. With $FAKE_INIT it plays that file in answer to
// initialize and exits.
func fakeClaude(t *testing.T, env map[string]string) (args string) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
echo "$@" > "$FAKE_ARGS"
if [ -n "$FAKE_BEFORE" ]; then cat "$FAKE_BEFORE"; fi
if [ -n "$FAKE_FAIL" ]; then echo "resume failed" >&2; exit 1; fi
while IFS= read -r line; do
  case "$line" in
    *'"initialize"'*)
      if [ -n "$FAKE_INIT" ]; then cat "$FAKE_INIT"; exit 0; fi
      echo '{"type":"control_response","response":{"subtype":"success","request_id":"initialize","response":{}}}' ;;
    *'"type":"user"'*) cat "$FAKE_STREAM" ;;
  esac
done
`
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	args = filepath.Join(dir, "args")
	t.Setenv("FAKE_ARGS", args)
	t.Setenv("FAKE_STREAM", filepath.Join("testdata", "text.jsonl"))
	for _, k := range []string{"FAKE_BEFORE", "FAKE_FAIL", "FAKE_INIT"} {
		t.Setenv(k, "")
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	return args
}

// playFile writes a file of claude output for the fake to play.
func playFile(t *testing.T, lines ...string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "lines.jsonl")
	if err := os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

// emitted collects the events of a started claude.
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

var startArgs = harness.StartArgs{
	Hypercube: "/bin/hc", Env: map[string]string{"HYPERCUBE_SESSION_ID": "a1"}, Session: "a1",
	Model: "opus", Effort: "high", Dir: ".", Instructions: "Fix the parser.",
}

func start(t *testing.T, a harness.StartArgs) (*claude, *emitted) {
	t.Helper()
	e := &emitted{}
	c, err := startClaude(a, e.emit)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.proc.Stop)
	return c, e
}

func TestStart(t *testing.T) {
	argsFile := fakeClaude(t, nil)
	c, e := start(t, startArgs)
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	args := string(data)
	for _, want := range []string{"-p", "--input-format stream-json", "--output-format stream-json", "--include-partial-messages",
		"--permission-prompt-tool stdio", "--permission-mode auto", "--model opus", "--effort high", "--append-system-prompt Fix the parser.",
		`"command":"/bin/hc","env":{"HYPERCUBE_SESSION_ID":"a1"}`} {
		if !strings.Contains(args, want) {
			t.Errorf("arguments lack %s: %s", want, args)
		}
	}
	if strings.Contains(args, "--resume") {
		t.Error(args)
	}

	if err := c.input("hello", nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !slices.ContainsFunc(e.all(), func(ev harness.Event) bool { return ev.Usage != nil && ev.Usage.Window > 0 }) {
		if time.Now().After(deadline) {
			t.Fatal("the turn did not end")
		}
		time.Sleep(10 * time.Millisecond)
	}
	var texts []string
	for _, ev := range e.all() {
		for _, it := range ev.Items {
			if it.Done {
				texts = append(texts, it.Text)
			}
		}
	}
	if !slices.Equal(texts, []string{"Hello there."}) {
		t.Errorf("texts %v", texts)
	}
	if err := c.answer("nothing", nil, ""); err == nil {
		t.Error("an answer to no request succeeded")
	}
}

func TestStartResumes(t *testing.T) {
	argsFile := fakeClaude(t, nil)
	a := startArgs
	a.Conversation = "saved-conversation"
	start(t, a)
	data, _ := os.ReadFile(argsFile)
	if !strings.Contains(string(data), "--resume saved-conversation") {
		t.Error(string(data))
	}
}

func TestStartForksWithoutAnnouncingTheParent(t *testing.T) {
	argsFile := fakeClaude(t, nil)
	a := startArgs
	a.Fork = "parent-conversation"
	c, _ := start(t, a)
	data, _ := os.ReadFile(argsFile)
	if !strings.Contains(string(data), "--resume parent-conversation --fork-session --session-id ") || c.conversation != "" {
		t.Fatalf("arguments %s, conversation %q", data, c.conversation)
	}
}

// startFails starts claude and returns the error, checking that nothing was
// emitted.
func startFails(t *testing.T, a harness.StartArgs) error {
	t.Helper()
	e := &emitted{}
	c, err := startClaude(a, e.emit)
	if err == nil {
		c.proc.Stop()
		t.Fatal("start succeeded")
	}
	if events := e.all(); len(events) != 0 {
		t.Errorf("events %+v", events)
	}
	return err
}

func TestStartFailedResume(t *testing.T) {
	const conversation = "11111111-1111-4111-8111-111111111111"
	other := strings.ReplaceAll(fixture(t, "badresume")[0], conversation, "22222222-2222-4222-8222-222222222222")
	for name, file := range map[string]string{
		"same session":  filepath.Join("testdata", "badresume.jsonl"),
		"other session": playFile(t, other),
	} {
		t.Run(name, func(t *testing.T) {
			fakeClaude(t, map[string]string{"FAKE_BEFORE": file, "FAKE_FAIL": "1"})
			a := startArgs
			a.Conversation = conversation
			if err := startFails(t, a); !strings.Contains(err.Error(), "No conversation found") {
				t.Error(err)
			}
		})
	}
}

// A result before the response to initialize fails the start, and what
// follows is discarded.
func TestStartFailsOnAResult(t *testing.T) {
	fakeClaude(t, map[string]string{
		"FAKE_BEFORE": playFile(t, `{"type":"result","subtype":"error_during_execution","is_error":true,"errors":["first failure"]}`),
		"FAKE_INIT": playFile(t,
			`{"type":"control_response","response":{"subtype":"success","request_id":"initialize","response":{}}}`,
			`{"type":"result","subtype":"error_during_execution","is_error":true,"errors":["second failure"]}`),
	})
	if err := startFails(t, startArgs); err.Error() != "first failure" {
		t.Error(err)
	}
}

func TestStartFailsOnAnInitializeError(t *testing.T) {
	fakeClaude(t, map[string]string{
		"FAKE_INIT": playFile(t,
			`{"type":"control_response","response":{"subtype":"error","request_id":"initialize","error":"bad options"}}`,
			`{"type":"assistant","message":{"id":"m1","content":[{"type":"text","text":"ignored"}]}}`,
			`{"type":"result","subtype":"success","result":"done"}`),
	})
	if err := startFails(t, startArgs); err.Error() != "bad options" {
		t.Error(err)
	}
}

func TestMCPToolNamesItsServer(t *testing.T) {
	h := newClaude()
	h.feed(`{"type":"assistant","message":{"id":"m1","role":"assistant","content":[{"type":"tool_use","id":"t1","name":"mcp__hypercube__send_message","input":{"to":"p","text":"done"}}]}}`)
	items := h.transcript()
	if len(items) != 1 || items[0].Server != "hypercube" || items[0].Tool != "send_message" {
		t.Fatalf("%+v", items)
	}
}

func TestInputSendsImagesAsContentBlocks(t *testing.T) {
	h := newClaude()
	images := []harness.Image{{MIMEType: "image/png", Data: "AAAA"}, {MIMEType: "image/jpeg", Data: "BBBB"}}
	if err := h.input("look", images); err != nil {
		t.Fatal(err)
	}
	if err := h.input("", images[:1]); err != nil {
		t.Fatal(err)
	}
	if err := h.input("plain", nil); err != nil {
		t.Fatal(err)
	}
	var got []any
	for _, f := range h.sentFrames(t) {
		got = append(got, f["message"].(map[string]any)["content"])
	}
	want := `[[{"text":"look","type":"text"},{"source":{"data":"AAAA","media_type":"image/png","type":"base64"},"type":"image"},{"source":{"data":"BBBB","media_type":"image/jpeg","type":"base64"},"type":"image"}],` +
		`[{"source":{"data":"AAAA","media_type":"image/png","type":"base64"},"type":"image"}],"plain"]`
	if b, _ := json.Marshal(got); string(b) != want {
		t.Errorf("%s", b)
	}
}

func TestToolResultImagesAreExtracted(t *testing.T) {
	h := newClaude()
	h.feed(
		`{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/a.png"}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"a picture"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}},{"type":"image","source":{"type":"url","url":"http://x/y.png"}}]}]}}`)
	items := h.transcript()
	if len(items) != 1 {
		t.Fatalf("%+v", items)
	}
	if it := items[0]; it.Output != "a picture\n[image]" || !slices.Equal(it.Images, []harness.Image{{MIMEType: "image/png", Data: "AAAA"}}) || !it.Done {
		t.Errorf("%+v", it)
	}
}

func TestIdleReportsTheSuccessfulResponse(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines []string
		final []string
	}{
		{"progress then response", append(fixture(t, "tool"), idleLine), []string{"msg_011CgbcP2J3YD4s1Y7b2eN1N:0"}},
		{"every text block of the response", []string{
			`{"type":"assistant","message":{"id":"m1","content":[{"type":"text","text":"Checking."},{"type":"tool_use","id":"t1","name":"Read","input":{}}]}}`,
			`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}`,
			`{"type":"assistant","message":{"id":"m2","content":[{"type":"thinking","thinking":"hm"},{"type":"text","text":"One."},{"type":"text","text":"Two."}]}}`,
			`{"type":"result","subtype":"success","modelUsage":{}}`, idleLine,
		}, []string{"m2:1", "m2:2"}},
		{"interrupted turn", fixture(t, "interrupt"), nil},
		{"failed turn", []string{
			`{"type":"assistant","message":{"id":"m1","content":[{"type":"text","text":"Partial."}]}}`,
			`{"type":"result","subtype":"success","is_error":true,"result":"overloaded","terminal_reason":"model_error","modelUsage":{}}`, idleLine,
		}, nil},
		{"tool call without a response", []string{
			`{"type":"assistant","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Read","input":{}}]}}`,
			`{"type":"result","subtype":"success","modelUsage":{}}`, idleLine,
		}, nil},
		{"a native subagent's response is not the turn's", []string{
			`{"type":"assistant","message":{"id":"m1","content":[{"type":"text","text":"Mine."}]}}`,
			`{"type":"assistant","parent_tool_use_id":"agent1","message":{"id":"c1","content":[{"type":"text","text":"Theirs."}]}}`,
			`{"type":"result","subtype":"success","modelUsage":{}}`, idleLine,
		}, []string{"m1:0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newClaude()
			h.feed(tc.lines...)
			if got := h.finals(); len(got) != 1 || !slices.Equal(got[0], tc.final) {
				t.Errorf("final %v, want %v", got, tc.final)
			}
		})
	}
}

// The nested fixture has a successful result while a native subagent still
// runs, then the main agent's reply and an empty bookkeeping result. Only the
// last main message answers the turn.
func TestOnlyTheLastMainMessageAnswersAfterSeveralResults(t *testing.T) {
	h := newClaude()
	h.feed(fixture(t, "nested")...)
	finals := h.finals()
	if len(finals) != 1 || !slices.Equal(finals[0], []string{"msg_011CgbcPqf7LppvrGwF6c57S:0"}) {
		t.Fatalf("final %v", finals)
	}
	for _, it := range h.transcript() {
		if it.ID == finals[0][0] && !strings.HasPrefix(it.Text, "The subagent read") {
			t.Errorf("final item %+v", it)
		}
	}
}
