package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
)

const assistantStop = `{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"Fix the parser\n\nMore"}],"stopReason":"stop"}}`

// printed writes the lines pi prints in a prompt run and returns the
// environment that plays them.
func printed(t *testing.T, lines ...string) map[string]string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "print.jsonl")
	if err := os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return map[string]string{"FAKE_PRINT": file}
}

func run(t *testing.T, req harness.GenerateRequest) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return generate(ctx, req)
}

func TestGenerateRunsThePromptWithTheUtilityFlags(t *testing.T) {
	env := printed(t,
		`{"type":"message_end","message":{"role":"system","content":"Be brief"}}`,
		assistantStop)
	args := fakePi(t, env)
	text, err := run(t, harness.GenerateRequest{Model: "openrouter/x/m", Effort: "high", Prompt: "Name this"})
	if err != nil {
		t.Fatal(err)
	}
	if text != "Fix the parser\n\nMore" {
		t.Fatalf("text: %q", text)
	}
	check := read(t, args)
	if !strings.Contains(check, "--mode rpc") || !strings.Contains(check, "--provider openrouter --model x/m") {
		t.Fatalf("check run: %q", check)
	}
	prompt := read(t, args+".print")
	for _, want := range []string{"-p --mode json", "--system-prompt " + utilitySystemPrompt, "--append-system-prompt  --provider", "--thinking high"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt run lacks %q: %q", want, prompt)
		}
	}
	for _, flag := range utilityFlags {
		if !strings.Contains(check, flag) || !strings.Contains(prompt, flag) {
			t.Errorf("flag %s is missing: %q, %q", flag, check, prompt)
		}
	}
	if got := read(t, args+".stdin"); got != "Name this" {
		t.Fatalf("stdin: %q", got)
	}
	dir := strings.TrimSpace(read(t, args+".dir"))
	if !strings.Contains(dir, "hypercube-utility-") {
		t.Fatalf("directory: %q", dir)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("temporary directory remains: %v", err)
	}
}

func read(t *testing.T, file string) string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestGenerateResults(t *testing.T) {
	tests := []struct {
		name    string
		req     harness.GenerateRequest
		env     map[string]string
		text    string // the expected answer when want is empty
		want    string // text the error contains
		printed bool   // the prompt run started
	}{
		{"unlisted model", harness.GenerateRequest{Model: "openrouter/unknown", Effort: "high"}, map[string]string{"FAKE_MODEL": "unknown"}, "", "with extensions off", false},
		{"level", harness.GenerateRequest{Model: "openrouter/x/m", Effort: "minimal"}, nil, "", "does not accept thinking level", false},
		{"no provider", harness.GenerateRequest{Model: "noslash"}, nil, "", "is not provider/id", false},
		{"retried", harness.GenerateRequest{Model: "openrouter/x/m", Effort: "high"}, printed(t,
			`{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"overloaded"}}`,
			`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"Second try"}],"stopReason":"stop"}}`), "Second try", "", true},
		{"error", harness.GenerateRequest{Model: "openrouter/x/m", Effort: "high"}, printed(t,
			`{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"rate limited"}}`), "", "rate limited", true},
		{"length", harness.GenerateRequest{Model: "openrouter/x/m", Effort: "high"}, printed(t,
			`{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"length"}}`), "", `"length"`, true},
		{"exit", harness.GenerateRequest{Model: "openrouter/x/m", Effort: "high"}, map[string]string{"FAKE_PRINT_FAIL": "No API key found for openrouter"}, "", "No API key found for openrouter", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := fakePi(t, tt.env)
			tt.req.Prompt = "Name this"
			text, err := run(t, tt.req)
			if tt.want == "" {
				if err != nil || text != tt.text {
					t.Fatalf("text %q, error %v", text, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error: %v", err)
			}
			_, statErr := os.Stat(args + ".print")
			if ran := statErr == nil; ran != tt.printed {
				t.Fatalf("prompt run started: %v", ran)
			}
		})
	}
}
