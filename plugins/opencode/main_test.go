package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

// The fixtures in testdata are what `opencode acp` sent while a client drove
// it: the updates and requests of one prompt, ending with the prompt's
// response unless the prompt was waiting for the user. Available-command and
// title updates are left out.

func fixture(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// standIn stands in for opencode: it answers the agent's requests from
// handlers and records everything the agent writes. It keeps the prompts open
// until the test ends their turns.
type standIn struct {
	t        *testing.T
	wmu      sync.Mutex
	out      io.Writer // to the agent
	mu       sync.Mutex
	wrote    []map[string]any
	prompts  []any // the IDs of the open prompts, oldest first
	handlers map[string]func(params map[string]any) (any, *plugin.RPCError)
}

// agentUnderTest is an agent connected to a stand-in opencode instead of a
// process.
type agentUnderTest struct {
	*agent
	acp *standIn

	evMu   sync.Mutex
	events []harness.Event
}

func connected(t *testing.T) *agentUnderTest {
	t.Helper()
	agentIn, peerOut := io.Pipe()
	peerIn, agentOut := io.Pipe()
	h := &agentUnderTest{acp: &standIn{t: t, out: peerOut, handlers: map[string]func(map[string]any) (any, *plugin.RPCError){
		"test/sync": func(map[string]any) (any, *plugin.RPCError) { return map[string]any{}, nil },
	}}}
	h.agent = newAgent(harness.StartArgs{}, func(ev harness.Event) {
		h.evMu.Lock()
		defer h.evMu.Unlock()
		h.events = append(h.events, ev)
	})
	h.rpc = plugin.NewRPC(agentOut, h.handle)
	go func() { h.rpc.End(plugin.ReadLines(agentIn, h.rpc.Line)) }()
	go h.acp.serve(peerIn)
	t.Cleanup(func() { h.cancel(); peerOut.Close(); peerIn.Close() })
	return h
}

func (p *standIn) write(line []byte) {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	_, _ = p.out.Write(append(line, '\n'))
}

func (p *standIn) serve(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		var m map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
			p.t.Error(err)
			return
		}
		p.mu.Lock()
		p.wrote = append(p.wrote, m)
		p.mu.Unlock()
		method, _ := m["method"].(string)
		id, hasID := m["id"]
		if method == "" || !hasID {
			continue
		}
		if method == "session/prompt" {
			p.mu.Lock()
			p.prompts = append(p.prompts, id)
			p.mu.Unlock()
			continue
		}
		params, _ := m["params"].(map[string]any)
		reply := map[string]any{"jsonrpc": "2.0", "id": id}
		if h := p.handlers[method]; h == nil {
			reply["error"] = map[string]any{"code": -32601, "message": "no handler for " + method}
		} else if result, rpcErr := h(params); rpcErr != nil {
			reply["error"] = rpcErr
		} else {
			reply["result"] = result
		}
		line, _ := json.Marshal(reply)
		// The agent may be writing to the stand-in while it replies, which a
		// pipe's buffer allows opencode but io.Pipe does not.
		go p.write(line)
	}
}

// sent returns what the agent has written so far with the given method.
func (p *standIn) sent(method string) []map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []map[string]any
	for _, m := range p.wrote {
		if m["method"] == method {
			out = append(out, m["params"].(map[string]any))
		}
	}
	return out
}

// responses returns the results and errors the agent wrote to requests.
func (p *standIn) responses() []map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []map[string]any
	for _, m := range p.wrote {
		if _, ok := m["method"]; !ok {
			out = append(out, m)
		}
	}
	return out
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

// feed sends recorded lines to the agent as opencode sends them. A prompt's
// response answers the open prompt and ends its turn. It returns once the
// agent has handled the lines.
func (h *agentUnderTest) feed(t *testing.T, lines ...string) {
	t.Helper()
	for _, line := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatal(err)
		}
		if _, ok := m["method"]; ok {
			h.acp.write([]byte(line))
		} else {
			h.answerPrompt(t, m)
		}
	}
	h.sync(t)
}

// endTurn answers the oldest open prompt with a stop reason.
func (h *agentUnderTest) endTurn(t *testing.T, stopReason string) {
	t.Helper()
	h.answerPrompt(t, map[string]any{"jsonrpc": "2.0", "result": map[string]any{"stopReason": stopReason}})
}

// answerPrompt sends a response to the oldest open prompt and waits until the
// agent has ended the prompt's turn.
func (h *agentUnderTest) answerPrompt(t *testing.T, response map[string]any) {
	t.Helper()
	eventually(t, "an open prompt", func() bool {
		h.acp.mu.Lock()
		defer h.acp.mu.Unlock()
		return len(h.acp.prompts) > 0
	})
	h.acp.mu.Lock()
	response["id"] = h.acp.prompts[0]
	h.acp.prompts = h.acp.prompts[1:]
	h.acp.mu.Unlock()
	h.mu.Lock()
	turn := h.turn
	h.mu.Unlock()
	line, _ := json.Marshal(response)
	h.acp.write(line)
	eventually(t, "the end of the turn", func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return !h.busy || h.turn != turn
	})
}

// sync waits until the agent has handled what was sent before: the response
// to a request comes after it.
func (h *agentUnderTest) sync(t *testing.T) {
	t.Helper()
	if err := h.call(context.Background(), "test/sync", nil, nil); err != nil {
		t.Fatal(err)
	}
}

