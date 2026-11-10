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

// utilitySystemPrompt replaces Claude Code's own system prompt for a utility
// prompt.
const utilitySystemPrompt = "Reply with only what the request asks for."

// generate sends one prompt to claude in an empty temporary directory, with
// no tools, MCP servers, slash commands, project instructions, or saved
// conversation, and returns the answer. The selection is checked against the
// models claude offers before the prompt is sent.
func generate(ctx context.Context, req harness.GenerateRequest) (string, error) {
	dir, err := os.MkdirTemp("", "hypercube-utility-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	argv := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose",
		"--safe-mode", "--no-session-persistence", "--tools", "", "--disallowedTools", "mcp__*",
		"--strict-mcp-config", "--disable-slash-commands", "--system-prompt", utilitySystemPrompt, "--model", req.Model}
	if req.Effort != "" {
		argv = append(argv, "--effort", req.Effort)
	}
	cmd := exec.Command("claude", argv...)
	cmd.Dir = dir
	p, err := plugin.StartProcess("Claude Code", cmd)
	if err != nil {
		return "", err
	}

	// outcome is what the reading goroutine learned. That goroutine owns it
	// until it sends it with Read's result.
	type outcome struct {
		failure error
		text    string
		got     bool   // a result line arrived
		plain   string // the last line that is not JSON
		readErr error
	}
	read := make(chan outcome, 1)
	go func() {
		var o outcome
		fail := func(err error) {
			if o.failure == nil {
				o.failure = err
			}
			_ = p.Stdin.Close()
		}
		o.readErr = p.Read(func(line []byte) {
			var m message
			if err := json.Unmarshal(line, &m); err != nil {
				// Claude can report a failure such as "Not logged in" as plain
				// text on stdout.
				o.plain = strings.TrimSpace(string(line))
				return
			}
			switch {
			case m.Type == "control_response" && m.Response.RequestID == "initialize":
				if m.Response.Subtype == "error" {
					fail(errors.New(m.Response.Error))
					return
				}
				models, err := initialized(m.Response.Response)
				if err == nil {
					_, err = check(models, harness.Selection{Model: req.Model, Effort: req.Effort})
				}
				if err != nil {
					fail(err)
					return
				}
				prompt := map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": req.Prompt}}
				b, err := json.Marshal(prompt)
				if err == nil {
					_, err = p.Stdin.Write(append(b, '\n'))
				}
				if err != nil {
					fail(err)
				}
			case m.Type == "result":
				if m.IsError || m.Subtype != "success" {
					fail(errors.New(m.summary()))
					return
				}
				o.text, o.got = m.Result, true
				_ = p.Stdin.Close()
			}
		})
		read <- o
	}()
	b, _ := json.Marshal(map[string]any{"type": "control_request", "request_id": "initialize", "request": map[string]any{"subtype": "initialize"}})
	_, _ = p.Stdin.Write(append(b, '\n'))

	var o outcome
	select {
	case o = <-read:
	case <-ctx.Done():
		p.Stop()
		return "", ctx.Err()
	}
	p.Stop()
	switch {
	case o.failure != nil:
		return "", o.failure
	case o.got:
		return o.text, nil
	case o.plain != "":
		return "", fmt.Errorf("%s: %w", o.plain, o.readErr)
	}
	return "", o.readErr
}
