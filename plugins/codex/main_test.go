package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

// The fixtures in testdata are what `codex app-server` sent while a client
// drove it: the notifications and requests of one turn, without the
// connection's housekeeping. approval.jsonl is a turn that asked for approval.

func fixture(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// appServer stands in for codex: it answers the agent's requests from handlers
// and records everything the agent writes.
type appServer struct {
	t        *testing.T
	out      io.Writer // to the agent
	mu       sync.Mutex
	wrote    []map[string]any
	handlers map[string]func(params map[string]any) (any, *plugin.RPCError)
}

// agentUnderTest is an agent connected to a stand-in app-server instead of a
// process.
type agentUnderTest struct {
	*agent
	server *appServer

	evMu   sync.Mutex
	events []harness.Event
}

func newAgent(t *testing.T) *agentUnderTest {
	t.Helper()
	agentIn, peerOut := io.Pipe()
	peerIn, agentOut := io.Pipe()
	h := &agentUnderTest{server: &appServer{t: t, out: peerOut, handlers: map[string]func(map[string]any) (any, *plugin.RPCError){}}}
	h.agent = &agent{
		finished: map[string]bool{}, open: map[string]harness.Item{}, running: map[string]harness.Item{},
		children: map[string]string{},
	}
	h.ctx, h.cancel = context.WithCancel(context.Background())
	h.emit = func(ev harness.Event) {
		h.evMu.Lock()
		defer h.evMu.Unlock()
		h.events = append(h.events, ev)
	}
	h.rpc = plugin.NewRPC(agentOut, h.handle)
	go func() { h.rpc.End(plugin.ReadLines(agentIn, h.rpc.Line)) }()
	go h.server.serve(peerIn)
	t.Cleanup(func() { h.cancel(); peerOut.Close(); peerIn.Close() })
	return h
}

func (p *appServer) serve(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		var m map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
			p.t.Error(err)
			return
		}
		method, _ := m["method"].(string)
		p.mu.Lock()
		p.wrote = append(p.wrote, m)
		handler := p.handlers[method]
		p.mu.Unlock()
		id, hasID := m["id"]
		if method == "" || !hasID {
			continue
		}
		params, _ := m["params"].(map[string]any)
		// The app-server's messages carry no "jsonrpc" field.
		reply := map[string]any{"id": id}
		if handler == nil {
			reply["error"] = map[string]any{"code": -32601, "message": "no handler for " + method}
		} else if result, rpcErr := handler(params); rpcErr != nil {
			reply["error"] = rpcErr
		} else {
			reply["result"] = result
		}
		line, _ := json.Marshal(reply)
		_, _ = p.out.Write(append(line, '\n'))
	}
}

func (p *appServer) handle(method string, f func(params map[string]any) (any, *plugin.RPCError)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[method] = f
}

// sent returns what the agent has written so far with the given method.
func (p *appServer) sent(method string) []map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []map[string]any
	for _, m := range p.wrote {
		if m["method"] == method {
			params, _ := m["params"].(map[string]any)
			out = append(out, params)
		}
	}
	return out
}