// adopt takes the session ID a fixture was recorded with.
func (h *agentUnderTest) adopt(t *testing.T, lines []string) {
	t.Helper()
	var m struct {
		Params struct {
			SessionID string `json:"sessionId"`
		} `json:"params"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatal(err)
	}
	h.agent.session = m.Params.SessionID
}

// startTurn sends input, so that a prompt is in flight as it was when the
// fixtures were recorded.
func (h *agentUnderTest) startTurn(t *testing.T) {
	t.Helper()
	if err := h.input("go", nil); err != nil {
		t.Fatal(err)
	}
}

func (h *agentUnderTest) snapshot() []harness.Event {
	h.evMu.Lock()
	defer h.evMu.Unlock()
	return slices.Clone(h.events)
}

// transcript applies events the way Hypercube does: a later item with the same
// ID replaces the earlier one, or extends its text.
func (h *agentUnderTest) transcript() []harness.Item {
	var items []harness.Item
	for _, ev := range h.snapshot() {
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

func (h *agentUnderTest) item(t *testing.T, id string) harness.Item {
	t.Helper()
	for _, it := range h.transcript() {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("no item %s in %+v", id, h.transcript())
	return harness.Item{}
}

func (h *agentUnderTest) states() []string {
	var states []string
	for _, ev := range h.snapshot() {
		if ev.State != "" {
			states = append(states, ev.State)
		}
	}
	return states
}

// finals are the response items reported with each idle state.
func (h *agentUnderTest) finals() [][]string {
	var finals [][]string
	for _, ev := range h.snapshot() {
		if ev.State == harness.Idle {
			finals = append(finals, ev.Final)
		}
	}
	return finals
}

// update is a session/update line for the session ses_1, from a native
// subagent's session when child is set.
func update(kind, messageID, text, tool, child string) string {
	u := map[string]any{"sessionUpdate": kind}
	switch kind {
	case "tool_call":
		u["toolCallId"], u["name"], u["status"] = tool, "shell", "pending"
	default:
		u["content"] = map[string]any{"type": "text", "text": text}
	}
	if messageID != "" {
		u["messageId"] = messageID
	}
	if child != "" {
		u["_meta"] = map[string]any{"opencode/child-session": map[string]any{"id": child, "parentID": "ses_1"}}
	}
	line, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "ses_1", "update": u}})
	return string(line)
}

func kinds(items []harness.Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Kind)
	}
	return out
}

func TestText(t *testing.T) {
	h := connected(t)
	lines := fixture(t, "text")
	h.adopt(t, lines)
	h.startTurn(t)
	h.feed(t, lines...)
	items := h.transcript()
	if len(items) != 1 || items[0].Kind != harness.KindText || items[0].Text != "Hello there, good to meet you." || !items[0].Done {
		t.Fatalf("%+v", items)
	}
	if got := h.states(); !slices.Equal(got, []string{harness.Working, harness.Idle}) {
		t.Fatal(got)
	}
	var usage harness.Usage
	for _, ev := range h.snapshot() {
		if ev.Usage != nil {
			usage = *ev.Usage
		}
	}
	if usage.Context != 9211 || usage.Window != 1000000 || usage.Cost < 4.9e-05 || usage.Cost > 5e-05 {
		t.Fatalf("%+v", usage)
	}
}

func TestStreamedChunksExtendOneItem(t *testing.T) {
	h := connected(t)
	lines := fixture(t, "tool")
	h.adopt(t, lines)
	h.startTurn(t)
	h.feed(t, lines...)
	var reasoning []harness.Item
	for _, it := range h.transcript() {
		if it.Kind == harness.KindReasoning {
			reasoning = append(reasoning, it)
		}
	}
	if len(reasoning) != 1 || !strings.HasPrefix(reasoning[0].Text, "The user wants me to run `echo hi`") || !strings.Contains(reasoning[0].Text, "Let's just call shell.") || !reasoning[0].Done {
		t.Fatalf("%+v", reasoning)
	}
}

func TestTool(t *testing.T) {
	h := connected(t)
	lines := fixture(t, "tool")
	h.adopt(t, lines)
	h.startTurn(t)
	h.feed(t, lines...)
	var tools []harness.Item
	for _, it := range h.transcript() {
		if it.Kind == harness.KindTool {
			tools = append(tools, it)
		}
	}
	if len(tools) != 1 {
		t.Fatalf("%+v", tools)
	}
	tl := tools[0]
	if tl.Tool != "shell" || !strings.Contains(string(tl.Input), `"command":"echo hi"`) || tl.Output != "hi\n" || tl.Failed || !tl.Done {
		t.Fatalf("%+v", tl)
	}
	if got := kinds(h.transcript()); !slices.Equal(got, []string{"reasoning", "tool", "text"}) {
		t.Fatal(got)
	}
}

// A call that completed stays completed when a stale update follows.
func TestFinishedToolIgnoresLaterUpdates(t *testing.T) {
	h := connected(t)
	lines := fixture(t, "tool")
	h.adopt(t, lines)
	h.feed(t, lines[:len(lines)-3]...) // up to the completed update
	before := h.transcript()
	h.feed(t, lines[len(lines)-5])
	if after := h.transcript(); len(after) != len(before) || !after[len(after)-1].Done {
		t.Fatalf("%+v", after)
	}
}

func TestChildUpdatesAreExcludedWithoutAParentToolID(t *testing.T) {
	h := connected(t)
	lines := fixture(t, "nested")
	h.adopt(t, lines)
	h.startTurn(t)
	h.feed(t, lines...)
	var task harness.Item
	for _, it := range h.transcript() {
		if it.Tool == "subagent" {
			task = it
		}
		if it.Parent != "" || it.Tool == "shell" || it.Text == "child\n" {
			t.Fatalf("child output leaked into the main transcript: %+v", it)
		}
	}
	if task.ID == "" || !task.Done || task.Output == "" {
		t.Fatalf("parent tool result missing: %+v", task)
	}
	for _, ev := range h.snapshot() {
		if ev.Usage != nil && ev.Usage.Context == 0 {
			t.Fatal("child usage replaced the main context")
		}
	}
}

// A permission request is approved once without asking the user.
func TestPermissionRequestIsApprovedOnce(t *testing.T) {
	h := connected(t)
	lines := fixture(t, "permission")
	h.adopt(t, lines)
	h.feed(t, lines...)
	var r []map[string]any
	eventually(t, "the approval", func() bool { r = h.acp.responses(); return len(r) == 1 })
	outcome := r[0]["result"].(map[string]any)["outcome"].(map[string]any)
	if r[0]["id"] != float64(0) || outcome["outcome"] != "selected" || outcome["optionId"] != "once" {
		t.Fatal(r)
	}
	for _, it := range h.transcript() {
		if it.Kind == harness.KindAttention {
			t.Fatalf("asked the user: %+v", it)
		}
	}
}

func TestBadPermissionParamsAreRefused(t *testing.T) {
	h := connected(t)
	h.session = "ses_1"
	h.feed(t, `{"jsonrpc":"2.0","id":7,"method":"session/request_permission","params":{"options":"once"}}`)
	var r []map[string]any
	eventually(t, "the refusal", func() bool { r = h.acp.responses(); return len(r) == 1 })
	if r[0]["id"] != float64(7) || r[0]["error"].(map[string]any)["code"] != float64(-32602) {
		t.Fatal(r)
	}
}

func TestQuestionForm(t *testing.T) {
	h := connected(t)
	lines := fixture(t, "question")
	h.adopt(t, lines)
	h.feed(t, lines...)
	color, size := h.item(t, "form:0/0"), h.item(t, "form:0/1")
	if color.Kind != harness.KindAttention || color.Text != "Which color do you prefer?" || !slices.Equal(color.Notes, []string{"Go with red.", "Go with blue."}) ||
		!slices.Equal(color.Choices, []string{"Red", "Blue"}) || !color.Free || !slices.Equal(size.Choices, []string{"Small", "Large"}) {
		t.Fatalf("%+v %+v", color, size)
	}
	// The questions follow the call of the question tool that asked them.
	if got := kinds(h.transcript()); !slices.Equal(got[len(got)-3:], []string{"tool", "attention", "attention"}) {
		t.Fatal(got)
	}
	// The form is answered once every question has an answer.
	if err := h.answer("form:0/1", "Large", ""); err != nil {
		t.Fatal(err)
	}
	if got := h.acp.responses(); len(got) != 0 {
		t.Fatal("answered early:", got)
	}
	if err := h.answer("form:0/0", "", "green"); err != nil {
		t.Fatal(err)
	}
	var r []map[string]any
	eventually(t, "the answer", func() bool { r = h.acp.responses(); return len(r) == 1 })
	result := r[0]["result"].(map[string]any)
	content := result["content"].(map[string]any)
	if r[0]["id"] != float64(0) || result["action"] != "accept" || len(content) != 2 || content["q1"] != "Large" || content["q0_custom"] != "green" {
		t.Fatal(r)
	}
	if it := h.item(t, "form:0/0"); !it.Done || it.Answer != "green" {
		t.Fatalf("%+v", it)
	}
	for _, id := range []string{"form:0/0", "form:9/0"} {
		if err := h.answer(id, "Red", ""); err == nil || !strings.Contains(err.Error(), "is not open") {
			t.Fatal(id, err)
		}
	}
}

// A form that OpenCode withdraws is refused once, and its questions close
// when the turn ends.
func TestWithdrawnFormIsRefused(t *testing.T) {
	h := connected(t)
	lines := fixture(t, "question")
	h.adopt(t, lines)
	h.startTurn(t)
	h.feed(t, lines...)
	h.feed(t, `{"jsonrpc":"2.0","method":"$/cancel_request","params":{"requestId":0}}`)
	var r []map[string]any
	eventually(t, "the refusal", func() bool { r = h.acp.responses(); return len(r) == 1 })
	if r[0]["id"] != float64(0) || r[0]["error"].(map[string]any)["code"] != float64(-32603) {
		t.Fatal(r)
	}
	if it := h.item(t, "form:0/0"); it.Done {
		t.Fatalf("%+v", it)
	}
	if err := h.answer("form:0/0", "Red", ""); err != nil {
		t.Fatal(err)
	}
	h.endTurn(t, "cancelled")
	h.sync(t)
	if r = h.acp.responses(); len(r) != 1 {
		t.Fatal("answered a withdrawn form:", r)
	}
	if it := h.item(t, "form:0/1"); !it.Done {
		t.Fatalf("%+v", it)
	}
}

func TestFormWithMultipleChoiceAndPlainText(t *testing.T) {
	h := connected(t)
	h.session = "ses_1"
	h.feed(t, `{"jsonrpc":"2.0","id":5,"method":"elicitation/create","params":{"mode":"form","sessionId":"ses_1","requestedSchema":{"type":"object","properties":{`+
		`"tags":{"type":"array","title":"Tags","items":{"anyOf":[{"const":"a","title":"a"},{"const":"b","title":"b"}]}},`+
		`"note":{"type":"string","title":"A note"}}}}}`)
	tags, note := h.item(t, "form:0/0"), h.item(t, "form:0/1")
	if tags.Text != "Tags" || tags.Free || !slices.Equal(tags.Choices, []string{"a", "b"}) || note.Text != "A note" || !note.Free || len(note.Choices) != 0 {
		t.Fatalf("%+v %+v", tags, note)
	}
	if err := h.answer("form:0/0", "", "x"); err == nil {
		t.Fatal("typed text answered a question that lists its choices only")
	}
	if err := h.answer("form:0/0", "b", ""); err != nil {
		t.Fatal(err)
	}
	if err := h.answer("form:0/1", "", "hello"); err != nil {
		t.Fatal(err)
	}
	var r []map[string]any
	eventually(t, "the answer", func() bool { r = h.acp.responses(); return len(r) == 1 })
	content := r[0]["result"].(map[string]any)["content"].(map[string]any)
	if !slices.Equal(content["tags"].([]any), []any{"b"}) || content["note"] != "hello" {
		t.Fatal(content)
	}
}

func TestUnshowableFormIsCancelledWithAnError(t *testing.T) {
	h := connected(t)
	h.session = "ses_1"
	h.feed(t, `{"jsonrpc":"2.0","id":3,"method":"elicitation/create","params":{"mode":"form","requestedSchema":{"properties":{"n":{"type":"integer","title":"Count"}}}}}`)
	var r []map[string]any
	eventually(t, "the cancellation", func() bool { r = h.acp.responses(); return len(r) == 1 })
	if r[0]["result"].(map[string]any)["action"] != "cancel" {
		t.Fatal(r)
	}
	items := h.transcript()
	if len(items) != 1 || items[0].Kind != harness.KindError || !strings.Contains(items[0].Text, "integer") {
		t.Fatalf("%+v", items)
	}
}

func TestUnknownRequestsAreRefused(t *testing.T) {
	h := connected(t)
	h.session = "ses_1"
	h.feed(t, `{"jsonrpc":"2.0","id":4,"method":"fs/read_text_file","params":{}}`, `{"jsonrpc":"2.0","method":"opencode/unknown","params":{}}`)
	var r []map[string]any
	eventually(t, "the refusal", func() bool { r = h.acp.responses(); return len(r) == 1 })
	if r[0]["id"] != float64(4) || r[0]["error"].(map[string]any)["code"] != float64(-32601) {
		t.Fatal(r)
	}
}

// The cancelled prompt is the user's own doing, and it ends the turn.
func TestInterruptedTurn(t *testing.T) {
	h := connected(t)
	lines := fixture(t, "interrupt")
	h.adopt(t, lines)
	h.startTurn(t)
	h.feed(t, lines...)
	for _, it := range h.transcript() {
		if it.Kind == harness.KindError {
			t.Fatalf("an interrupted turn shows an error: %+v", it)
		}
	}
	tl := h.transcript()[slices.IndexFunc(h.transcript(), func(it harness.Item) bool { return it.Kind == harness.KindTool })]
	if !tl.Done || !tl.Failed {
		t.Fatalf("%+v", tl)
	}
}

func TestPromptFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		reason string
		err    *plugin.RPCError
		want   string
	}{
		"rejected":   {err: &plugin.RPCError{Code: -32603, Message: "provider.auth: sign in"}, want: "session/prompt: provider.auth: sign in (code -32603)"},
		"max tokens": {reason: "max_tokens", want: "output limit"},
		"refusal":    {reason: "refusal", want: "refused"},
	} {
		t.Run(name, func(t *testing.T) {
			h := connected(t)
			h.session = "ses_1"
			h.startTurn(t)
			if tc.err != nil {
				h.answerPrompt(t, map[string]any{"jsonrpc": "2.0", "error": tc.err})
			} else {
				h.endTurn(t, tc.reason)
			}
			if got := h.states(); !slices.Equal(got, []string{harness.Working, harness.Idle}) {
				t.Fatal(got)
			}
			items := h.transcript()
			if len(items) != 1 || items[0].Kind != harness.KindError || !strings.Contains(items[0].Text, tc.want) {
				t.Fatalf("%+v", items)
			}
		})
	}
	t.Run("stopping", func(t *testing.T) {
		h := connected(t)
		h.session = "ses_1"
		h.startTurn(t)
		h.cancel()
		eventually(t, "the end of the turn", func() bool { return slices.Equal(h.states(), []string{harness.Working, harness.Idle}) })
		if len(h.transcript()) != 0 {
			t.Fatalf("%+v", h.transcript())
		}
	})
}

// The serve loop reports a CLI exit. The pending prompt must not add a turn
// error before the serve loop has cancelled the agent's context.
func TestExitDuringAPromptIsNotATurnError(t *testing.T) {
	h := connected(t)
	h.session = "ses_1"
	h.startTurn(t)
	eventually(t, "the prompt", func() bool { return len(h.acp.sent("session/prompt")) == 1 })
	if err := h.acp.out.(io.Closer).Close(); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the end of the turn", func() bool { return slices.Equal(h.states(), []string{harness.Working, harness.Idle}) })
	if items := h.transcript(); len(items) != 0 {
		t.Fatalf("%+v", items)
	}
}

// A turn that ends finishes what was still streaming or waiting.
func TestEndOfTurnFinishesOpenItems(t *testing.T) {
	h := connected(t)
	lines := fixture(t, "question")
	h.adopt(t, lines)
	h.startTurn(t)
	h.feed(t, lines...)
	h.endTurn(t, "cancelled")
	for _, it := range h.transcript() {
		if !it.Done && it.Kind != harness.KindTool {
			t.Fatalf("%+v is still open", it)
		}
	}
	if err := h.answer("form:0/0", "Red", ""); err == nil {
		t.Fatal("answered a request of a finished turn")
	}
	// The form is cancelled while the connection stays open.
	var r []map[string]any
	eventually(t, "the form's answer", func() bool { r = h.acp.responses(); return len(r) == 1 })
	if r[0]["result"].(map[string]any)["action"] != "cancel" {
		t.Fatal(r)
	}
	select {
	case <-h.rpc.Done():
		t.Fatal("the connection closed")
	default:
	}
}

func defaultHandlers(h *agentUnderTest, effort bool) {
	options := func(model, effortValue string) []any {
		opts := []any{map[string]any{"id": "model", "type": "select", "name": "Model", "currentValue": model, "options": []any{
			map[string]any{"value": "p/m", "name": "m"},
		}}}
		if effort {
			opts = append(opts, map[string]any{"id": "effort", "type": "select", "name": "Effort", "currentValue": effortValue, "options": []any{
				map[string]any{"value": "default", "name": "Default"}, map[string]any{"value": "high", "name": "High"},
			}})
		}
		return opts
	}
	h.acp.handlers["initialize"] = func(map[string]any) (any, *plugin.RPCError) {
		return map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{"sessionCapabilities": map[string]any{"resume": map[string]any{}}}}, nil
	}
	h.acp.handlers["session/new"] = func(map[string]any) (any, *plugin.RPCError) {
		return map[string]any{"sessionId": "ses_new", "configOptions": options("other/model", "default")}, nil
	}
	h.acp.handlers["session/resume"] = func(p map[string]any) (any, *plugin.RPCError) {
		if p["sessionId"] != "ses_saved" {
			return nil, &plugin.RPCError{Code: -32602, Message: "Invalid params: session not found: " + p["sessionId"].(string)}
		}
		return map[string]any{"configOptions": options("other/model", "default")}, nil
	}
	h.acp.handlers["session/set_config_option"] = func(p map[string]any) (any, *plugin.RPCError) {
		return map[string]any{"configOptions": options(p["value"].(string), p["value"].(string))}, nil
	}
}

func TestBegin(t *testing.T) {
	args := harness.StartArgs{Hypercube: "/bin/hc", Env: map[string]string{"B": "2", "A": "1"}, Model: "p/m", Effort: "high", Dir: "/work"}
	t.Run("new session", func(t *testing.T) {
		h := connected(t)
		defaultHandlers(h, true)
		if err := h.begin(context.Background(), args); err != nil {
			t.Fatal(err)
		}
		init := h.acp.sent("initialize")[0]
		caps := init["clientCapabilities"].(map[string]any)
		if init["protocolVersion"] != float64(1) || caps["elicitation"].(map[string]any)["form"] == nil {
			t.Fatal(init)
		}
		session := h.acp.sent("session/new")[0]
		server := session["mcpServers"].([]any)[0].(map[string]any)
		env, _ := json.Marshal(server["env"])
		if session["cwd"] != "/work" || server["name"] != "hypercube" || server["command"] != "/bin/hc" || server["args"].([]any)[0] != "mcp" ||
			string(env) != `[{"name":"A","value":"1"},{"name":"B","value":"2"}]` || server["type"] != nil {
			t.Fatal(session)
		}
		set := h.acp.sent("session/set_config_option")
		if len(set) != 2 || set[0]["configId"] != "model" || set[0]["value"] != "p/m" || set[0]["sessionId"] != "ses_new" || set[1]["configId"] != "effort" || set[1]["value"] != "high" {
			t.Fatal(set)
		}
		if ev := h.snapshot(); len(ev) != 1 || ev[0].Conversation != "ses_new" {
			t.Fatal(ev)
		}
	})
	t.Run("resumed session", func(t *testing.T) {
		h := connected(t)
		defaultHandlers(h, true)
		h.session = "ses_saved"
		a := args
		a.Conversation = "ses_saved"
		if err := h.begin(context.Background(), a); err != nil {
			t.Fatal(err)
		}
		resumed := h.acp.sent("session/resume")
		if len(resumed) != 1 || resumed[0]["sessionId"] != "ses_saved" || resumed[0]["cwd"] != "/work" || len(h.acp.sent("session/new")) != 0 {
			t.Fatal(resumed)
		}
		// OpenCode forgets the effort of a resumed session.
		if set := h.acp.sent("session/set_config_option"); len(set) != 2 || set[1]["value"] != "high" {
			t.Fatal(set)
		}
	})
	t.Run("a session that cannot be resumed", func(t *testing.T) {
		h := connected(t)
		defaultHandlers(h, true)
		h.session = "ses_gone"
		a := args
		a.Conversation = "ses_gone"
		err := h.begin(context.Background(), a)
		if err == nil || !strings.Contains(err.Error(), "could not resume session ses_gone") || !strings.Contains(err.Error(), "session not found") {
			t.Fatal(err)
		}
		if len(h.acp.sent("session/new")) != 0 {
			t.Fatal("a failed resume started a new session")
		}
	})
	t.Run("grouped models", func(t *testing.T) {
		h := connected(t)
		defaultHandlers(h, true)
		h.acp.handlers["session/new"] = func(map[string]any) (any, *plugin.RPCError) {
			return map[string]any{"sessionId": "ses_new", "configOptions": []any{map[string]any{"id": "model", "type": "select", "name": "Model", "currentValue": "p/m", "options": []any{
				map[string]any{"group": "g", "name": "G", "options": []any{map[string]any{"value": "p/m", "name": "m"}, map[string]any{"value": "p/grouped", "name": "grouped"}}},
			}}}}, nil
		}
		a := args
		a.Model = "p/grouped"
		if err := h.begin(context.Background(), a); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a model that is not offered", func(t *testing.T) {
		h := connected(t)
		defaultHandlers(h, true)
		a := args
		a.Model = "p/unknown"
		err := h.begin(context.Background(), a)
		if err == nil || !strings.Contains(err.Error(), `does not offer model "p/unknown"`) {
			t.Fatal(err)
		}
		if len(h.acp.sent("session/set_config_option")) != 0 {
			t.Fatal("selected a model OpenCode does not offer")
		}
	})
	t.Run("an effort that is not offered", func(t *testing.T) {
		h := connected(t)
		defaultHandlers(h, true)
		a := args
		a.Effort = "max"
		err := h.begin(context.Background(), a)
		if err == nil || !strings.Contains(err.Error(), `does not offer effort "max"`) || !strings.Contains(err.Error(), "p/m") {
			t.Fatal(err)
		}
	})
	t.Run("a model without effort levels", func(t *testing.T) {
		h := connected(t)
		defaultHandlers(h, false)
		a := args
		a.Effort = "default"
		if err := h.begin(context.Background(), a); err != nil {
			t.Fatal(err)
		}
		if set := h.acp.sent("session/set_config_option"); len(set) != 1 {
			t.Fatal(set)
		}
		h = connected(t)
		defaultHandlers(h, false)
		if err := h.begin(context.Background(), args); err == nil || !strings.Contains(err.Error(), "no effort setting") {
			t.Fatal(err)
		}
	})
	t.Run("an old OpenCode", func(t *testing.T) {
		h := connected(t)
		defaultHandlers(h, true)
		h.acp.handlers["initialize"] = func(map[string]any) (any, *plugin.RPCError) {
			return map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{}}, nil
		}
		a := args
		a.Conversation = "ses_saved"
		if err := h.begin(context.Background(), a); err == nil || !strings.Contains(err.Error(), "OpenCode 2.x is required") {
			t.Fatal(err)
		}
	})
}

// OpenCode refuses a prompt while one runs, so input during a turn waits for
// the turn to end. The agent is idle only when nothing is in flight or waiting.
func TestInputDuringATurnWaits(t *testing.T) {
	h := connected(t)
	h.session = "ses_1"
	prompts := func() [][]string {
		var out [][]string
		for _, p := range h.acp.sent("session/prompt") {
			var texts []string
			for _, b := range p["prompt"].([]any) {
				texts = append(texts, b.(map[string]any)["text"].(string))
			}
			out = append(out, texts)
		}
		return out
	}
	if err := h.input("first", nil); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the first prompt", func() bool { return len(prompts()) == 1 })
	if err := h.input("second", nil); err != nil {
		t.Fatal(err)
	}
	if err := h.input("third", nil); err != nil {
		t.Fatal(err)
	}
	h.sync(t)
	if got := prompts(); len(got) != 1 || !slices.Equal(got[0], []string{"first"}) {
		t.Fatal("sent a prompt during a turn:", got)
	}
	h.endTurn(t, "end_turn")
	eventually(t, "the queued prompt", func() bool { return len(prompts()) == 2 })
	if got := prompts()[1]; !slices.Equal(got, []string{"second", "third"}) {
		t.Fatal(got)
	}
	if got := h.states(); !slices.Equal(got, []string{harness.Working}) {
		t.Fatal("idle between turns:", got)
	}
	if p := h.acp.sent("session/prompt")[0]; p["sessionId"] != "ses_1" {
		t.Fatal(p)
	}
	h.endTurn(t, "end_turn")
	if got := h.states(); !slices.Equal(got, []string{harness.Working, harness.Idle}) {
		t.Fatal(got)
	}
	// The next input starts a turn of its own.
	if err := h.input("fourth", nil); err != nil {
		t.Fatal(err)
	}
	h.endTurn(t, "end_turn")
	if got := prompts(); len(got) != 3 || !slices.Equal(got[2], []string{"fourth"}) {
		t.Fatal(got)
	}
	if got := h.states(); !slices.Equal(got, []string{harness.Working, harness.Idle, harness.Working, harness.Idle}) {
		t.Fatal(got)
	}
}

func TestInterruptCancelsAForm(t *testing.T) {
	h := connected(t)
	lines := fixture(t, "question")
	h.adopt(t, lines)
	h.startTurn(t)
	h.feed(t, lines...)
	if err := h.interrupt(); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the cancel", func() bool { return len(h.acp.sent("session/cancel")) == 1 })
	// One answer for the whole form, though it has two questions.
	var r []map[string]any
	eventually(t, "the form's answer", func() bool { r = h.acp.responses(); return len(r) == 1 })
	h.sync(t)
	if r = h.acp.responses(); len(r) != 1 || r[0]["result"].(map[string]any)["action"] != "cancel" {
		t.Fatal(r)
	}
	if got := h.acp.sent("session/cancel")[0]["sessionId"]; got != h.session {
		t.Fatal(got)
	}
	for _, id := range []string{"form:0/0", "form:0/1"} {
		if it := h.item(t, id); !it.Done {
			t.Fatalf("%+v", it)
		}
	}
	h.endTurn(t, "cancelled")
	if err := h.interrupt(); err != nil || len(h.acp.sent("session/cancel")) != 1 {
		t.Fatal("interrupted an idle agent", err)
	}
}

func TestInputWhenNotRunning(t *testing.T) {
	h := connected(t)
	h.acp.out.(io.Closer).Close()
	<-h.rpc.Done()
	if err := h.input("hello", nil); err == nil {
		t.Fatal("input to a process that has ended")
	}
}

func TestStartWithoutOpenCode(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := startAgent(harness.StartArgs{Dir: t.TempDir()}, func(harness.Event) {})
	if err == nil || !strings.Contains(err.Error(), "OpenCode is missing") {
		t.Fatal(err)
	}
}

// startAgent runs opencode in the agent's directory with its instructions as
// a file, and reports a process that cannot start its server.
func TestStartRunsOpenCodeAcp(t *testing.T) {
	bin := t.TempDir()
	dir := t.TempDir()
	state := t.TempDir()
	script := "#!/bin/sh\npwd > \"$FAKE_OUT/pwd\"\nprintf '%s\\n' \"$@\" > \"$FAKE_OUT/args\"\nprintf '%s' \"$OPENCODE_CONFIG_CONTENT\" > \"$FAKE_OUT/config\"\necho 'cannot start' >&2\nexit 3\n"
	if err := os.WriteFile(filepath.Join(bin, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("FAKE_OUT", state)
	_, err := startAgent(harness.StartArgs{Dir: dir, Session: "a1", Instructions: "Be brief.", StateDir: state}, func(harness.Event) {})
	if err == nil || !strings.Contains(err.Error(), "cannot start") {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(filepath.Join(state, "args"))
	config, _ := os.ReadFile(filepath.Join(state, "config"))
	pwd, _ := os.ReadFile(filepath.Join(state, "pwd"))
	file := filepath.Join(state, "opencode-instructions", "a1.md")
	if strings.TrimSpace(string(args)) != "acp" || string(config) != `{"instructions":["`+file+`"]}` || filepath.Base(strings.TrimSpace(string(pwd))) != filepath.Base(dir) {
		t.Fatalf("%q %q %q", args, config, pwd)
	}
	if text, err := os.ReadFile(file); err != nil || string(text) != "Be brief." {
		t.Fatal(err, string(text))
	}
	if errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestHypercubeToolNamesItsServer(t *testing.T) {
	h := connected(t)
	h.agent.session = "s1"
	h.feed(t,
		`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"tool_call","toolCallId":"c1","name":"hypercube_create_session","status":"pending","rawInput":{"name":"auditor"}}}}`,
		`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"tool_call","toolCallId":"c2","name":"shell","status":"pending","rawInput":{}}}}`)
	items := h.transcript()
	if len(items) != 2 || items[0].Server != "hypercube" || items[0].Tool != "create_session" || items[1].Server != "" || items[1].Tool != "shell" {
		t.Fatalf("%+v", items)
	}
}

func TestInputImagesFollowTextInOrderAndQueue(t *testing.T) {
	h := connected(t)
	h.session = "ses_1"
	png := harness.Image{MIMEType: "image/png", Data: "AAAA"}
	jpg := harness.Image{MIMEType: "image/jpeg", Data: "BBBB"}
	blocks := func(i int) []string {
		var out []string
		for _, b := range h.acp.sent("session/prompt")[i]["prompt"].([]any) {
			m := b.(map[string]any)
			if m["type"] == "image" {
				out = append(out, m["mimeType"].(string)+":"+m["data"].(string))
			} else {
				out = append(out, "text:"+m["text"].(string))
			}
		}
		return out
	}
	if err := h.input("look", []harness.Image{png}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the first prompt", func() bool { return len(h.acp.sent("session/prompt")) == 1 })
	if err := h.input("", []harness.Image{jpg}); err != nil {
		t.Fatal(err)
	}
	if err := h.input("and this", []harness.Image{png}); err != nil {
		t.Fatal(err)
	}
	h.endTurn(t, "end_turn")
	eventually(t, "the queued prompt", func() bool { return len(h.acp.sent("session/prompt")) == 2 })
	if got := blocks(0); !slices.Equal(got, []string{"text:look", "image/png:AAAA"}) {
		t.Fatal(got)
	}
	if got := blocks(1); !slices.Equal(got, []string{"image/jpeg:BBBB", "text:and this", "image/png:AAAA"}) {
		t.Fatal(got)
	}
}

func TestToolResultImages(t *testing.T) {
	h := connected(t)
	h.session = "ses_1"
	h.feed(t,
		`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"ses_1","update":{"sessionUpdate":"tool_call","toolCallId":"c1","name":"read","status":"pending","rawInput":{}}}}`,
		`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"ses_1","update":{"sessionUpdate":"tool_call_update","toolCallId":"c1","status":"in_progress"}}}`,
		`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"ses_1","update":{"sessionUpdate":"tool_call_update","toolCallId":"c1","status":"completed","content":[{"type":"content","content":{"type":"text","text":"shown"}},{"type":"content","content":{"type":"image","mimeType":"image/png","data":"AAAA"}}]}}}`,
		`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"ses_1","update":{"sessionUpdate":"tool_call_update","toolCallId":"c1","status":"completed","content":[{"type":"content","content":{"type":"image","mimeType":"image/png","data":"AAAA"}}]}}}`,
	)
	it := h.item(t, "c1")
	if it.Output != "shown" || !it.Done || len(it.Images) != 1 || it.Images[0] != (harness.Image{MIMEType: "image/png", Data: "AAAA"}) {
		t.Fatalf("%+v", it)
	}
}

func TestIdleReportsTheMessageTheTurnEndedOn(t *testing.T) {
	for _, tc := range []struct {
		name   string
		lines  []string
		reason string
		final  []string
	}{
		{"progress then response", []string{
			update("agent_message_chunk", "m1", "Looking.", "", ""), update("tool_call", "", "", "c1", ""),
			update("agent_message_chunk", "m2", "Done.", "", ""),
		}, "end_turn", []string{"m2"}},
		{"progress message then response with no tool call between", []string{
			update("agent_message_chunk", "m1", "Looking.", "", ""),
			update("agent_message_chunk", "m2", "Done.", "", ""),
		}, "end_turn", []string{"m2"}},
		{"progress before a tool call is not a response", []string{
			update("agent_message_chunk", "m1", "Looking.", "", ""), update("tool_call", "", "", "c1", ""),
		}, "end_turn", nil},
		{"a subagent's message is not a candidate", []string{
			update("agent_message_chunk", "m1", "Mine.", "", ""), update("tool_call", "", "", "c1", ""),
			update("agent_message_chunk", "child1", "Theirs.", "", "ses_c"),
		}, "end_turn", nil},
		{"cancelled", []string{update("agent_message_chunk", "m1", "Done.", "", "")}, "cancelled", nil},
		{"output limit", []string{update("agent_message_chunk", "m1", "Cut", "", "")}, "max_tokens", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := connected(t)
			h.session = "ses_1"
			h.startTurn(t)
			h.feed(t, tc.lines...)
			h.endTurn(t, tc.reason)
			if got := h.finals(); len(got) != 1 || !slices.Equal(got[0], tc.final) {
				t.Errorf("final %v, want %v", got, tc.final)
			}
		})
	}
}

func TestNativeSubagentFixtureAnswersWithTheMainMessage(t *testing.T) {
	h := connected(t)
	lines := fixture(t, "nested")
	h.adopt(t, lines)
	h.startTurn(t)
	h.feed(t, lines...)
	got := h.finals()
	if len(got) != 1 || !slices.Equal(got[0], []string{"msg_1958a4c6e001aeweRFBa7e3ZRV"}) {
		t.Fatalf("finals %v", got)
	}
}

func TestQueuedPromptsContinueOneRunWithoutAnIntermediateResponse(t *testing.T) {
	h := connected(t)
	h.session = "ses_1"
	h.startTurn(t)
	if err := h.input("more", nil); err != nil {
		t.Fatal(err)
	}
	h.feed(t, update("agent_message_chunk", "m1", "First answer.", "", ""))
	h.endTurn(t, "end_turn")
	eventually(t, "the queued prompt", func() bool { return len(h.acp.sent("session/prompt")) == 2 })
	h.endTurn(t, "end_turn")
	got := h.finals()
	if len(got) != 1 || got[0] != nil {
		t.Fatalf("finals %v", got)
	}
	// A queued turn's own response is reported when it ends the run.
	if err := h.input("again", nil); err != nil {
		t.Fatal(err)
	}
	if err := h.input("last", nil); err != nil {
		t.Fatal(err)
	}
	h.feed(t, update("agent_message_chunk", "m2", "Old.", "", ""))
	h.endTurn(t, "end_turn")
	eventually(t, "the queued prompt", func() bool { return len(h.acp.sent("session/prompt")) == 4 })
	h.feed(t, update("agent_message_chunk", "m3", "New.", "", ""))
	h.endTurn(t, "end_turn")
	if got := h.finals(); len(got) != 2 || !slices.Equal(got[1], []string{"m3"}) {
		t.Fatalf("finals %v", got)
	}
}
