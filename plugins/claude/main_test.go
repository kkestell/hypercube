package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestLaunch(t *testing.T) {
	run := func(input string) []string {
		t.Helper()
		var out bytes.Buffer
		if err := launch(strings.NewReader(input), &out); err != nil {
			t.Fatal(err)
		}
		var o struct{ Argv []string }
		if err := json.Unmarshal(out.Bytes(), &o); err != nil {
			t.Fatal(err)
		}
		return o.Argv
	}
	lead := run(`{"hypercube":"/bin/hc","env":{"HYPERCUBE_AGENT_ID":"a"},"role":"lead","model":"opus","effort":"high","prompt":"go"}`)
	if lead[0] != "claude" || lead[len(lead)-1] != "go" || slices.Contains(lead, "--resume") {
		t.Fatal(lead)
	}
	settings := lead[slices.Index(lead, "--settings")+1]
	for _, want := range []string{`'hook' '/bin/hc' 'SessionStart'`, `'wait' '/bin/hc'`, `"asyncRewake":true`, `'statusline' '/bin/hc' '--limits'`} {
		if !strings.Contains(settings, want) {
			t.Errorf("settings lack %s: %s", want, settings)
		}
	}
	if mcp := lead[slices.Index(lead, "--mcp-config")+1]; !strings.Contains(mcp, `"command":"/bin/hc","env":{"HYPERCUBE_AGENT_ID":"a"}`) {
		t.Error(mcp)
	}
	worker := run(`{"hypercube":"/bin/hc","role":"worker","model":"haiku","effort":"low","session":"s1"}`)
	if i := slices.Index(worker, "--resume"); i < 0 || worker[i+1] != "s1" || strings.Contains(worker[slices.Index(worker, "--settings")+1], "--limits") {
		t.Fatal(worker)
	}
}

func TestHook(t *testing.T) {
	dir := t.TempDir()
	hypercube := filepath.Join(dir, "hypercube")
	script := "#!/bin/sh\ncat >\"" + dir + "/$2\"\necho '{\"context\":\"queued\"}'\n"
	if err := os.WriteFile(hypercube, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(event, input string) string {
		t.Helper()
		var out bytes.Buffer
		if err := hook(hypercube, event, strings.NewReader(input), &out); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	if got := run("Stop", `{}`); !strings.Contains(got, `"decision":"block","reason":"queued"`) {
		t.Error(got)
	}
	if got := run("UserPromptSubmit", `{"session_id":"s1","prompt":"fix it"}`); !strings.Contains(got, `"additionalContext":"queued"`) {
		t.Error(got)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "prompt")); string(b) != `{"session":"s1","text":"fix it"}` {
		t.Errorf("prompt report = %s", b)
	}
	for _, c := range []struct{ prompt, want string }{
		{"<pasted_content id=\"5c59\">\nFix the parser\nand the lexer\n</pasted_content id=\"5c59\">", "\nFix the parser\nand the lexer\n"},
		{"<pasted_content id=\"5c59\">Fix the parser</pasted_content id=\"5c59\">", "Fix the parser"},
		{"Review <pasted_content id=\"a\">one</pasted_content id=\"a\"> and <pasted_content id=\"b\">two</pasted_content id=\"b\">", "Review one and two"},
		{"Fix <div> rendering", "Fix <div> rendering"},
	} {
		input, _ := json.Marshal(map[string]string{"session_id": "s1", "prompt": c.prompt})
		run("UserPromptSubmit", string(input))
		b, err := os.ReadFile(filepath.Join(dir, "prompt"))
		if err != nil {
			t.Fatal(err)
		}
		var report struct{ Session, Text string }
		if err := json.Unmarshal(b, &report); err != nil {
			t.Fatal(err)
		}
		if report.Session != "s1" || report.Text != c.want {
			t.Errorf("prompt %q: report = %+v, want text %q", c.prompt, report, c.want)
		}
	}
	run("StopFailure", `{"error":"provider","error_details":" "}`)
	if b, _ := os.ReadFile(filepath.Join(dir, "failed")); string(b) != `{"error":"provider"}` {
		t.Errorf("failed report = %s", b)
	}
	if got := run("PostToolUse", `{"agent_id":"native"}`); got != "" {
		t.Error("a native subagent's event was reported", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "tool")); err == nil {
		t.Error("a native subagent's tool use was reported")
	}
	run("PermissionRequest", `{"agent_id":"native"}`)
	if _, err := os.Stat(filepath.Join(dir, "approval")); err != nil {
		t.Error("a native subagent's approval was not reported")
	}
	run("PostToolUseFailure", `{"tool_name":"Bash","error":"exit 1"}`)
	if _, err := os.Stat(filepath.Join(dir, "toolfailed")); err == nil {
		t.Error("another tool's failure was reported")
	}
	if got := run("PostToolUseFailure", `{"agent_id":"native","tool_name":"mcp__hypercube__send_message","tool_input":{"to":"x"},"error":"no agent"}`); got != "" {
		t.Error(got)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "toolfailed")); !strings.Contains(string(b), `"input":{"to":"x"}`) || !strings.Contains(string(b), `"tool":"mcp__hypercube__send_message"`) {
		t.Errorf("toolfailed report = %s", b)
	}
	run("PermissionDenied", `{"tool_name":"Bash","tool_input":{"command":"rm"},"reason":"destructive"}`)
	if b, _ := os.ReadFile(filepath.Join(dir, "denied")); !strings.Contains(string(b), `"reason":"destructive"`) {
		t.Errorf("denied report = %s", b)
	}
}

func TestStatusline(t *testing.T) {
	dir := t.TempDir()
	hypercube := filepath.Join(dir, "hypercube")
	if err := os.WriteFile(hypercube, []byte("#!/bin/sh\ncat >\""+dir+"/$2\"\necho '{}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `{"model":{"display_name":"Opus"},"thinking":{"enabled":true},"effort":{"level":"high"},"context_window":{"total_input_tokens":45300,"context_window_size":200000,"used_percentage":22.6},"cost":{"total_cost_usd":0.31},"rate_limits":{"five_hour":{"used_percentage":34},"seven_day":{"used_percentage":71.5}}}`
	ansi := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	for _, showLimits := range []bool{false, true} {
		var out bytes.Buffer
		if err := statusline(hypercube, showLimits, strings.NewReader(source), &out); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(filepath.Join(dir, "usage")); string(b) != `{"context":45200,"cost":0.31,"window":200000}` {
			t.Errorf("usage report = %s", b)
		}
		want := "✳︎ Opus :: high 45.3k/200k (22%)"
		if showLimits {
			want += " | 5h ▪▪▪······· | 7d ▪▪▪▪▪▪▪···"
		}
		if got := ansi.ReplaceAllString(out.String(), ""); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
	for _, source := range []string{`null`, `{}`, `{"model":null,"thinking":null}`, `malformed`} {
		var out bytes.Buffer
		if err := statusline(hypercube, false, strings.NewReader(source), &out); err != nil {
			t.Fatal(err)
		}
		if got := ansi.ReplaceAllString(out.String(), ""); got != "✳︎  ::  0/0 (0%)" {
			t.Fatal(got)
		}
	}
}
