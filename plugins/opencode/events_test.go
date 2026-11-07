package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
)

func TestNativeTranscriptAndChildIsolation(t *testing.T) {
	h := connected(t)
	h.feed(t, "session.execution.started", nil)
	h.feed(t, "session.text.delta", map[string]any{"assistantMessageID": "m1", "delta": "progress"})
	h.feed(t, "session.text.ended", map[string]any{"assistantMessageID": "m1", "text": "progress"})
	h.feed(t, "session.tool.input.started", map[string]any{"assistantMessageID": "m1", "id": "call", "name": "hypercube_open_file"})
	h.feed(t, "session.tool.called", map[string]any{"assistantMessageID": "m1", "id": "call", "input": map[string]string{"path": "a.png"}})
	h.feed(t, "session.tool.success", map[string]any{"assistantMessageID": "m1", "id": "call", "content": []any{map[string]string{"type": "text", "text": "picture"}, map[string]string{"type": "file", "uri": "data:image/png;base64,AAAA", "mime": "image/png"}}})
	h.feed(t, "session.text.delta", map[string]any{"sessionID": "child", "assistantMessageID": "child", "delta": "not ours"})
	h.feed(t, "session.execution.succeeded", map[string]any{"sessionID": "child"})
	if len(h.states()) != 1 || h.item("child:text:0").ID != "" {
		t.Fatal("child entered main session")
	}
	h.feed(t, "session.step.started", map[string]any{"assistantMessageID": "m2"})
	h.feed(t, "session.reasoning.delta", map[string]any{"assistantMessageID": "m2", "delta": "think"})
	h.feed(t, "session.reasoning.ended", map[string]any{"assistantMessageID": "m2", "text": "thinking"})
	h.feed(t, "session.text.delta", map[string]any{"assistantMessageID": "m2", "delta": "hel"})
	h.feed(t, "session.text.delta", map[string]any{"assistantMessageID": "m2", "delta": "lo"})
	h.feed(t, "session.text.ended", map[string]any{"assistantMessageID": "m2", "text": "hello"})
	h.feed(t, "session.step.ended", map[string]any{"finish": "stop", "tokens": map[string]any{"input": 100, "output": 10, "cache": map[string]int{"read": 20, "write": 5}}})
	h.feed(t, "session.usage.updated", map[string]any{"cost": 1.25, "tokens": map[string]any{"input": 1000, "output": 100}})
	h.feed(t, "session.execution.succeeded", nil)
	if it := h.item("m2:text:0"); it.Text != "hello" || !it.Done {
		t.Fatal(it)
	}
	if it := h.item("m2:reasoning:0"); it.Text != "thinking" || !it.Done {
		t.Fatal(it)
	}
	if it := h.item("m1:call"); !it.Done || it.Server != "hypercube" || it.Tool != "open_file" || it.Output != "picture" || len(it.Images) != 1 {
		t.Fatal(it)
	}
	ev := h.snapshot()
	if !reflect.DeepEqual(ev[len(ev)-1].Final, []string{"m2:text:0"}) {
		t.Fatal(ev[len(ev)-1])
	}
	var usage *harness.Usage
	for _, e := range ev {
		if e.Usage != nil {
			usage = e.Usage
		}
	}
	if usage == nil || usage.Context != 135 || usage.Cost != 1.25 {
		t.Fatal(usage)
	}
}

func TestNativeCompletionErrorsAndPendingInput(t *testing.T) {
	for _, tc := range []struct {
		kind, finish, message string
		final                 bool
	}{
		{"session.execution.succeeded", "stop", "", true},
		{"session.execution.succeeded", "length", "output limit", false},
		{"session.execution.succeeded", "content-filter", "refused", false},
		{"session.execution.failed", "", "overloaded", false},
		{"session.execution.interrupted", "", "", false},
	} {
		t.Run(tc.kind+tc.finish, func(t *testing.T) {
			h := connected(t)
			h.feed(t, "session.execution.started", nil)
			h.feed(t, "session.text.delta", map[string]any{"assistantMessageID": "m", "delta": "partial"})
			h.feed(t, "session.step.ended", map[string]any{"finish": tc.finish})
			h.feed(t, "session.inbox.enqueued", map[string]any{"inboxID": "pending"})
			h.feed(t, tc.kind, map[string]any{"error": map[string]string{"message": "overloaded"}})
			ev := h.snapshot()
			last := ev[len(ev)-1]
			if !last.InputPending || last.State != harness.Idle || (len(last.Final) > 0) != tc.final || !h.item("m:text:0").Done {
				t.Fatal(last)
			}
			var message string
			for _, it := range last.Items {
				if it.Kind == harness.KindError {
					message = it.Text
				}
			}
			if tc.message != "" && !strings.Contains(message, tc.message) || tc.message == "" && message != "" {
				t.Fatalf("error %q", message)
			}
		})
	}
}

