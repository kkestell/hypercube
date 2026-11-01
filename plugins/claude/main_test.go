package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

type nopCloser struct{ *bytes.Buffer }

func (nopCloser) Close() error { return nil }

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
		stdin: nopCloser{h.sent}, ready: make(chan struct{}),
		blocks: map[string]int{}, open: map[string]harness.Item{}, tools: map[string]harness.Item{},
		requests: map[string]*request{},
	}
	h.emit = func(ev harness.Event) { h.events = append(h.events, ev) }
	close(h.ready)
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
	if h.conversation != "98dec42e-cafa-48e3-a4a4-b76fa5c35b3a" || h.events[0].Conversation != "" {
		t.Errorf("conversation %q, first event %+v", h.conversation, h.events[0])
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

func TestQuestionBecomesAttentionAndAnswerReachesClaude(t *testing.T) {
	h := newClaude()
	head, rest := until(t, fixture(t, "question"), `"control_request"`)
	h.feed(head...)
	items := h.transcript()
	if len(items) != 1 || items[0].Kind != harness.KindAttention || items[0].Done || !items[0].Free ||
		!slices.Equal(items[0].Choices, []string{"Red", "Blue"}) || !strings.HasPrefix(items[0].Text, "Which color do you like, Red or Blue?") {
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
	if err := h.answer(item.ID, "", ""); err == nil {
		t.Error("an empty answer succeeded")
	}
	if err := h.answer(item.ID, "Red", ""); err != nil {
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
	if err := h.answer(item.ID, "Red", ""); err == nil {
		t.Error("a finished request took an answer")
	}
}

func TestQuestionTypedAnswer(t *testing.T) {
	h := newClaude()
	head, _ := until(t, fixture(t, "question"), `"control_request"`)
	h.feed(head...)
	if err := h.answer(h.transcript()[0].ID, "", "Green"); err != nil {
		t.Fatal(err)
	}
	_, body := h.reply(t)
	answers := body["updatedInput"].(map[string]any)["answers"].(map[string]any)
	if answers["Which color do you like, Red or Blue?"] != "Green" {
		t.Errorf("%v", answers)
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
	if err := h.answer("r1/1", "c", ""); err != nil {
		t.Fatal(err)
	}
	if len(h.sentFrames(t)) != 0 {
		t.Fatal("claude was answered before every question was")
	}
	if err := h.answer("r1/0", "b", ""); err != nil {
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
	if prompt.Kind != harness.KindAttention || !slices.Equal(prompt.Choices, []string{"Allow", "Deny"}) || !prompt.Free ||
		!strings.HasPrefix(prompt.Text, "Allow Bash?") {
		t.Fatalf("%+v", prompt)
	}
	if err := h.answer(prompt.ID, "Maybe", ""); err == nil {
		t.Error("an unknown choice succeeded")
	}
	if err := h.answer(prompt.ID, "Allow", ""); err != nil {
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
	if err := h.answer(prompt.ID, "Deny", "not now"); err != nil {
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
	if err := h.answer(h.transcript()[1].ID, "", "use a different file"); err != nil {
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
	if err := h.answer(prompt.ID, "Allow", ""); err == nil {
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
	if h.conversation == "" {
		t.Error("the line after it was not read")
	}
}

// fakeClaude installs a `claude` that answers initialize, plays $FAKE_STREAM
// for each user message, and records its arguments in $FAKE_ARGS. With
// $FAKE_FAIL it plays that file and exits at once, as claude does when it cannot
// resume. With $FAKE_EXIT it exits when it gets a user message.
func fakeClaude(t *testing.T, env map[string]string) (args string) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
echo "$@" > "$FAKE_ARGS"
if [ -n "$FAKE_FAIL" ]; then cat "$FAKE_FAIL"; echo "resume failed" >&2; exit 1; fi
while IFS= read -r line; do
  case "$line" in
    *'"initialize"'*) echo '{"type":"control_response","response":{"subtype":"success","request_id":"initialize","response":{}}}' ;;
    *'"type":"user"'*) if [ -n "$FAKE_EXIT" ]; then echo boom >&2; exit 3; fi; cat "$FAKE_STREAM" ;;
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
	t.Setenv("FAKE_FAIL", "")
	t.Setenv("FAKE_EXIT", "")
	for k, v := range env {
		t.Setenv(k, v)
	}
	return args
}

// connection is a test's end of serve.
type connection struct {
	t       *testing.T
	in      *os.File
	records chan harness.Record
	done    chan error
	next    int
}

func connect(t *testing.T) *connection {
	t.Helper()
	inRead, inWrite, _ := os.Pipe()
	outRead, outWrite, _ := os.Pipe()
	c := &connection{t: t, in: inWrite, records: make(chan harness.Record, 256), done: make(chan error, 1)}
	go func() {
		c.done <- serve(inRead, outWrite)
		outWrite.Close()
	}()
	go func() {
		scanner := bufio.NewScanner(outRead)
		for scanner.Scan() {
			var r harness.Record
			if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
				t.Error(err)
			}
			c.records <- r
		}
		if err := scanner.Err(); err != nil {
			t.Error(err)
		}
		close(c.records)
	}()
	t.Cleanup(func() { inWrite.Close() })
	return c
}

func (c *connection) send(cmd harness.Command) string {
	c.next++
	cmd.ID = string(rune('a' + c.next))
	b, _ := json.Marshal(cmd)
	if _, err := c.in.Write(append(b, '\n')); err != nil {
		c.t.Fatal(err)
	}
	return cmd.ID
}

// until reads records until match accepts one, and returns the events and the
// reply seen on the way.
func (c *connection) until(match func(harness.Record) bool) (events []harness.Event, replies []harness.Reply) {
	c.t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case r, ok := <-c.records:
			if !ok {
				c.t.Fatal("the connection closed")
			}
			if r.Event != nil {
				events = append(events, *r.Event)
			}
			if r.Reply != nil {
				replies = append(replies, *r.Reply)
			}
			if match(r) {
				return
			}
		case <-timeout:
			c.t.Fatal("timed out")
		}
	}
}

func replyTo(id string) func(harness.Record) bool {
	return func(r harness.Record) bool { return r.Reply != nil && r.Reply.ID == id }
}

var startArgs = harness.StartArgs{
	Hypercube: "/bin/hc", Env: map[string]string{"HYPERCUBE_AGENT_ID": "a1"}, Agent: "a1", Role: "lead",
	Model: "opus", Effort: "high", Dir: ".", Instructions: "You lead a team.",
}

func TestServe(t *testing.T) {
	argsFile := fakeClaude(t, nil)
	c := connect(t)
	id := c.send(harness.Command{Command: harness.CmdStart, Start: &startArgs})
	_, replies := c.until(replyTo(id))
	if replies[len(replies)-1].Error != "" {
		t.Fatal(replies)
	}
	events, _ := c.until(func(r harness.Record) bool { return r.Event != nil && r.Event.State == harness.Idle })
	if len(events) == 0 {
		t.Error("no idle event after start")
	}
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	args := string(data)
	for _, want := range []string{"-p", "--input-format stream-json", "--output-format stream-json", "--include-partial-messages",
		"--permission-prompt-tool stdio", "--permission-mode auto", "--model opus", "--effort high", "--append-system-prompt You lead a team.",
		`"command":"/bin/hc","env":{"HYPERCUBE_AGENT_ID":"a1"}`} {
		if !strings.Contains(args, want) {
			t.Errorf("arguments lack %s: %s", want, args)
		}
	}
	if strings.Contains(args, "--resume") {
		t.Error(args)
	}

	id = c.send(harness.Command{Command: harness.CmdInput, Text: "hello"})
	events, _ = c.until(func(r harness.Record) bool { return r.Event != nil && r.Event.Usage != nil && r.Event.Usage.Window > 0 })
	var texts []string
	for _, ev := range events {
		for _, it := range ev.Items {
			if it.Done {
				texts = append(texts, it.Text)
			}
		}
	}
	if !slices.Equal(texts, []string{"Hello there."}) {
		t.Errorf("texts %v", texts)
	}

	id = c.send(harness.Command{Command: harness.CmdAnswer, Request: "nothing"})
	if _, replies = c.until(replyTo(id)); replies[len(replies)-1].Error == "" {
		t.Error("an answer to no request succeeded")
	}
	id = c.send(harness.Command{Command: harness.CmdShutdown})
	c.until(replyTo(id))
	select {
	case err := <-c.done:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not end")
	}
}

func TestServeResumesTheConversation(t *testing.T) {
	argsFile := fakeClaude(t, nil)
	c := connect(t)
	start := startArgs
	start.Conversation = "saved-conversation"
	id := c.send(harness.Command{Command: harness.CmdStart, Start: &start})
	if _, replies := c.until(replyTo(id)); replies[len(replies)-1].Error != "" {
		t.Fatal(replies)
	}
	data, _ := os.ReadFile(argsFile)
	if !strings.Contains(string(data), "--resume saved-conversation") {
		t.Error(string(data))
	}
}

func TestServeFailedResumeIsAnErrorReply(t *testing.T) {
	fakeClaude(t, map[string]string{"FAKE_FAIL": filepath.Join("testdata", "badresume.jsonl")})
	c := connect(t)
	start := startArgs
	start.Conversation = "11111111-1111-4111-8111-111111111111"
	id := c.send(harness.Command{Command: harness.CmdStart, Start: &start})
	events, replies := c.until(replyTo(id))
	if got := replies[len(replies)-1].Error; !strings.Contains(got, "No conversation found") {
		t.Errorf("error %q", got)
	}
	if len(events) != 0 {
		t.Errorf("events %+v", events)
	}
}

func TestServeEndsWhenClaudeExits(t *testing.T) {
	fakeClaude(t, map[string]string{"FAKE_EXIT": "1"})
	c := connect(t)
	id := c.send(harness.Command{Command: harness.CmdStart, Start: &startArgs})
	c.until(replyTo(id))
	c.send(harness.Command{Command: harness.CmdInput, Text: "hello"})
	select {
	case err := <-c.done:
		if err == nil || !strings.Contains(err.Error(), "exit status 3") || !strings.Contains(err.Error(), "boom") {
			t.Errorf("error %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not end")
	}
}

func TestServeRejectsCommandsBeforeStart(t *testing.T) {
	fakeClaude(t, nil)
	c := connect(t)
	id := c.send(harness.Command{Command: harness.CmdInput, Text: "hello"})
	if _, replies := c.until(replyTo(id)); replies[len(replies)-1].Error == "" {
		t.Error("input before start succeeded")
	}
}

func TestMCPToolNamesItsServer(t *testing.T) {
	h := newClaude()
	h.feed(`{"type":"assistant","message":{"id":"m1","role":"assistant","content":[{"type":"tool_use","id":"t1","name":"mcp__hypercube__send_message","input":{"to":"lead","text":"done"}}]}}`)
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