// responses returns the results and errors the agent wrote to requests.
func (p *appServer) responses() []map[string]any {
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

// await returns the responses the agent has written once there are n.
func (p *appServer) await(n int) []map[string]any {
	p.t.Helper()
	eventually(p.t, "the agent's responses", func() bool { return len(p.responses()) >= n })
	return p.responses()
}

// compact is v as compact JSON. A value decoded from JSON always marshals.
func compact(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
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

// feed handles recorded lines directly, as codex would send them.
func (h *agentUnderTest) feed(t *testing.T, lines ...string) {
	t.Helper()
	for _, line := range lines {
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatal(err)
		}
		h.handle(plugin.Message{ID: m.ID, Method: m.Method, Params: m.Params})
	}
}

// adopt takes the thread ID a fixture was recorded with.
func (h *agentUnderTest) adopt(t *testing.T, lines []string) {
	t.Helper()
	var m struct {
		Params struct {
			ThreadID string `json:"threadId"`
		} `json:"params"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatal(err)
	}
	h.agent.thread = m.Params.ThreadID
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

func kinds(items []harness.Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Kind)
	}
	return out
}

func TestTextAndTool(t *testing.T) {
	h := newAgent(t)
	lines := fixture(t, "tool")
	h.adopt(t, lines)
	h.feed(t, lines...)
	items := h.transcript()
	if got := kinds(items); !slices.Equal(got, []string{harness.KindTool, harness.KindText}) {
		t.Fatalf("%+v", items)
	}
	tl := items[0]
	if tl.Tool != "shell" || !strings.Contains(string(tl.Input), "sleep 6; echo hi") || tl.Output != "hi\n" || tl.Failed || !tl.Done {
		t.Fatalf("%+v", tl)
	}
	// The streamed pieces and the completed text make one item.
	if text := items[1]; text.Text != "done banana" || !text.Done || text.Parent != "" {
		t.Fatalf("%+v", text)
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
	if usage.Context != 18501 || usage.Window != 475000 || usage.Cost != 0 {
		t.Fatalf("%+v", usage)
	}
}

// Codex may report a completion more than once; later reports change nothing.
func TestDuplicateCompletedItems(t *testing.T) {
	h := newAgent(t)
	lines := fixture(t, "tool")
	h.adopt(t, lines)
	h.feed(t, lines[:len(lines)-1]...)
	before, events := h.transcript(), len(h.snapshot())
	h.feed(t, lines[8], lines[13], lines[3], lines[11]) // both completions, then a start and a delta after them
	if after := h.transcript(); !slices.EqualFunc(before, after, func(a, b harness.Item) bool {
		return a.ID == b.ID && a.Text == b.Text && a.Output == b.Output && a.Done == b.Done
	}) || len(h.snapshot()) != events {
		t.Fatalf("%+v", after)
	}
}

func TestFailedTurnReportsItsError(t *testing.T) {
	h := newAgent(t)
	h.thread = "t1"
	h.feed(t,
		`{"method":"turn/started","params":{"threadId":"t1","turn":{"id":"u1","status":"inProgress"}}}`,
		`{"method":"turn/completed","params":{"threadId":"t1","turn":{"id":"u1","status":"failed","error":{"message":"model unavailable","additionalDetails":"try later"}}}}`)
	items := h.transcript()
	if len(items) != 1 || items[0].Kind != harness.KindError || items[0].Text != "model unavailable\ntry later" {
		t.Fatalf("%+v", items)
	}
	if got := h.states(); !slices.Equal(got, []string{harness.Working, harness.Idle}) {
		t.Fatal(got)
	}
}

func TestReasoningStreamsAndCompletes(t *testing.T) {
	h := newAgent(t)
	h.thread = "t1"
	h.feed(t,
		`{"method":"turn/started","params":{"threadId":"t1","turn":{"id":"u1","status":"inProgress"}}}`,
		`{"method":"item/reasoning/summaryTextDelta","params":{"threadId":"t1","turnId":"u1","itemId":"r1","summaryIndex":0,"delta":"First"}}`,
		`{"method":"item/reasoning/summaryPartAdded","params":{"threadId":"t1","turnId":"u1","itemId":"r1","summaryIndex":1}}`,
		`{"method":"item/reasoning/summaryTextDelta","params":{"threadId":"t1","turnId":"u1","itemId":"r1","summaryIndex":1,"delta":"Second"}}`)
	if got := h.item(t, "r1"); got.Text != "First\n\nSecond" || got.Done {
		t.Fatalf("%+v", got)
	}
	h.feed(t, `{"method":"item/completed","params":{"threadId":"t1","turnId":"u1","item":{"type":"reasoning","id":"r1","summary":["First","Second"],"content":[]}}}`)
	if got := h.item(t, "r1"); got.Text != "First\n\nSecond" || !got.Done || got.Kind != harness.KindReasoning {
		t.Fatalf("%+v", got)
	}
	// Reasoning without a summary is not shown.
	h.feed(t, `{"method":"item/completed","params":{"threadId":"t1","turnId":"u1","item":{"type":"reasoning","id":"r2","summary":[],"content":[]}}}`)
	if len(h.transcript()) != 1 {
		t.Fatalf("%+v", h.transcript())
	}
}

func TestOtherItemKinds(t *testing.T) {
	h := newAgent(t)
	h.thread = "t1"
	done := func(item string) string {
		return `{"method":"item/completed","params":{"threadId":"t1","turnId":"u1","item":` + item + `}}`
	}
	h.feed(t,
		done(`{"type":"commandExecution","id":"c1","command":"false","cwd":"/w","status":"failed","exitCode":1,"aggregatedOutput":"bad\n"}`),
		done(`{"type":"commandExecution","id":"c2","command":"rm x","cwd":"/w","status":"declined","aggregatedOutput":null,"exitCode":null}`),
		done(`{"type":"fileChange","id":"f1","status":"completed","changes":[{"path":"/w/a.go","kind":{"type":"add"},"diff":"+x"},{"path":"/w/b.go","kind":{"type":"delete"},"diff":"-y"}]}`),
		done(`{"type":"mcpToolCall","id":"m1","server":"s","tool":"t","status":"completed","arguments":{"q":1},"result":{"content":[{"type":"text","text":"found"}]},"error":null}`),
		done(`{"type":"mcpToolCall","id":"m2","server":"s","tool":"t","status":"failed","arguments":{},"result":null,"error":{"message":"boom"}}`),
		done(`{"type":"dynamicToolCall","id":"d1","namespace":"n","tool":"t","status":"completed","arguments":{},"contentItems":[{"type":"inputText","text":"ok"}],"success":false}`),
		done(`{"type":"webSearch","id":"w1","query":"go","action":null}`),
		done(`{"type":"userMessage","id":"x1","content":[]}`),
		done(`{"type":"contextCompaction","id":"x2"}`))
	if c := h.item(t, "c1"); c.Tool != "shell" || !c.Failed || c.Output != "bad\n" {
		t.Errorf("%+v", c)
	}
	if c := h.item(t, "c2"); !c.Failed || c.Output != "Declined." {
		t.Errorf("%+v", c)
	}
	if f := h.item(t, "f1"); f.Tool != "apply_patch" || f.Failed || !strings.Contains(string(f.Input), `"path":"/w/a.go, /w/b.go"`) {
		t.Errorf("%+v", f)
	}
	if m := h.item(t, "m1"); m.Server != "s" || m.Tool != "t" || m.Output != "found" || m.Failed || string(m.Input) != `{"q":1}` {
		t.Errorf("%+v", m)
	}
	if m := h.item(t, "m2"); !m.Failed || m.Output != "boom" {
		t.Errorf("%+v", m)
	}
	if d := h.item(t, "d1"); d.Tool != "n__t" || d.Output != "ok" || !d.Failed {
		t.Errorf("%+v", d)
	}
	if w := h.item(t, "w1"); w.Tool != "web_search" || !strings.Contains(string(w.Input), `"query":"go"`) {
		t.Errorf("%+v", w)
	}
	if len(h.transcript()) != 7 {
		t.Fatalf("%+v", h.transcript())
	}
}

func TestNativeSubagentItemsNestUnderTheirCall(t *testing.T) {
	h := newAgent(t)
	lines := fixture(t, "subagent")
	h.adopt(t, lines)
	h.feed(t, lines[:24]...)
	// The subagent's turn has ended, but the main turn goes on.
	if got := h.states(); !slices.Equal(got, []string{harness.Working}) {
		t.Fatal(got)
	}
	h.feed(t, lines[24:]...)
	var call harness.Item
	var nested []harness.Item
	for _, it := range h.transcript() {
		switch {
		case it.Tool == "subagent":
			call = it
		case it.Parent != "":
			nested = append(nested, it)
		}
	}
	if call.ID == "" || !call.Done || len(nested) != 2 {
		t.Fatalf("%+v", h.transcript())
	}
	for _, it := range nested {
		if it.Parent != call.ID {
			t.Errorf("%+v", it)
		}
	}
	if nested[0].Tool != "shell" || nested[1].Kind != harness.KindText || nested[1].Text == "" {
		t.Fatalf("%+v", nested)
	}
	if got := h.states(); !slices.Equal(got, []string{harness.Working, harness.Idle}) {
		t.Fatal(got)
	}
	// Only the main conversation's requests are its context.
	var usage []harness.Usage
	for _, ev := range h.snapshot() {
		if ev.Usage != nil {
			usage = append(usage, *ev.Usage)
		}
	}
	if len(usage) != 3 {
		t.Fatalf("%+v", usage)
	}
}

func TestUnknownThreadIsIgnored(t *testing.T) {
	h := newAgent(t)
	h.thread = "t1"
	h.feed(t,
		`{"method":"item/completed","params":{"threadId":"other","turnId":"u9","item":{"type":"agentMessage","id":"m9","text":"stray"}}}`,
		`{"method":"turn/completed","params":{"threadId":"other","turn":{"id":"u9","status":"completed"}}}`)
	if len(h.snapshot()) != 0 {
		t.Fatalf("%+v", h.snapshot())
	}
}

func TestInterruptEndsTheTurnWithoutAnError(t *testing.T) {
	h := newAgent(t)
	lines := fixture(t, "interrupt")
	h.adopt(t, lines)
	h.server.handle("turn/interrupt", func(map[string]any) (any, *plugin.RPCError) { return map[string]any{}, nil })
	h.feed(t, lines[:len(lines)-1]...)
	if err := h.interrupt(); err != nil {
		t.Fatal(err)
	}
	sent := h.server.sent("turn/interrupt")
	if len(sent) != 1 || sent[0]["threadId"] != h.thread || sent[0]["turnId"] == "" {
		t.Fatalf("%+v", sent)
	}
	h.feed(t, lines[len(lines)-1])
	items := h.transcript()
	if len(items) != 1 || items[0].Kind != harness.KindTool || !items[0].Done || items[0].Failed {
		t.Fatalf("%+v", items)
	}
	if got := h.states(); !slices.Equal(got, []string{harness.Working, harness.Idle}) {
		t.Fatal(got)
	}
	// With no turn running there is nothing to interrupt.
	if err := h.interrupt(); err != nil || len(h.server.sent("turn/interrupt")) != 1 {
		t.Fatal(err)
	}
}

// turnStart answers turn/start with a turn ID and records the text.
func turnStart(h *agentUnderTest, id string) {
	h.server.handle("turn/start", func(map[string]any) (any, *plugin.RPCError) {
		return map[string]any{"turn": map[string]any{"id": id, "status": "inProgress"}}, nil
	})
}

// threadStatus makes thread/read report the thread's status.
func threadStatus(h *agentUnderTest, status string) {
	h.server.handle("thread/read", func(map[string]any) (any, *plugin.RPCError) {
		return map[string]any{"thread": map[string]any{"id": "t1", "status": map[string]any{"type": status}}}, nil
	})
}

func inputText(params map[string]any) string {
	return params["input"].([]any)[0].(map[string]any)["text"].(string)
}

func TestInputStartsATurnThenSteersIt(t *testing.T) {
	h := newAgent(t)
	h.thread = "t1"
	turnStart(h, "u1")
	h.server.handle("turn/steer", func(map[string]any) (any, *plugin.RPCError) { return map[string]any{"turnId": "u1"}, nil })
	if err := h.input("first", nil); err != nil {
		t.Fatal(err)
	}
	if got := h.states(); !slices.Equal(got, []string{harness.Working}) {
		t.Fatal(got)
	}
	// The turn's own notification changes nothing.
	h.feed(t, `{"method":"turn/started","params":{"threadId":"t1","turn":{"id":"u1","status":"inProgress"}}}`)
	if err := h.input("second", nil); err != nil {
		t.Fatal(err)
	}
	starts, steers := h.server.sent("turn/start"), h.server.sent("turn/steer")
	if len(starts) != 1 || inputText(starts[0]) != "first" || len(steers) != 1 || inputText(steers[0]) != "second" || steers[0]["expectedTurnId"] != "u1" || steers[0]["threadId"] != "t1" {
		t.Fatalf("%+v %+v", starts, steers)
	}
	if got := h.states(); !slices.Equal(got, []string{harness.Working}) {
		t.Fatal(got)
	}
}

// A steer that lands after the turn ended starts the next turn, whether or not
// the turn's completion has arrived yet.
func TestSteerRejectedAfterTheTurnEnded(t *testing.T) {
	h := newAgent(t)
	h.thread = "t1"
	turnStart(h, "u2")
	threadStatus(h, "idle")
	h.server.handle("turn/steer", func(map[string]any) (any, *plugin.RPCError) {
		return nil, &plugin.RPCError{Code: -32600, Message: "no active turn to steer"}
	})
	h.feed(t, `{"method":"turn/started","params":{"threadId":"t1","turn":{"id":"u1","status":"inProgress"}}}`)
	if err := h.input("late", nil); err != nil {
		t.Fatal(err)
	}
	if starts := h.server.sent("turn/start"); len(starts) != 1 || inputText(starts[0]) != "late" {
		t.Fatalf("%+v", starts)
	}
	// The old turn's completion arrives after the new turn started.
	h.feed(t, `{"method":"turn/completed","params":{"threadId":"t1","turn":{"id":"u1","status":"completed"}}}`)
	if got := h.states(); !slices.Equal(got, []string{harness.Working, harness.Working}) {
		t.Fatal(got)
	}
	h.feed(t, `{"method":"turn/completed","params":{"threadId":"t1","turn":{"id":"u2","status":"completed"}}}`)
	if got := h.states(); !slices.Equal(got, []string{harness.Working, harness.Working, harness.Idle}) {
		t.Fatal(got)
	}
	// With the turn already complete, input starts a turn without a steer.
	turnStart(h, "u3")
	if err := h.input("again", nil); err != nil {
		t.Fatal(err)
	}
	if len(h.server.sent("turn/steer")) != 1 || len(h.server.sent("turn/start")) != 2 {
		t.Fatal(h.server.sent("turn/steer"), h.server.sent("turn/start"))
	}
}

// Only an idle thread starts a turn after a failed steer. Any other status, or
// a thread that cannot be read, returns the steer's error.
func TestSteerFailureOnALiveTurnIsReturned(t *testing.T) {
	for name, test := range map[string]struct {
		status  string // "" makes thread/read fail
		message string
	}{
		"active":                   {"active", "cannot steer a compaction turn"},
		"active with the old text": {"active", "no active turn to steer"},
		"not loaded":               {"notLoaded", "no active turn to steer"},
		"system error":             {"systemError", "no active turn to steer"},
		"unknown status":           {"paused", "no active turn to steer"},
		"unreadable thread":        {"", "no active turn to steer"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newAgent(t)
			h.thread = "t1"
			h.server.handle("turn/steer", func(map[string]any) (any, *plugin.RPCError) {
				return nil, &plugin.RPCError{Code: -32600, Message: test.message}
			})
			if test.status != "" {
				threadStatus(h, test.status)
			}
			h.feed(t, `{"method":"turn/started","params":{"threadId":"t1","turn":{"id":"u1","status":"inProgress"}}}`)
			if err := h.input("x", nil); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatal(err)
			}
			if len(h.server.sent("thread/read")) != 1 || len(h.server.sent("turn/start")) != 0 {
				t.Fatal("started a turn beside the live one")
			}
		})
	}
}

func TestStartFailureIsReturned(t *testing.T) {
	h := newAgent(t)
	h.thread = "t1"
	h.server.handle("turn/start", func(map[string]any) (any, *plugin.RPCError) {
		return nil, &plugin.RPCError{Code: -32000, Message: "not signed in"}
	})
	if err := h.input("x", nil); err == nil || !strings.Contains(err.Error(), "not signed in") {
		t.Fatal(err)
	}
	if len(h.states()) != 0 {
		t.Fatal(h.states())
	}
}

func TestInputWhenNotRunning(t *testing.T) {
	h := newAgent(t)
	h.server.out.(io.Closer).Close()
	<-h.rpc.Done()
	if err := h.input("hello", nil); err == nil {
		t.Fatal("input to a process that has ended")
	}
}

func TestApprovalBecomesAnAttentionRequest(t *testing.T) {
	h := newAgent(t)
	lines := fixture(t, "approval")
	h.adopt(t, lines)
	h.feed(t, lines...)
	var ask harness.Item
	for _, it := range h.transcript() {
		if it.Kind == harness.KindAttention {
			ask = it
		}
	}
	// The request lists the decisions it accepts, which has no decline.
	if ask.ID != "request:0/0" || ask.Done || ask.Free || !slices.Equal(ask.Choices, []string{allow, deny}) ||
		!strings.Contains(ask.Text, "echo probe > /Users/me/codex-probe-outside.txt") || !strings.Contains(ask.Text, "May I write") {
		t.Fatalf("%+v", ask)
	}
	for _, bad := range [][2]string{{"Maybe", ""}, {"", ""}, {"", "free text"}} {
		if err := h.answer(ask.ID, bad[0], bad[1]); err == nil {
			t.Fatalf("answered with %q", bad)
		}
	}
	if err := h.answer(ask.ID, deny, ""); err != nil {
		t.Fatal(err)
	}
	responses := h.server.await(1)
	if len(responses) != 1 || responses[0]["id"] != float64(0) || compact(responses[0]["result"]) != `{"decision":"cancel"}` {
		t.Fatalf("%+v", responses)
	}
	if got := h.item(t, ask.ID); !got.Done || got.Answer != deny {
		t.Fatalf("%+v", got)
	}
	// A finished request cannot be answered again.
	if err := h.answer(ask.ID, allow, ""); err == nil || !strings.Contains(err.Error(), "is not open") {
		t.Fatal(err)
	}
}

func TestApprovalDecisions(t *testing.T) {
	h := newAgent(t)
	h.thread = "t1"
	h.feed(t,
		`{"id":7,"method":"item/commandExecution/requestApproval","params":{"threadId":"t1","turnId":"u1","itemId":"i","command":"ls","kind":"command"}}`,
		`{"id":"s8","method":"item/fileChange/requestApproval","params":{"threadId":"t1","turnId":"u1","itemId":"j","grantRoot":"/w","reason":"needs it"}}`,
		`{"id":9,"method":"item/commandExecution/requestApproval","params":{"threadId":"t1","turnId":"u1","itemId":"k","command":null,"networkApprovalContext":{"host":"example.com","protocol":"https"},"availableDecisions":["accept","decline","acceptForSession"]}}`)
	cmd, file, network := h.item(t, "request:7/0"), h.item(t, "request:s8/0"), h.item(t, "request:9/0")
	if !slices.Equal(cmd.Choices, []string{allow, allowSession, deny}) || !strings.Contains(cmd.Text, "Run this command?") {
		t.Fatalf("%+v", cmd)
	}
	if !strings.Contains(file.Text, "Allow writes under /w?") || !strings.Contains(file.Text, "needs it") {
		t.Fatalf("%+v", file)
	}
	if !strings.Contains(network.Text, "network access to example.com") {
		t.Fatalf("%+v", network)
	}
	for _, a := range [][2]string{{"request:7/0", allowSession}, {"request:s8/0", allow}, {"request:9/0", deny}} {
		if err := h.answer(a[0], a[1], ""); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for _, r := range h.server.await(3) {
		got = append(got, compact(r["id"])+compact(r["result"]))
	}
	if want := []string{`7{"decision":"acceptForSession"}`, `"s8"{"decision":"accept"}`, `9{"decision":"decline"}`}; !slices.Equal(got, want) {
		t.Fatalf("%q", got)
	}
}

func TestPermissionsRequest(t *testing.T) {
	h := newAgent(t)
	h.thread = "t1"
	h.feed(t, `{"id":3,"method":"item/permissions/requestApproval","params":{"threadId":"t1","turnId":"u1","itemId":"i","cwd":"/w","reason":"to fetch","permissions":{"network":{"enabled":true}}}}`)
	ask := h.item(t, "request:3/0")
	if !strings.Contains(ask.Text, `"enabled":true`) || !strings.Contains(ask.Text, "to fetch") {
		t.Fatalf("%+v", ask)
	}
	if err := h.answer(ask.ID, allowSession, ""); err != nil {
		t.Fatal(err)
	}
	h.feed(t, `{"id":4,"method":"item/permissions/requestApproval","params":{"threadId":"t1","turnId":"u1","itemId":"i","cwd":"/w","permissions":{"network":{"enabled":true}}}}`)
	if err := h.answer("request:4/0", deny, ""); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range h.server.await(2) {
		got = append(got, compact(r["result"]))
	}
	if want := []string{`{"permissions":{"network":{"enabled":true}},"scope":"session"}`, `{"permissions":{}}`}; !slices.Equal(got, want) {
		t.Fatalf("%q", got)
	}
}

func TestQuestionsAreAnsweredTogether(t *testing.T) {
	h := newAgent(t)
	h.thread = "t1"
	h.feed(t, `{"id":5,"method":"item/tool/requestUserInput","params":{"threadId":"t1","turnId":"u1","itemId":"i","isBlocking":true,"questions":[
		{"id":"size","header":"Size","question":"Which size?","isOther":false,"isSecret":false,"options":[{"label":"Small","description":"fits"},{"label":"Large","description":""}]},
		{"id":"name","header":"Name","question":"What name?","isOther":true,"isSecret":false,"options":null},
		{"id":"color","header":"Color","question":"Which color?","isOther":true,"isSecret":false,"options":[{"label":"Red","description":""}]}]}}`)
	size, name, color := h.item(t, "request:5/0"), h.item(t, "request:5/1"), h.item(t, "request:5/2")
	if size.Free || !slices.Equal(size.Choices, []string{"Small", "Large"}) || !strings.Contains(size.Text, "Which size?") || !strings.Contains(size.Text, "- Small: fits") {
		t.Fatalf("%+v", size)
	}
	if !name.Free || len(name.Choices) != 0 || !color.Free || len(color.Choices) != 1 {
		t.Fatalf("%+v %+v", name, color)
	}
	if err := h.answer(size.ID, "", "huge"); err == nil {
		t.Fatal("typed text for a question with fixed choices")
	}
	if err := h.answer(size.ID, "Large", ""); err != nil {
		t.Fatal(err)
	}
	if err := h.answer(name.ID, "", "Ada"); err != nil {
		t.Fatal(err)
	}
	if len(h.server.responses()) != 0 {
		t.Fatal("answered before every question had an answer")
	}
	if err := h.answer(color.ID, "", "teal"); err != nil {
		t.Fatal(err)
	}
	responses := h.server.await(1)
	want := `{"answers":{"color":{"answers":["teal"]},"name":{"answers":["Ada"]},"size":{"answers":["Large"]}}}`
	if len(responses) != 1 || compact(responses[0]["result"]) != want {
		t.Fatalf("%+v", responses)
	}
}

func TestElicitationApproval(t *testing.T) {
	h := newAgent(t)
	h.thread = "t1"
	h.feed(t,
		`{"id":1,"method":"mcpServer/elicitation/request","params":{"serverName":"docs","threadId":"t1","turnId":"u1","mode":"form","message":"Allow docs to run lookup?","requestedSchema":{"type":"object","properties":{}}}}`,
		`{"id":2,"method":"mcpServer/elicitation/request","params":{"serverName":"docs","threadId":"t1","turnId":"u1","mode":"form","message":"Allow docs to run lookup?","requestedSchema":{"type":"object","properties":{}}}}`,
		`{"id":3,"method":"mcpServer/elicitation/request","params":{"serverName":"auth","threadId":"t1","mode":"url","message":"Sign in","elicitationId":"e","url":"https://example.com/login"}}`)
	ask := h.item(t, "request:1/0")
	if ask.Text != "docs: Allow docs to run lookup?" || !slices.Equal(ask.Choices, []string{"Accept", "Decline"}) || ask.Free {
		t.Fatalf("%+v", ask)
	}
	if got := h.item(t, "request:3/0").Text; !strings.Contains(got, "https://example.com/login") {
		t.Fatal(got)
	}
	for _, a := range [][2]string{{"request:1/0", "Accept"}, {"request:2/0", "Decline"}} {
		if err := h.answer(a[0], a[1], ""); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for _, r := range h.server.await(2) {
		got = append(got, compact(r["result"]))
	}
	if want := []string{`{"action":"accept","content":{}}`, `{"action":"decline"}`}; !slices.Equal(got, want) {
		t.Fatalf("%q", got)
	}
}

func TestElicitationForm(t *testing.T) {
	h := newAgent(t)
	h.thread = "t1"
	h.feed(t, `{"id":1,"method":"mcpServer/elicitation/request","params":{"serverName":"srv","threadId":"t1","mode":"form","message":"Details","requestedSchema":{"type":"object","properties":{
		"zeta":{"type":"string","title":"Mode","oneOf":[{"const":"fast","title":"Fast one"},{"const":"slow","title":"Slow one"}]},
		"alpha":{"type":"boolean","title":"Sure?"},
		"count":{"type":"number","title":"How many?"},
		"tags":{"type":"array","title":"Tags","items":{"enum":["a","b"]}},
		"note":{"type":"string","title":"Note"}}}}}`)
	zeta, alpha, count, tags, note := h.item(t, "request:1/0"), h.item(t, "request:1/1"), h.item(t, "request:1/2"), h.item(t, "request:1/3"), h.item(t, "request:1/4")
	if !strings.HasPrefix(zeta.Text, "srv: Details") || !slices.Equal(zeta.Choices, []string{"fast", "slow"}) || !strings.Contains(zeta.Text, "- fast: Fast one") {
		t.Fatalf("%+v", zeta)
	}
	if !slices.Equal(alpha.Choices, []string{"Yes", "No"}) || !count.Free || !slices.Equal(tags.Choices, []string{"a", "b"}) || !note.Free {
		t.Fatalf("%+v", h.transcript())
	}
	if err := h.answer(count.ID, "", "many"); err == nil {
		t.Fatal("accepted a number that is not one")
	}
	for _, a := range [][2]string{{zeta.ID, "slow"}, {alpha.ID, "No"}, {count.ID, "3"}, {tags.ID, "b"}, {note.ID, "hi"}} {
		text, choice := "", a[1]
		if a[0] == count.ID || a[0] == note.ID {
			text, choice = a[1], ""
		}
		if err := h.answer(a[0], choice, text); err != nil {
			t.Fatal(err)
		}
	}
	responses := h.server.await(1)
	want := `{"action":"accept","content":{"alpha":false,"count":3,"note":"hi","tags":["b"],"zeta":"slow"}}`
	if len(responses) != 1 || compact(responses[0]["result"]) != want {
		t.Fatalf("%+v", responses)
	}
}

func TestUnsupportedRequestsAreRefused(t *testing.T) {
	h := newAgent(t)
	h.thread = "t1"
	h.feed(t,
		`{"id":1,"method":"item/tool/call","params":{"threadId":"t1"}}`,
		`{"id":2,"method":"mcpServer/elicitation/request","params":{"serverName":"s","threadId":"t1","mode":"openaiForm","message":"m","requestedSchema":{}}}`,
		`{"id":3,"method":"mcpServer/elicitation/request","params":{"serverName":"s","threadId":"t1","mode":"form","message":"m","requestedSchema":{"type":"object","properties":{"x":{"type":"object"}}}}}`)
	responses := h.server.await(3)
	if len(responses) != 3 {
		t.Fatalf("%+v", responses)
	}
	for _, r := range responses {
		if r["error"] == nil {
			t.Errorf("%+v", r)
		}
	}
	items := h.transcript()
	if len(items) != 2 || items[0].Kind != harness.KindError || !strings.Contains(items[0].Text, "openaiForm") || !strings.Contains(items[1].Text, "object field") {
		t.Fatalf("%+v", items)
	}
}

func TestInterruptRefusesOpenRequests(t *testing.T) {
	h := newAgent(t)
	lines := fixture(t, "approval")
	h.adopt(t, lines)
	h.server.handle("turn/interrupt", func(map[string]any) (any, *plugin.RPCError) { return map[string]any{}, nil })
	h.feed(t, lines...)
	if err := h.interrupt(); err != nil {
		t.Fatal(err)
	}
	responses := h.server.await(1)
	if len(responses) != 1 || compact(responses[0]["result"]) != `{"decision":"cancel"}` {
		t.Fatalf("%+v", responses)
	}
	if got := h.item(t, "request:0/0"); !got.Done || got.Answer != "" {
		t.Fatalf("%+v", got)
	}
	if err := h.answer("request:0/0", allow, ""); err == nil {
		t.Fatal("answered a request that was interrupted")
	}
}

func TestResolvedAndEndedTurnsCloseRequests(t *testing.T) {
	h := newAgent(t)
	h.thread = "t1"
	ask := func(id string) string {
		return `{"id":` + id + `,"method":"item/fileChange/requestApproval","params":{"threadId":"t1","turnId":"u1","itemId":"i"}}`
	}
	h.feed(t,
		`{"method":"turn/started","params":{"threadId":"t1","turn":{"id":"u1","status":"inProgress"}}}`,
		ask("1"), ask("2"),
		`{"method":"serverRequest/resolved","params":{"threadId":"t1","requestId":1}}`)
	if !h.item(t, "request:1/0").Done || h.item(t, "request:2/0").Done {
		t.Fatalf("%+v", h.transcript())
	}
	if err := h.answer("request:1/0", allow, ""); err == nil {
		t.Fatal("answered a resolved request")
	}
	h.feed(t, `{"method":"turn/completed","params":{"threadId":"t1","turn":{"id":"u1","status":"completed"}}}`)
	if !h.item(t, "request:2/0").Done {
		t.Fatalf("%+v", h.transcript())
	}
	if len(h.server.responses()) != 0 {
		t.Fatal("responded to requests that were no longer open")
	}
}

func TestMCPStartupFailureIsShown(t *testing.T) {
	h := newAgent(t)
	h.thread = "t1"
	h.feed(t,
		`{"method":"mcpServer/startupStatus/updated","params":{"threadId":"t1","name":"other","status":"failed","error":"x"}}`,
		`{"method":"mcpServer/startupStatus/updated","params":{"threadId":"t1","name":"hypercube","status":"ready","error":null}}`,
		`{"method":"mcpServer/startupStatus/updated","params":{"threadId":"t1","name":"hypercube","status":"failed","error":"no such file"}}`)
	items := h.transcript()
	if len(items) != 1 || items[0].Kind != harness.KindError || !strings.Contains(items[0].Text, "no such file") {
		t.Fatalf("%+v", items)
	}
}

// connect starts the thread with the launch information. A repository's Git
// directories are writable.
func TestConnectStartsAThread(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	resolved, _ := filepath.EvalSymlinks(dir)
	h := newAgent(t)
	h.server.handle("initialize", func(map[string]any) (any, *plugin.RPCError) { return map[string]any{}, nil })
	h.server.handle("thread/start", func(map[string]any) (any, *plugin.RPCError) {
		return map[string]any{"thread": map[string]any{"id": "thread-1"}}, nil
	})
	args := harness.StartArgs{Hypercube: "/bin/hc", Env: map[string]string{"HYPERCUBE_AGENT_ID": "a"}, Model: "m", Effort: "low", Dir: dir, Instructions: "be useful"}
	if err := h.connect(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	if h.thread != "thread-1" || len(h.snapshot()) != 1 || h.snapshot()[0].Conversation != "thread-1" {
		t.Fatalf("%q %+v", h.thread, h.snapshot())
	}
	init := h.server.sent("initialize")[0]
	if init["clientInfo"].(map[string]any)["name"] != "hypercube" || len(h.server.sent("initialized")) != 1 {
		t.Fatalf("%+v", init)
	}
	p := h.server.sent("thread/start")[0]
	config, _ := json.Marshal(p["config"])
	wantConfig := `{"mcp_servers":{"hypercube":{"args":["mcp"],"command":"/bin/hc","default_tools_approval_mode":"approve","env":{"HYPERCUBE_AGENT_ID":"a"}}},` +
		`"model_reasoning_effort":"low","projects":{"` + resolved + `":{"trust_level":"trusted"}},` +
		`"sandbox_workspace_write":{"network_access":true,"writable_roots":["` + resolved + `/.git"]}}`
	if p["model"] != "m" || p["cwd"] != dir || p["approvalPolicy"] != "on-request" || p["approvalsReviewer"] != "auto_review" || p["sandbox"] != "workspace-write" ||
		p["developerInstructions"] != "be useful" || string(config) != wantConfig || p["threadId"] != nil || p["threadSource"] != "chatgpt_hidden" {
		t.Fatalf("%+v\n%s", p, config)
	}
}

func TestConnectResumesAThread(t *testing.T) {
	h := newAgent(t)
	h.server.handle("initialize", func(map[string]any) (any, *plugin.RPCError) { return map[string]any{}, nil })
	h.server.handle("thread/resume", func(p map[string]any) (any, *plugin.RPCError) {
		if p["threadId"] == "gone" {
			return nil, &plugin.RPCError{Code: -32600, Message: "no rollout found"}
		}
		return map[string]any{"thread": map[string]any{"id": p["threadId"]}}, nil
	})
	args := harness.StartArgs{Model: "m", Effort: "low", Dir: t.TempDir(), Conversation: "saved"}
	if err := h.connect(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	p := h.server.sent("thread/resume")[0]
	if h.thread != "saved" || p["threadId"] != "saved" || p["approvalsReviewer"] != "auto_review" || p["excludeTurns"] != true || p["threadSource"] != nil || len(h.server.sent("thread/start")) != 0 {
		t.Fatalf("%q %+v", h.thread, p)
	}
	// A conversation that cannot be resumed is an error, not a new conversation.
	args.Conversation = "gone"
	err := h.connect(context.Background(), args)
	if err == nil || !strings.Contains(err.Error(), "could not resume thread gone") || !strings.Contains(err.Error(), "no rollout found") || len(h.server.sent("thread/start")) != 0 {
		t.Fatal(err)
	}
}

func TestConnectReportsAnIncompatibleServer(t *testing.T) {
	h := newAgent(t)
	err := h.connect(context.Background(), harness.StartArgs{Dir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "update Codex") {
		t.Fatal(err)
	}
}

func TestStartWithoutCodex(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := startAgent(harness.StartArgs{Dir: t.TempDir()}, func(harness.Event) {})
	if err == nil || !strings.Contains(err.Error(), "Codex is missing") {
		t.Fatal(err)
	}
}

// startAgent runs codex in the agent's directory, and reports a process that
// ends before it is ready.
func TestStartReportsAProcessThatEnds(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\necho 'config error: bad value' >&2\nexit 3\n"
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, err := startAgent(harness.StartArgs{Dir: t.TempDir()}, func(harness.Event) {})
	if err == nil || !strings.Contains(err.Error(), "bad value") {
		t.Fatal(err)
	}
}

// The agent talks to a real process: initialize, a thread, a turn, and an
// exit that the serve loop reports.
func TestServeDrivesAProcess(t *testing.T) {
	bin := t.TempDir()
	script := `#!/bin/sh
case "$1" in app-server) ;; *) exit 9;; esac
while IFS= read -r line; do
  id=$(printf '%s' "$line" | sed -n 's/.*"id":\([0-9]*\),.*/\1/p')
  case "$line" in
    *'"method":"initialize"'*) printf '{"id":%s,"result":{}}\n' "$id";;
    *'"method":"thread/start"'*) printf '{"id":%s,"result":{"thread":{"id":"th"}}}\n' "$id";;
    *'"method":"turn/start"'*)
      printf '{"id":%s,"result":{"turn":{"id":"tu"}}}\n' "$id"
      printf '{"method":"item/completed","params":{"threadId":"th","turnId":"tu","item":{"type":"agentMessage","id":"m","text":"hello"}}}\n'
      printf '{"method":"turn/completed","params":{"threadId":"th","turn":{"id":"tu","status":"completed"}}}\n';;
  esac
