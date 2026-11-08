package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
)

type nativeRequest struct {
	turn  string
	input string // user messages and tool results, without tool definitions
}

// nativeProvider records what reaches the model and serves the scenario's
// Responses stream. Codex handles execution, steering, and cancellation.
func nativeProvider(t *testing.T, respond func(http.ResponseWriter, *http.Request, nativeRequest, int)) (*httptest.Server, func() []nativeRequest) {
	t.Helper()
	var mu sync.Mutex
	var requests []nativeRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			t.Errorf("unexpected provider request: %s %s", r.Method, r.URL)
			http.Error(w, "unexpected endpoint", http.StatusBadRequest)
			return
		}
		var body struct {
			Input []struct {
				Type    string          `json:"type"`
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
				Output  json.RawMessage `json:"output"`
			} `json:"input"`
			Metadata struct {
				Turn string `json:"turn_id"`
			} `json:"client_metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var input strings.Builder
		for _, it := range body.Input {
			if it.Type == "message" && it.Role == "user" {
				input.Write(it.Content)
			}
			if it.Type == "custom_tool_call_output" || it.Type == "function_call_output" {
				input.Write(it.Output)
			}
		}
		trace := nativeRequest{turn: body.Metadata.Turn, input: input.String()}
		mu.Lock()
		requests = append(requests, trace)
		n := len(requests)
		mu.Unlock()
		respond(w, r, trace, n)
	}))
	t.Cleanup(server.Close)
	return server, func() []nativeRequest {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(requests)
	}
}

// nativeToolProvider starts a shell, waits for its result if steering yields
// the exec cell, and answers. The tool result must reach the model unchanged.
func nativeToolProvider(t *testing.T, command string) (*httptest.Server, func() []nativeRequest) {
	t.Helper()
	cellID := regexp.MustCompile(`Script running with cell ID ([a-zA-Z0-9_-]+)`)
	return nativeProvider(t, func(w http.ResponseWriter, r *http.Request, trace nativeRequest, n int) {
		var item map[string]any
		if n == 1 {
			args, _ := json.Marshal(map[string]any{"cmd": command, "yield_time_ms": 30000})
			item = map[string]any{"id": "ctc_probe", "type": "custom_tool_call", "call_id": "probe_shell", "namespace": "functions", "name": "exec", "input": "text(await tools.exec_command(" + string(args) + "));", "status": "completed"}
		} else if !strings.Contains(trace.input, "TOOL_FINISHED") {
			// Steering can yield a still-running code-mode cell. A real model
			// waits for that cell rather than discarding the pending tool result.
			cells := cellID.FindAllStringSubmatch(trace.input, -1)
			if len(cells) == 0 {
				t.Errorf("no tool result or running cell in model continuation: %s", trace.input)
				http.Error(w, "missing tool result", http.StatusBadRequest)
				return
			}
			args, _ := json.Marshal(map[string]any{"cell_id": cells[len(cells)-1][1], "yield_time_ms": 30000})
			item = map[string]any{"id": fmt.Sprintf("fc_wait_%d", n), "type": "function_call", "call_id": fmt.Sprintf("wait_%d", n), "namespace": "functions", "name": "wait", "arguments": string(args), "status": "completed"}
		} else {
			text := "CORRECTED"
			if strings.Contains(trace.input, "FOLLOW_UP:") {
				text = "FOLLOW_UP_DONE"
			}
			item = nativeMessage(fmt.Sprintf("msg_%d", n), "final_answer", text)
		}
		writeNativeResponse(t, w, fmt.Sprintf("resp_%d", n), item)
	})
}

func nativeMessage(id, phase, text string) map[string]any {
	return map[string]any{"id": id, "type": "message", "role": "assistant", "phase": phase, "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": text, "annotations": []any{}}}}
}

func writeNativeResponse(t *testing.T, w http.ResponseWriter, id string, item map[string]any) {
	t.Helper()
	stream := nativeStream{t: t, w: w}
	stream.write(
		map[string]any{"type": "response.created", "response": map[string]any{"id": id, "status": "in_progress", "output": []any{}}},
		map[string]any{"type": "response.output_item.added", "output_index": 0, "item": item},
		map[string]any{"type": "response.output_item.done", "output_index": 0, "item": item},
		nativeCompleted(id, item),
	)
}

func nativeCompleted(id string, item map[string]any) map[string]any {
	return map[string]any{"type": "response.completed", "response": map[string]any{"id": id, "status": "completed", "output": []any{item}, "usage": map[string]any{"input_tokens": 100, "output_tokens": 10, "total_tokens": 110}}}
}

type nativeStream struct {
	t        *testing.T
	w        http.ResponseWriter
	sequence int
}

func (s *nativeStream) write(events ...map[string]any) {
	s.t.Helper()
	s.w.Header().Set("Content-Type", "text/event-stream")
	for _, event := range events {
		event["sequence_number"] = s.sequence
		s.sequence++
		data, err := json.Marshal(event)
		if err != nil {
			s.t.Error(err)
			return
		}
		// Cancellation can close the original response while it is released.
		if _, err := io.WriteString(s.w, fmt.Sprintf("event: %s\ndata: %s\n\n", event["type"], data)); err != nil {
			return
		}
	}
}
