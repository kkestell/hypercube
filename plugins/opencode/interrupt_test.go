package main

import (
	"encoding/json"
	"slices"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

type writeFunc func([]byte) (int, error)

func (f writeFunc) Write(p []byte) (int, error) { return f(p) }

func TestInputWritesPromptBeforeReturningAndStopFollowsIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ag := newAgent(harness.StartArgs{Conversation: "s1"}, func(harness.Event) {})
		defer ag.cancel()
		allowWrite := make(chan struct{})
		release := sync.OnceFunc(func() { close(allowWrite) })
		defer release()
		var sent []string
		ag.rpc = plugin.NewRPC(writeFunc(func(p []byte) (int, error) {
			var m struct {
				Method string `json:"method"`
				Params struct {
					Session string `json:"sessionId"`
				} `json:"params"`
			}
			if err := json.Unmarshal(p, &m); err != nil {
				return 0, err
			}
			if m.Method == "session/prompt" {
				<-allowWrite // Hold the native write, without waiting for a prompt response.
			}
			sent = append(sent, m.Method+" "+m.Params.Session)
			return len(p), nil
		}), ag.handle)
		done := make(chan error, 1)
		go func() { done <- ag.input("go", nil) }()
		synctest.Wait()
		select {
		case err := <-done:
			t.Fatalf("Input returned before the prompt was written: %v", err)
		default:
		}
		release()
		synctest.Wait()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		// The prompt is still awaiting its response when Stop arrives.
		if err := ag.interrupt(); err != nil {
			t.Fatal(err)
		}
		if want := []string{"session/prompt s1", "session/cancel s1"}; !slices.Equal(sent, want) {
			t.Fatalf("native calls: %v", sent)
		}
	})
}