done
`
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	inRead, inWrite := io.Pipe()
	outRead, outWrite := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- serve(inRead, outWrite); outWrite.Close() }()
	records := make(chan harness.Record, 64)
	go func() {
		scanner := bufio.NewScanner(outRead)
		for scanner.Scan() {
			var r harness.Record
			if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
				t.Error(err)
			}
			records <- r
		}
		close(records)
	}()
	send := func(cmd harness.Command) {
		b, _ := json.Marshal(cmd)
		if _, err := inWrite.Write(append(b, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	// until reads records until the ones seen so far satisfy ok. Events and
	// replies race, so a record is never skipped.
	var seen []harness.Record
	until := func(ok func([]harness.Record) bool) {
		t.Helper()
		timeout := time.After(10 * time.Second)
		for !ok(seen) {
			select {
			case r, open := <-records:
				if !open {
					t.Fatal("the connection closed")
				}
				seen = append(seen, r)
			case <-timeout:
				t.Fatalf("timed out; saw %+v", seen)
			}
		}
	}
	has := func(match func(harness.Record) bool) func([]harness.Record) bool {
		return func(rs []harness.Record) bool { return slices.ContainsFunc(rs, match) }
	}
	send(harness.Command{ID: "1", Command: harness.CmdStart, Start: &harness.StartArgs{Model: "m", Effort: "low", Dir: t.TempDir()}})
	until(has(func(r harness.Record) bool { return r.Reply != nil && r.Reply.ID == "1" && r.Reply.Error == "" }))
	until(has(func(r harness.Record) bool {
		return r.Event != nil && r.Event.State == harness.Idle && r.Event.Conversation == "th"
	}))
	send(harness.Command{ID: "2", Command: harness.CmdInput, Text: "hi"})
	until(has(func(r harness.Record) bool { return r.Reply != nil && r.Reply.ID == "2" && r.Reply.Error == "" }))
	until(has(func(r harness.Record) bool {
		return r.Event != nil && len(r.Event.Items) == 1 && r.Event.Items[0].Text == "hello" && r.Event.Items[0].Done
	}))
	until(has(func(r harness.Record) bool {
		return r.Event != nil && r.Event.State == harness.Idle && r.Event.Conversation == ""
	}))
	send(harness.Command{ID: "3", Command: harness.CmdShutdown})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestInputSendsImagesAsDataURLs(t *testing.T) {
	h := newAgent(t)
	h.thread = "t1"
	turnStart(h, "u1")
	h.server.handle("turn/steer", func(map[string]any) (any, *plugin.RPCError) { return map[string]any{"turnId": "u1"}, nil })
	png := harness.Image{MIMEType: "image/png", Data: "AAAA"}
	// An image alone starts the turn without an empty text element, and the
	// next input steers it with its images.
	if err := h.input("", []harness.Image{png}); err != nil {
		t.Fatal(err)
	}
	if err := h.input("more", []harness.Image{png}); err != nil {
		t.Fatal(err)
	}
	image := map[string]any{"type": "image", "url": "data:image/png;base64,AAAA"}
	start, steer := h.server.sent("turn/start")[0]["input"], h.server.sent("turn/steer")[0]["input"]
	if !reflect.DeepEqual(start, []any{image}) || !reflect.DeepEqual(steer, []any{map[string]any{"type": "text", "text": "more"}, image}) {
		t.Errorf("%v %v", start, steer)
	}
}

func TestImageOutputs(t *testing.T) {
	// A 1x1 PNG.
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/iZk9HQAAAABJRU5ErkJggg=="
	raw, err := base64.StdEncoding.DecodeString(png)
	if err != nil {
		t.Fatal(err)
	}
	saved := filepath.Join(t.TempDir(), "generated.png")
	if err := os.WriteFile(saved, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	notImage := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(notImage, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newAgent(t)
	h.thread = "t1"
	done := func(item string) string {
		return `{"method":"item/completed","params":{"threadId":"t1","turnId":"u1","item":` + item + `}}`
	}
	h.feed(t,
		done(`{"type":"imageGeneration","id":"g1","status":"completed","savedPath":"`+saved+`","result":""}`),
		done(`{"type":"imageGeneration","id":"g2","status":"completed","savedPath":null,"result":"`+png+`"}`),
		done(`{"type":"imageGeneration","id":"g3","status":"completed","fileId":"file_1","result":""}`),
		done(`{"type":"imageGeneration","id":"g4","status":"failed","failure":{"type":"usageLimitExceeded","limitId":"x"}}`),
		done(`{"type":"mcpToolCall","id":"m1","server":"s","tool":"t","status":"completed","arguments":{},"result":{"content":[{"type":"text","text":"shot"},{"type":"image","data":"`+png+`","mimeType":"image/png"},{"type":"image","data":"x","mimeType":"image/svg+xml"}]},"error":null}`),
		done(`{"type":"dynamicToolCall","id":"d1","tool":"t","status":"completed","arguments":{},"contentItems":[{"type":"inputImage","imageUrl":"data:image/jpeg;base64,/9j/"},{"type":"inputImage","imageUrl":"https://x/y.png"}],"success":true}`),
		done(`{"type":"imageView","id":"v1","path":"`+saved+`"}`),
		done(`{"type":"imageView","id":"v2","path":"`+notImage+`"}`))
	want := []harness.Image{{MIMEType: "image/png", Data: png}}
	for _, id := range []string{"g1", "g2"} {
		if it := h.item(t, id); it.Kind != harness.KindText || !it.Done || it.Text != "" || !slices.Equal(it.Images, want) {
			t.Errorf("%s: %+v", id, it)
		}
	}
	if m := h.item(t, "m1"); m.Output != "shot" || !slices.Equal(m.Images, want) {
		t.Errorf("%+v", m)
	}
	if d := h.item(t, "d1"); !slices.Equal(d.Images, []harness.Image{{MIMEType: "image/jpeg", Data: "/9j/"}}) {
		t.Errorf("%+v", d)
	}
	if v := h.item(t, "v1"); v.Tool != "view_image" || !slices.Equal(v.Images, want) {
		t.Errorf("%+v", v)
	}
	if v := h.item(t, "v2"); len(v.Images) != 0 {
		t.Errorf("%+v", v)
	}
	// A generation without an image shows nothing, and a failed one an error.
	var errs []string
	for _, it := range h.transcript() {
		switch {
		case it.ID == "g3":
			t.Errorf("shown: %+v", it)
		case it.Kind == harness.KindError:
			errs = append(errs, it.Text)
		}
	}
	if len(errs) != 1 {
		t.Errorf("errors %v", errs)
	}
	// The completion is reported once.
	h.feed(t, done(`{"type":"imageGeneration","id":"g1","status":"completed","savedPath":null,"result":""}`))
	if it := h.item(t, "g1"); len(it.Images) != 1 {
		t.Errorf("%+v", it)
	}
}
