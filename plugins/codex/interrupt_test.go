package main

import (
	"sync"
	"testing"
	"time"

	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

func TestInterruptWhileTurnStarts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		notified bool
		failed   bool
	}{
		{name: "before turn ID is known"},
		{name: "after turn started notification", notified: true},
		{name: "start fails", failed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newAgent(t)
			h.thread = "t1"
			starting, accepted := make(chan struct{}), make(chan struct{})
			release := sync.OnceFunc(func() { close(accepted) })
			defer release()
			h.server.handle("turn/start", func(map[string]any) (any, *plugin.RPCError) {
				close(starting)
				<-accepted
				if tc.failed {
					return nil, &plugin.RPCError{Code: -32600, Message: "start refused"}
				}
				return map[string]any{"turn": map[string]any{"id": "u1"}}, nil
			})
			h.server.handle("turn/interrupt", func(map[string]any) (any, *plugin.RPCError) {
				return map[string]any{}, nil
			})
			done := make(chan error, 1)
			go func() { done <- h.input("first", nil) }()
			select {
			case <-starting:
			case <-time.After(5 * time.Second):
				t.Fatal("turn/start did not arrive")
			}
			if tc.notified {
				h.feed(t, `{"method":"turn/started","params":{"threadId":"t1","turn":{"id":"u1"}}}`)
			}
			if err := h.interrupt(); err != nil {
				t.Fatal(err)
			}
			release()
			select {
			case err := <-done:
				if (err != nil) != tc.failed {
					t.Fatalf("input error: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("input did not finish")
			}
			sent := h.server.sent("turn/interrupt")
			want := 1
			if tc.failed {
				want = 0
			}
			if len(sent) != want || (want == 1 && (sent[0]["threadId"] != "t1" || sent[0]["turnId"] != "u1")) {
				t.Fatalf("turn interrupts: %+v", sent)
			}
			// The interrupt belongs to this input, including when start failed.
			if !tc.failed {
				h.feed(t, `{"method":"turn/completed","params":{"threadId":"t1","turn":{"id":"u1","status":"interrupted"}}}`)
			}
			turnStart(h, "u2")
			if err := h.input("next", nil); err != nil {
				t.Fatal(err)
			}
			if sent := h.server.sent("turn/interrupt"); len(sent) != want {
				t.Fatalf("interrupt reached the next turn: %+v", sent)
			}
		})
	}
}
