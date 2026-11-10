package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

// utilitySystemPrompt replaces Pi's own system prompt for a utility prompt.
const utilitySystemPrompt = "Reply with only what the request asks for."

// utilityFlags keep tools, extensions, MCP servers, skills, prompt templates,
// project instructions, themes, saved conversations, and network checks out of
// a utility prompt.
var utilityFlags = []string{"--no-session", "--no-tools", "--no-extensions", "--no-mcp",
	"--no-skills", "--no-prompt-templates", "--no-context-files", "--no-themes", "--no-approve", "--offline"}

// generate sends one prompt to pi in an empty temporary directory and returns
// the answer. A first pi process, started with the same flags as the prompt,
// checks the selection against the models and thinking levels pi offers before
// the prompt is sent.
func generate(ctx context.Context, req harness.GenerateRequest) (string, error) {
	provider, model, ok := strings.Cut(req.Model, "/")
	if !ok {
		return "", fmt.Errorf("model %q is not provider/id; see list_models", req.Model)
	}
	dir, err := os.MkdirTemp("", "hypercube-utility-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	if err := checkUtility(ctx, dir, req, provider, model); err != nil {
		return "", err
	}

	argv := append([]string{"-p", "--mode", "json"}, utilityFlags...)
	argv = append(argv, "--system-prompt", utilitySystemPrompt, "--append-system-prompt", "", "--provider", provider, "--model", model)
	if req.Effort != "" {
		argv = append(argv, "--thinking", req.Effort)
	}
	cmd := exec.Command("pi", argv...)
	cmd.Dir = dir
	p, err := plugin.StartProcess("Pi", cmd)
	if err != nil {
		return "", err
	}

	// outcome is what the reading goroutine learned. That goroutine owns it
	// until it sends it with Read's result.
	type outcome struct {
		found   bool   // an assistant message ended
		stop    string // that message's stop reason
		failure string // that message's error message
		text    string // that message's text blocks
		plain   string // the last line that is not JSON
		readErr error
	}
	read := make(chan outcome, 1)
	go func() {
		var o outcome
		o.readErr = p.Read(func(line []byte) {
			var e struct {
				Type    string `json:"type"`
				Message struct {
					Role string `json:"role"`
				} `json:"message"`
			}
			if err := json.Unmarshal(line, &e); err != nil {
				o.plain = strings.TrimSpace(string(line))
				return
			}
			if e.Type != "message_end" || e.Message.Role != "assistant" {
				return
			}
			// User and system messages can have string content, so the blocks
			// are decoded only for an assistant message.
			var m struct {
				Message struct {
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
					StopReason   string `json:"stopReason"`
					ErrorMessage string `json:"errorMessage"`
				} `json:"message"`
			}
			if err := json.Unmarshal(line, &m); err != nil {
				return
			}
			// A retried request ends an errored message before the retry, so
			// the latest assistant message is the answer.
			var texts []string
			for _, b := range m.Message.Content {
				if b.Type == "text" {
					texts = append(texts, b.Text)
				}
			}
			o.found, o.stop, o.failure, o.text = true, m.Message.StopReason, m.Message.ErrorMessage, strings.Join(texts, "\n")
		})
		read <- o
	}()
	_, _ = p.Stdin.Write([]byte(req.Prompt))
	_ = p.Stdin.Close()

	var o outcome
	select {
	case o = <-read:
	case <-ctx.Done():
		p.Stop()
		return "", ctx.Err()
	}
	switch {
	case o.found && o.stop == "stop":
		return o.text, nil
	case o.found && o.failure != "":
		return "", errors.New(o.failure)
	case o.found:
		return "", fmt.Errorf("Pi stopped with reason %q", o.stop)
	case o.plain != "":
		return "", fmt.Errorf("%s: %w", o.plain, o.readErr)
	}
	return "", o.readErr
}

// checkUtility starts pi with the utility flags and rejects a model it does not
// list, which pi would otherwise accept as a custom model, and a thinking level
// the model lacks.
func checkUtility(ctx context.Context, dir string, req harness.GenerateRequest, provider, model string) error {
	argv := append([]string{"--mode", "rpc"}, utilityFlags...)
	argv = append(argv, "--provider", provider, "--model", model)
	ag, err := launch(dir, argv, func(harness.Event) {})
	if err != nil {
		return err
	}
	defer ag.stop()
	var available struct {
		Models []struct {
			ID       string `json:"id"`
			Provider string `json:"provider"`
		} `json:"models"`
	}
	if err := ag.call(ctx, "get_available_models", nil, &available); err != nil {
		return err
	}
	listed := false
	for _, m := range available.Models {
		listed = listed || m.Provider+"/"+m.ID == req.Model
	}
	if !listed {
		return fmt.Errorf("Pi does not offer model %s with extensions off, as utility prompts run it; choose a model that no Pi extension provides", req.Model)
	}
	var state sessionState
	if err := ag.call(ctx, "get_state", nil, &state); err != nil {
		return err
	}
	if err := state.check(req.Model); err != nil {
		return err
	}
	return ag.checkLevel(ctx, req.Model, req.Effort)
}