func TestQuestionsStayOpenDuringCorrections(t *testing.T) {
	h := connected(t)
	h.feed(t, "form.created", map[string]any{"form": map[string]any{"id": "f1", "sessionID": "s1", "title": "Choose", "fields": []any{
		map[string]any{"key": "one", "type": "string", "title": "First?", "custom": true, "options": []any{map[string]string{"label": "Yes", "value": "yes", "description": "go"}}},
		map[string]any{"key": "many", "type": "multiselect", "title": "Which?", "custom": true, "options": []any{map[string]string{"label": "A", "value": "a"}, map[string]string{"label": "B", "value": "b"}}},
	}}})
	if err := h.input("correction", nil); err != nil {
		t.Fatal(err)
	}
	if h.item("form:f1/0").Done || len(h.sent("/api/session/s1/form/f1")) != 0 {
		t.Fatal("correction dismissed a question")
	}
	if err := h.answer("form:f1/0", []string{"Yes"}, ""); err != nil {
		t.Fatal(err)
	}
	if len(h.sent("/api/session/s1/form/f1/reply")) != 0 {
		t.Fatal("answered an incomplete form")
	}
	if err := h.answer("form:f1/1", []string{"A", "B"}, "extra"); err != nil {
		t.Fatal(err)
	}
	got := h.sent("/api/session/s1/form/f1/reply")[0].Body["answer"].(map[string]any)
	if got["one"] != "yes" || !reflect.DeepEqual(got["many"], []any{"a", "b", "extra"}) {
		t.Fatal(got)
	}
	if !h.item("form:f1/1").Done {
		t.Fatal("answer left the question open")
	}
	if err := h.answer("form:f1/1", nil, "again"); err == nil {
		t.Fatal("answered twice")
	}
}

func TestPermissionIsAllowedOnce(t *testing.T) {
	h := connected(t)
	h.feed(t, "permission.asked", map[string]any{"id": "permission"})
	got := h.sent("/api/session/s1/permission/permission/reply")
	if len(got) != 1 || got[0].Body["decision"] != "once" {
		t.Fatal(got)
	}
	h.feed(t, "permission.asked", map[string]any{"sessionID": "child", "id": "child-permission"})
	if len(h.sent("/api/session/child/permission/child-permission/reply")) != 1 {
		t.Fatal("native child permission was left waiting")
	}
}

func TestSSEFrames(t *testing.T) {
	var types []string
	err := readEvents(strings.NewReader(": heartbeat\r\n\r\nevent: native\ndata: {\"type\":\"server.connected\",\ndata: \"data\":{}}\n\n"), func(e nativeEvent) error { types = append(types, e.Type); return nil })
	if err != io.ErrUnexpectedEOF || !reflect.DeepEqual(types, []string{"server.connected"}) {
		t.Fatal(types, err)
	}
	if err := readEvents(strings.NewReader("data: invalid\n\n"), func(nativeEvent) error { return nil }); err == nil {
		t.Fatal("accepted invalid JSON")
	}
	if err := readEvents(strings.NewReader("data: {\"type\":\"session.instructions.updated\",\"data\":{\"delta\":{\"key\":\"hash\"}}}\n\n"), func(nativeEvent) error { return nil }); err != io.ErrUnexpectedEOF {
		t.Fatalf("unrelated event payload broke the stream: %v", err)
	}
}

func TestAnswerFailureLeavesQuestionOpen(t *testing.T) {
	h := connected(t)
	h.feed(t, "form.created", map[string]any{"form": map[string]any{"id": "f", "sessionID": "s1", "fields": []any{map[string]any{"key": "q", "type": "string"}}}})
	h.reply = func(w http.ResponseWriter, r *http.Request, _ map[string]any) bool {
		if strings.HasSuffix(r.URL.Path, "/reply") {
			w.WriteHeader(409)
			fmt.Fprint(w, `{"message":"try again"}`)
			return true
		}
		return false
	}
	if err := h.answer("form:f/0", nil, "value"); err == nil {
		t.Fatal("lost reply failure")
	}
	if h.item("form:f/0").Done {
		t.Fatal("failed reply closed question")
	}
	h.reply = nil
	if err := h.answer("form:f/0", nil, "value"); err != nil {
		t.Fatal(err)
	}
}

func TestRecordedNativeSteeringEvents(t *testing.T) {
	// These are the public payload shapes observed with OpenCode 2.1.4.
	h := connected(t)
	for _, line := range []string{
		`{"type":"session.tool.input.started","data":{"sessionID":"s1","assistantMessageID":"m","id":"call","name":"shell"}}`,
		`{"type":"session.tool.success","data":{"sessionID":"s1","assistantMessageID":"m","id":"call","content":[{"type":"text","text":"TOOL_FINISHED\n"}],"executed":false}}`,
	} {
		var e nativeEvent
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatal(err)
		}
		if err := h.handle(e); err != nil {
			t.Fatal(err)
		}
	}
	if it := h.item("m:call"); !it.Done || it.Failed || it.Output != "TOOL_FINISHED\n" {
		t.Fatal(it)
	}
}
