package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
)

const resultLine = `{"type":"result","subtype":"success","is_error":false,"result":"Fix the flaky parser test"}`

func generateRequest() harness.GenerateRequest {
	return harness.GenerateRequest{Model: "haiku", Effort: "low", Prompt: "Title: fix the parser"}
}

func TestGenerate(t *testing.T) {
	args := fakeClaude(t, map[string]string{"FAKE_STREAM": playFile(t, resultLine)})
	got, err := generate(context.Background(), generateRequest())
	if err != nil || got != "Fix the flaky parser test" {
		t.Fatalf("generate = %q, %v", got, err)
	}
	data, _ := os.ReadFile(args)
	for _, want := range []string{"--safe-mode", "--no-session-persistence", "--tools  ", "--disallowedTools mcp__*", "--strict-mcp-config",
		"--disable-slash-commands", "--system-prompt " + utilitySystemPrompt, "--model haiku", "--effort low"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("arguments %q lack %q", data, want)
		}
	}
	stdin, _ := os.ReadFile(args + ".stdin")
	if !strings.Contains(string(stdin), `"content":"Title: fix the parser"`) {
		t.Errorf("stdin %q lacks the prompt", stdin)
	}
	dir, _ := os.ReadFile(args + ".dir")
	name := strings.TrimSpace(string(dir))
	if !strings.Contains(name, "hypercube-utility-") {
		t.Errorf("ran in %q", name)
	}
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Errorf("the temporary directory remains: %v", err)
	}
}

func TestGenerateFailures(t *testing.T) {
	t.Run("error result", func(t *testing.T) {
		fakeClaude(t, map[string]string{"FAKE_STREAM": playFile(t, `{"type":"result","subtype":"success","is_error":true,"result":"credit balance too low"}`)})
		if _, err := generate(context.Background(), generateRequest()); err == nil || !strings.Contains(err.Error(), "credit balance too low") {
			t.Fatalf("generate = %v", err)
		}
	})
	t.Run("unsupported effort", func(t *testing.T) {
		args := fakeClaude(t, map[string]string{"FAKE_STREAM": playFile(t, resultLine)})
		req := generateRequest()
		req.Effort = "high"
		if _, err := generate(context.Background(), req); err == nil || !strings.Contains(err.Error(), "does not accept effort") {
			t.Fatalf("generate = %v", err)
		}
		stdin, _ := os.ReadFile(args + ".stdin")
		if strings.Contains(string(stdin), `"type":"user"`) {
			t.Errorf("the prompt was sent: %s", stdin)
		}
	})
	t.Run("exit", func(t *testing.T) {
		fakeClaude(t, map[string]string{"FAKE_FAIL": "1"})
		if _, err := generate(context.Background(), generateRequest()); err == nil || !strings.Contains(err.Error(), "resume failed") {
			t.Fatalf("generate = %v", err)
		}
	})
	t.Run("plain line", func(t *testing.T) {
		fakeClaude(t, map[string]string{"FAKE_BEFORE": playFile(t, "Not logged in · Please run /login"), "FAKE_FAIL": "1"})
		if _, err := generate(context.Background(), generateRequest()); err == nil || !strings.Contains(err.Error(), "Not logged in") {
			t.Fatalf("generate = %v", err)
		}
	})
}
