package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
)

// Run with HYPERCUBE_TEST_OPENCODE=1. The actual installed server runs a real
// blocking shell tool; a local deterministic provider replaces all model calls.
func TestNativeSteering(t *testing.T) {
	if os.Getenv("HYPERCUBE_TEST_OPENCODE") != "1" {
		t.Skip("set HYPERCUBE_TEST_OPENCODE=1 to exercise installed OpenCode")
	}
	gate, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	running := make(chan net.Conn, 1)
	go func() {
		for {
			c, err := gate.Accept()
			if err != nil {
				return
			}
			var b [1]byte
			if _, err := io.ReadFull(c, b[:]); err != nil {
				c.Close()
				return
			}
			running <- c
		}
	}()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENCODE_STEER_GATE", gate.Addr().String())
	t.Setenv("OPENCODE_MCP_HELPER", "1")
	command := "'" + strings.ReplaceAll(executable, "'", "'\\''") + "' -test.run=^TestNativeToolHelper$"
	var mu sync.Mutex
	var continuation string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Stream   bool `json:"stream"`
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		hasResult, hasShell := false, false
		for _, m := range body.Messages {
			if m.Role == "user" && strings.Contains(string(m.Content), "BLOCKING_PROBE") {
				hasResult = false
			}
			if m.Role == "tool" {
				hasResult = true
			}
		}
		for _, tool := range body.Tools {
			if tool.Function.Name == "shell" {
				hasShell = true
			}
		}
		delta := map[string]any{"content": "Probe title"}
		finish := "stop"
		if hasShell && !hasResult {
			args, _ := json.Marshal(map[string]string{"command": command})
			delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "probe_tool", "type": "function", "function": map[string]string{"name": "shell", "arguments": string(args)}}}}
			finish = "tool_calls"
		} else if hasResult {
			messages, _ := json.Marshal(body.Messages)
			mu.Lock()
			continuation = string(messages)
			mu.Unlock()
			delta = map[string]any{"content": "CORRECTED"}
		}
		if !body.Stream {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "probe", "object": "chat.completion", "created": time.Now().Unix(), "model": "probe", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "Probe title"}, "finish_reason": "stop"}}})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range []struct {
			delta  map[string]any
			finish any
		}{{delta, nil}, {map[string]any{}, finish}} {
			data, _ := json.Marshal(map[string]any{"id": "probe", "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": "probe", "choices": []any{map[string]any{"index": 0, "delta": chunk.delta, "finish_reason": chunk.finish}}})
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer provider.Close()
	dir := t.TempDir()
	t.Setenv("OPENCODE_DB", filepath.Join(dir, "opencode.db"))
	config := map[string]any{"providers": map[string]any{"steer-probe": map[string]any{
		"package": "@opencode/ai/providers/openai-compatible", "settings": map[string]string{"apiKey": "probe", "baseURL": provider.URL + "/v1"},
		"models": map[string]any{"probe": map[string]any{"name": "Probe", "capabilities": map[string]bool{"tools": true}, "limit": map[string]int{"context": 100000, "output": 2000}}},
	}}}
	data, _ := json.Marshal(config)
	file := filepath.Join(dir, "probe.json")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENCODE_CONFIG", file)
	mcp := filepath.Join(dir, "mcp")
	if err := os.WriteFile(mcp, []byte("#!/bin/sh\nexec '"+strings.ReplaceAll(executable, "'", "'\\''")+"' -test.run=^TestNativeMCPHelper$ -- \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	h := &testHarness{changed: make(chan struct{}, 1)}
	ag, err := startAgent(harness.StartArgs{Hypercube: mcp, Session: "probe", Dir: dir, StateDir: dir, Model: "steer-probe/probe"}, func(e harness.Event) {
		h.mu.Lock()
		h.events = append(h.events, e)
		h.mu.Unlock()
		select {
		case h.changed <- struct{}{}:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	h.agent = ag
	defer ag.stop()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := ag.server.call(ctx, "DELETE", sessionPath(ag.session), nil, nil); err != nil {
			t.Error(err)
		}
	}()
	if err := ag.input("BLOCKING_PROBE: run the blocking tool, then respond.", nil); err != nil {
		t.Fatal(err)
	}
	var tool net.Conn
	select {
	case tool = <-running:
	case <-time.After(20 * time.Second):
		t.Fatalf("native shell did not start; events: %+v", h.snapshot())
	}
	defer tool.Close()
	for _, text := range []string{"CORRECTION_ONE: change the response.", "CORRECTION_TWO: keep the tool result."} {
		if err := ag.input(text, nil); err != nil {
			t.Fatal(err)
		}
	}
	if len(h.states()) != 1 {
		t.Fatalf("steering ended execution: %v", h.states())
	}
	tool.Close() // The tool, not cancellation, now completes normally.
	waitFor(t, h, func() bool { return len(h.states()) >= 2 })
	mu.Lock()
	got := continuation
	mu.Unlock()
	for _, text := range []string{"CORRECTION_ONE", "CORRECTION_TWO", "TOOL_FINISHED"} {
		if !strings.Contains(got, text) {
			t.Fatalf("next model request lacks %s: %s", text, got)
		}
	}
	var completed, answered bool
	for _, e := range h.snapshot() {
		for _, it := range e.Items {
			if it.Kind == harness.KindTool && it.Done {
				if it.Failed {
					t.Fatalf("tool cancelled or failed: %+v", it)
				}
				completed = strings.Contains(it.Output, "TOOL_FINISHED")
			}
			if it.Kind == harness.KindText && it.Done && it.Text == "CORRECTED" {
				answered = true
			}
		}
	}
	if !completed || !answered {
		t.Fatalf("tool completion %v, corrected answer %v", completed, answered)
	}
	// Stop remains a different operation: it must cancel the running tool.
	if err := ag.input("BLOCKING_PROBE: this time test explicit Stop.", nil); err != nil {
		t.Fatal(err)
	}
	select {
	case tool = <-running:
	case <-time.After(20 * time.Second):
		t.Fatal("second native tool did not start")
	}
	defer tool.Close()
	if err := ag.interrupt(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, h, func() bool { return len(h.states()) == 4 })
	events := h.snapshot()
	last := events[len(events)-1]
	if last.State != harness.Idle || len(last.Final) > 0 {
		t.Fatalf("interrupted completion: %+v", last)
	}
	var cancelled bool
	for _, e := range events {
		for _, it := range e.Items {
			if it.Kind == harness.KindTool && it.Done && it.Failed {
				cancelled = true
			}
		}
	}
	if !cancelled {
		t.Fatal("Stop did not cancel the native tool")
	}
}

func TestNativeToolHelper(t *testing.T) {
	address := os.Getenv("OPENCODE_STEER_GATE")
	if address == "" {
		return
	}
	c, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, c); err != nil {
		t.Fatal(err)
	}
	fmt.Println("TOOL_FINISHED")
}

func TestNativeMCPHelper(t *testing.T) {
	if os.Getenv("OPENCODE_MCP_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		if len(m.ID) == 0 {
			continue
		}
		var result any = map[string]any{}
		switch m.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "probe", "version": "1"}}
		case "tools/list":
			result = map[string]any{"tools": []any{}}
		}
		if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": result}); err != nil {
			t.Fatal(err)
		}
	}
}
