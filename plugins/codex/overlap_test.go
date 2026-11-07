package main

import (
	"context"
	"encoding/json"
	"testing"
	"testing/synctest"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

type writeFunc func([]byte) (int, error)

func (f writeFunc) Write(p []byte) (int, error) { return f(p) }

func TestInterruptRetainedAcrossOverlappingInputAndAsyncAnswer(t *testing.T) {
	for _, when := range []string{"before answer completes", "after answer completes"} {
		t.Run(when, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				h := &agentUnderTest{agent: &agent{
					ctx: ctx, thread: "t1", finished: map[string]bool{},
					open: map[string]harness.Item{}, running: map[string]harness.Item{},
				}}
				h.emit = func(harness.Event) {}
				calls := make(chan map[string]any, 10)
				h.rpc = plugin.NewRPC(writeFunc(func(p []byte) (int, error) {
					var m map[string]any
					if err := json.Unmarshal(p, &m); err != nil {
						return 0, err
					}
					calls <- m
					// Interrupts finish immediately; steer/start acceptance is controlled below.
					if m["method"] == "turn/interrupt" {
						h.rpc.Line([]byte(compact(map[string]any{"id": m["id"], "result": map[string]any{}})))
					}
					return len(p), nil
				}), h.handle)
				next := func(method string) map[string]any {
					t.Helper()
					synctest.Wait()
					select {
					case m := <-calls:
						if m["method"] != method {
							t.Fatalf("wanted %s, got %v", method, m)
						}
						return m
					default:
						t.Fatalf("no native %s call", method)
						return nil
					}
				}
				h.feed(t,
					`{"method":"turn/started","params":{"threadId":"t1","turn":{"id":"old"}}}`,
					`{"method":"item/completed","params":{"threadId":"t1","item":{"type":"agentMessage","id":"q","delivery":"async","questions":[{"title":"Name?","options":null}]}}}`)
				inputDone := make(chan error, 1)
				go func() { inputDone <- h.input("pending", nil) }()
				first := next("turn/steer")
				answerDone := make(chan error, 1)
				go func() { answerDone <- h.answer("question:q/0", nil, "Ada") }()
				answer := next("turn/steer")
				if when == "before answer completes" {
					if err := h.interrupt(); err != nil {
						t.Fatal(err)
					}
				}
				h.rpc.Line([]byte(compact(map[string]any{"id": answer["id"], "result": map[string]any{}})))
				synctest.Wait()
				if err := <-answerDone; err != nil {
					t.Fatal(err)
				}
				if when == "after answer completes" {
					if err := h.interrupt(); err != nil {
						t.Fatal(err)
					}
				}
				h.feed(t, `{"method":"turn/completed","params":{"threadId":"t1","turn":{"id":"old","status":"completed"}}}`)
				h.rpc.Line([]byte(compact(map[string]any{"id": first["id"], "error": plugin.RPCError{Code: -32600, Message: "no active turn"}})))
				start := next("turn/start")
				h.rpc.Line([]byte(compact(map[string]any{"id": start["id"], "result": map[string]any{"turn": map[string]any{"id": "new"}}})))
				interrupt := next("turn/interrupt")
				params := interrupt["params"].(map[string]any)
				if params["threadId"] != "t1" || params["turnId"] != "new" {
					t.Fatalf("interrupt missed the fallback turn: %v", params)
				}
				if err := <-inputDone; err != nil {
					t.Fatal(err)
				}
				select {
				case extra := <-calls:
					t.Fatalf("extra native call: %v", extra)
				default:
				}
			})
		})
	}
}
