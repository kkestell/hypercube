package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/harness/harnesstest"
)

// Run with HYPERCUBE_TEST_CODEX=1 go test -race ./plugins/codex -run '^TestNative'.
// These tests use the installed app-server, an isolated CODEX_HOME, and a local
// Responses provider. No credentials or paid model calls are needed.
func TestNativeSteering(t *testing.T) {
	p := startNativeProbe(t)
	p.command(harness.Command{Command: harness.CmdInput, Text: "Run the blocking tool, then answer."})
	tool := p.runningTool()
	for _, text := range []string{"CORRECTION_ONE: change the answer.", "CORRECTION_TWO: retain the tool result."} {
		p.command(harness.Command{Command: harness.CmdInput, Text: text})
	}
	if err := tool.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(tool, "ping"); err != nil {
		t.Fatal(err)
	}
	if line, err := bufio.NewReader(tool).ReadString('\n'); err != nil || line != "alive\n" {
		t.Fatalf("steering stopped the tool: %q, %v", line, err)
	}
	if got := p.states(); !slices.Equal(got, []string{harness.Idle, harness.Working}) {
		t.Fatalf("steering ended or restarted the turn: %v", got)
	}
	// Only the test releases the tool. Steering must leave it running.
	if _, err := fmt.Fprintln(tool, "finish"); err != nil {
		t.Fatal(err)
	}
	p.waitStates(harness.Idle, harness.Working, harness.Idle)
	requests := p.providerRequests()
	input := requests[len(requests)-1].input
	for _, marker := range []string{"CORRECTION_ONE", "CORRECTION_TWO", "TOOL_FINISHED"} {
		if !strings.Contains(input, marker) {
			t.Errorf("continuation lacks %s: %s", marker, input)
		}
	}
	for _, request := range requests {
		if request.turn == "" || request.turn != requests[0].turn {
			t.Errorf("steering replaced native turn %q with %q", requests[0].turn, request.turn)
		}
	}
	p.requireFinal("CORRECTED")
	// Input after completion must still start a new turn.
	p.command(harness.Command{Command: harness.CmdInput, Text: "FOLLOW_UP: answer again."})
	p.waitStates(harness.Idle, harness.Working, harness.Idle, harness.Working, harness.Idle)
	p.requireFinal("FOLLOW_UP_DONE")
	followup := p.providerRequests()
	if last := followup[len(followup)-1]; last.turn == requests[0].turn || !strings.Contains(last.input, "FOLLOW_UP:") {
		t.Fatalf("idle input did not start a new native turn: %+v", last)
	}
}

func TestNativeStop(t *testing.T) {
	p := startNativeProbe(t)
	p.command(harness.Command{Command: harness.CmdInput, Text: "Run the blocking tool, then answer."})
	tool := p.runningTool()
	p.command(harness.Command{Command: harness.CmdInterrupt})
	p.waitStates(harness.Idle, harness.Working, harness.Idle)
	// The shell's connection closes when Stop kills it, without releasing it.
	if err := tool.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var b [1]byte
	if n, err := tool.Read(b[:]); n != 0 || err != io.EOF {
		t.Fatalf("Stop left the tool running: read %d, %v", n, err)
	}
	for _, ev := range p.events {
		if len(ev.Final) > 0 {
			t.Fatalf("Stop reported a final answer: %+v", ev)
		}
	}
}

type nativeProbe struct {
	t                *testing.T
	client           *harnesstest.Serving
	id               int64
	events           []harness.Event
	gate             *net.TCPListener
	providerRequests func() []nativeRequest
}

func startNativeProbe(t *testing.T) *nativeProbe {
	t.Helper()
	requireNativeCodex(t)
	gate, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { gate.Close() })
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := "CODEX_NATIVE_TOOL=1 " + nativeQuote(executable) + " -test.run=^TestNativeToolHelper$ -- " + gate.Addr().String()
	provider, requests := nativeToolProvider(t, command)
	p := startNativeClient(t, provider.URL)
	p.gate, p.providerRequests = gate, requests
	return p
}

func requireNativeCodex(t *testing.T) {
	t.Helper()
	if os.Getenv("HYPERCUBE_TEST_CODEX") != "1" {
		t.Skip("set HYPERCUBE_TEST_CODEX=1 to exercise installed Codex")
	}
	if _, err := exec.LookPath("codex"); err != nil {
		t.Fatal(err)
	}
}

func nativeQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

