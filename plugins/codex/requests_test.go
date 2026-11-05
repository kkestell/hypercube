package main

import (
	"fmt"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

func TestAsyncChildAnswersStayOnTheirThread(t *testing.T) {
	for _, state := range []string{"running", "idle", "ended before steer"} {
		t.Run(state, func(t *testing.T) {
			h := newAgent(t)
			h.thread = "main"
			h.selection = harness.Selection{Model: "main-model", Effort: "high"}
			h.server.handle("turn/steer", func(map[string]any) (any, *plugin.RPCError) {
				if state == "ended before steer" {
					return nil, &plugin.RPCError{Code: -32600, Message: "no active turn to steer"}
				}
				return map[string]any{}, nil
			})
			turnStart(h, "child-next")
			threadStatus(h, "idle")
			h.feed(t,
				`{"method":"turn/started","params":{"threadId":"main","turn":{"id":"main-turn"}}}`,
				`{"method":"item/started","params":{"threadId":"main","item":{"type":"subAgentActivity","kind":"started","id":"spawn","agentThreadId":"child"}}}`,
				`{"method":"turn/started","params":{"threadId":"child","turn":{"id":"child-turn"}}}`,
				`{"method":"item/completed","params":{"threadId":"main","item":{"type":"agentMessage","id":"main-question","delivery":"async","questions":[{"title":"Main question?"}]}}}`,
				`{"method":"item/completed","params":{"threadId":"child","turnId":"child-turn","item":{"type":"agentMessage","id":"child-question","delivery":"async","questions":[{"title":"Which directory?","options":["src","docs"]},{"title":"Other question?"}]}}}`)
			if state == "idle" {
				h.feed(t, `{"method":"turn/completed","params":{"threadId":"child","turn":{"id":"child-turn","status":"completed"}}}`)
			}
			if err := h.answer("question:child-question/0", "src", ""); err != nil {
				t.Fatal(err)
			}
			steers, starts := h.server.sent("turn/steer"), h.server.sent("turn/start")
			if state == "idle" {
				if len(steers) != 0 {
					t.Fatalf("idle child steered: %+v", steers)
				}
			} else if len(steers) != 1 || steers[0]["threadId"] != "child" || steers[0]["expectedTurnId"] != "child-turn" {
				t.Fatalf("steers = %+v", steers)
			}
			if state == "running" {
				if len(starts) != 0 {
					t.Fatalf("running child started again: %+v", starts)
				}
			} else if len(starts) != 1 || starts[0]["threadId"] != "child" || starts[0]["model"] != nil || starts[0]["effort"] != nil || inputText(starts[0]) != "Which directory?\n\nsrc" {
				t.Fatalf("starts = %+v", starts)
			}
			if reads := h.server.sent("thread/read"); state == "ended before steer" && (len(reads) != 1 || reads[0]["threadId"] != "child") {
				t.Fatalf("thread reads = %+v", reads)
			}
			if got := h.item(t, "question:child-question/0"); !got.Done || got.Answer != "src" {
				t.Fatalf("answered question = %+v", got)
			}
			if !h.item(t, "question:child-question/1").Done || h.item(t, "question:main-question/0").Done {
				t.Fatalf("question closures = %+v", h.transcript())
			}
			if got := h.states(); len(got) != 1 || got[0] != harness.Working || h.turn != "main-turn" {
				t.Fatalf("main state = %v, turn = %q", got, h.turn)
			}
		})
	}
}

func TestInvalidNumericFormAnswerStaysOpenForCorrection(t *testing.T) {
	for _, tc := range []struct {
		typeName string
		answer   string
	}{
		{"number", "NaN"}, {"number", "Inf"}, {"number", "-Inf"},
		{"integer", "NaN"}, {"integer", "Inf"}, {"integer", "1.5"},
	} {
		t.Run(tc.typeName+"/"+tc.answer, func(t *testing.T) {
			h := newAgent(t)
			h.thread = "main"
			h.feed(t, fmt.Sprintf(`{"id":1,"method":"mcpServer/elicitation/request","params":{"serverName":"srv","threadId":"main","mode":"form","message":"Details","requestedSchema":{"type":"object","properties":{"count":{"type":%q,"title":"Count"},"note":{"type":"string","title":"Note"}}}}}`, tc.typeName))
			if err := h.answer("request:1/0", "", tc.answer); err == nil {
				t.Fatal("invalid numeric answer accepted")
			}
			if h.item(t, "request:1/0").Done {
				t.Fatal("invalid answer closed its field")
			}
			if err := h.answer("request:1/0", "", "3"); err != nil {
				t.Fatal(err)
			}
			if err := h.answer("request:1/1", "", "hello"); err != nil {
				t.Fatal(err)
			}
			got := h.server.await(1)
			if len(got) != 1 || compact(got[0]["result"]) != `{"action":"accept","content":{"count":3,"note":"hello"}}` {
				t.Fatalf("form response = %+v", got)
			}
		})
	}
}
