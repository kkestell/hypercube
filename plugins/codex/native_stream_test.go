package main

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
)

func TestNativeSteeringDuringResponse(t *testing.T) {
	requireNativeCodex(t)
	for _, phase := range []string{"thinking", "streaming"} {
		t.Run(phase, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			finish := sync.OnceFunc(func() { close(release) })
			// Release handlers before their server cleanup, even on failure.
			defer finish()
			provider, requests := nativeProvider(t, func(w http.ResponseWriter, r *http.Request, _ nativeRequest, n int) {
				if n != 1 {
					writeNativeResponse(t, w, "resp_corrected", nativeMessage("msg_corrected", "final_answer", "CORRECTED"))
					return
				}
				stream := nativeStream{t: t, w: w}
				stream.write(map[string]any{"type": "response.created", "response": map[string]any{"id": "resp_original", "status": "in_progress", "output": []any{}}})
				item := nativeMessage("msg_original", "commentary", "ORIGINAL_STREAM")
				if phase == "streaming" {
					stream.write(
						map[string]any{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{"id": "msg_original", "type": "message", "role": "assistant", "phase": "commentary", "status": "in_progress", "content": []any{}}},
						map[string]any{"type": "response.output_text.delta", "output_index": 0, "content_index": 0, "item_id": "msg_original", "delta": "ORIGINAL_STREAM"},
					)
				}
				if err := http.NewResponseController(w).Flush(); err != nil {
					t.Error(err)
					return
				}
				close(started)
				// Codex may cancel the HTTP response to incorporate steering.
				// Its native turn must continue rather than completing as cancelled.
				select {
				case <-r.Context().Done():
					return
				case <-release:
				}
				if phase == "thinking" {
					stream.write(map[string]any{"type": "response.output_item.added", "output_index": 0, "item": item})
				}
				stream.write(
					map[string]any{"type": "response.output_item.done", "output_index": 0, "item": item},
					nativeCompleted("resp_original", item),
				)
			})
			p := startNativeClient(t, provider.URL)
			p.command(harness.Command{Command: harness.CmdInput, Text: "Start the original reply."})
			select {
			case <-started:
			case <-time.After(10 * time.Second):
				t.Fatal("Codex did not open the model response")
			}
			if phase == "streaming" {
				p.waitUntil(func() bool {
					for _, ev := range p.events {
						for _, it := range ev.Items {
							if it.Kind == harness.KindText && strings.Contains(it.Text, "ORIGINAL_STREAM") {
								return true
							}
						}
					}
					return false
				})
			}
			p.command(harness.Command{Command: harness.CmdInput, Text: "STREAM_CORRECTION: change the reply."})
			finish()
			p.waitStates(harness.Idle, harness.Working, harness.Idle)
			trace := requests()
			if len(trace) < 2 || !strings.Contains(trace[len(trace)-1].input, "STREAM_CORRECTION") {
				t.Fatalf("correction did not reach the model: %+v", trace)
			}
			for _, request := range trace {
				if request.turn == "" || request.turn != trace[0].turn {
					t.Errorf("steering replaced native turn %q with %q", trace[0].turn, request.turn)
				}
			}
			p.requireFinal("CORRECTED")
		})
	}
}