func startNativeClient(t *testing.T, providerURL string) *nativeProbe {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	home, dir := t.TempDir(), t.TempDir()
	t.Setenv("CODEX_HOME", home)
	config := fmt.Sprintf(`model = "gpt-6-luna"
model_provider = "steering-probe"
web_search = "disabled"
[features]
enable_request_compression = false
plugins = false
[model_providers.steering-probe]
name = "Steering probe"
base_url = %q
wire_api = "responses"
requires_openai_auth = false
request_max_retries = 0
stream_max_retries = 0
`, providerURL+"/v1")
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	mcp := filepath.Join(dir, "mcp")
	if err := os.WriteFile(mcp, []byte("#!/bin/sh\nCODEX_NATIVE_MCP=1 exec "+nativeQuote(executable)+" -test.run=^TestNativeMCPHelper$\n"), 0700); err != nil {
		t.Fatal(err)
	}
	p := &nativeProbe{t: t, client: harnesstest.Serve(t, serve)}
	p.command(harness.Command{Command: harness.CmdStart, Start: &harness.StartArgs{MCP: mcp, Env: map[string]string{}, Dir: dir, StateDir: dir, Model: "gpt-6-luna", Effort: "low"}})
	p.waitStates(harness.Idle)
	return p
}

func (p *nativeProbe) until(match func(harness.Record) bool) harness.Record {
	p.t.Helper()
	return p.client.Until(func(r harness.Record) bool {
		if r.Event != nil {
			p.events = append(p.events, *r.Event)
			for _, it := range r.Event.Items {
				if it.Kind == harness.KindError {
					p.t.Fatalf("native Codex error: %s", it.Text)
				}
			}
		}
		return match(r)
	})
}

func (p *nativeProbe) command(c harness.Command) {
	p.t.Helper()
	p.id++
	c.ID = p.id
	p.client.Send(c)
	r := p.until(func(r harness.Record) bool { return r.Reply != nil && r.Reply.ID == c.ID }).Reply
	if r.Error != "" {
		p.t.Fatalf("%s: %s", c.Command, r.Error)
	}
}

func (p *nativeProbe) runningTool() net.Conn {
	p.t.Helper()
	if err := p.gate.SetDeadline(time.Now().Add(20 * time.Second)); err != nil {
		p.t.Fatal(err)
	}
	c, err := p.gate.Accept()
	if err != nil {
		p.t.Fatalf("native shell did not connect: %v; model requests: %+v", err, p.providerRequests())
	}
	p.t.Cleanup(func() { c.Close() })
	return c
}

func (p *nativeProbe) states() []string {
	var states []string
	for _, ev := range p.events {
		if ev.State != "" {
			states = append(states, ev.State)
		}
	}
	return states
}

// Command replies can follow notifications, so check collected events before
// waiting for more. Conditions must identify the phase they are waiting for.
func (p *nativeProbe) waitUntil(match func() bool) {
	p.t.Helper()
	if !match() {
		p.until(func(harness.Record) bool { return match() })
	}
}

func (p *nativeProbe) waitStates(want ...string) {
	p.t.Helper()
	p.waitUntil(func() bool { return len(p.states()) >= len(want) })
	if got := p.states(); !slices.Equal(got, want) {
		p.t.Fatalf("native turn lifecycle: got %v, want %v", got, want)
	}
}

func (p *nativeProbe) requireFinal(text string) {
	p.t.Helper()
	var finalIDs []string
	for _, ev := range p.events {
		finalIDs = append(finalIDs, ev.Final...)
	}
	for _, ev := range p.events {
		for _, it := range ev.Items {
			if it.Kind == harness.KindText && it.Done && it.Text == text && slices.Contains(finalIDs, it.ID) {
				return
			}
		}
	}
	p.t.Fatalf("no final answer %q; events: %+v", text, p.events)
}

func TestNativeToolHelper(t *testing.T) {
	if os.Getenv("CODEX_NATIVE_TOOL") != "1" {
		return
	}
	c, err := net.Dial("tcp", os.Args[len(os.Args)-1])
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	scanner := bufio.NewScanner(c)
	for scanner.Scan() {
		switch scanner.Text() {
		case "ping":
			if _, err := fmt.Fprintln(c, "alive"); err != nil {
				t.Fatal(err)
			}
		case "finish":
			fmt.Println("TOOL_FINISHED")
			return
		default:
			t.Fatalf("unexpected tool command: %s", scanner.Text())
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatal("tool connection closed before finish")
}

func TestNativeMCPHelper(t *testing.T) {
	if os.Getenv("CODEX_NATIVE_MCP") != "1" {
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
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}
