package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/harness/harnesstest"
)

func TestServeConfigureAliasUsesResolvedModelWindow(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
while IFS= read -r line; do
 case "$line" in
 *'"subtype":"initialize"'*) echo "$FAKE_INITIALIZED";;
 *'"subtype":"apply_flag_settings"'*)
  id=$(printf '%s' "$line" | sed -n 's/.*"request_id":"\([^"]*\)".*/\1/p')
  case "$line" in
  *'"model":"bad"'*) printf '{"type":"control_response","response":{"subtype":"error","request_id":"%s","error":"bad model"}}\n' "$id";;
  *'"effortLevel":"max","model":"haiku"'*)
   echo '{"type":"control_response","response":{"subtype":"error","request_id":"other","error":"unrelated"}}'
   printf '{"type":"control_response","response":{"subtype":"success","request_id":"%s"}}\n' "$id";;
  *'"effortLevel":null,"model":"opus"'*) printf '{"type":"control_response","response":{"subtype":"success","request_id":"%s"}}\n' "$id";;
  *) exit 4;;
  esac;;
 *'"content":"work"'*)
  echo '{"type":"result","modelUsage":{"claude-opus-resolved":{"contextWindow":100}}}'
  echo '{"type":"system","subtype":"session_state_changed","state":"running"}';;
 *'"type":"user"'*)
  echo '{"type":"assistant","message":{"id":"main","model":"claude-haiku-resolved","content":[{"type":"text","text":"OK"}]}}'
  echo '{"type":"assistant","parent_tool_use_id":"child","message":{"id":"subagent","model":"claude-opus-resolved","content":[{"type":"text","text":"child"}]}}'
  echo '{"type":"result","modelUsage":{"claude-opus-resolved":{"contextWindow":100},"claude-haiku-resolved":{"contextWindow":900}}}'
  echo '{"type":"system","subtype":"session_state_changed","state":"idle"}';;
 esac
done
`
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_INITIALIZED", initializedLine)
	c := harnesstest.Serve(t, serve)
	c.Send(harness.Command{ID: 1, Command: harness.CmdStart, Start: &harness.StartArgs{MCP: "/bin/hc-mcp", Model: "opus", Effort: "high", Dir: t.TempDir()}})
	if r := c.Reply(1); r.Error != "" {
		t.Fatal(r)
	}
	c.Until(func(r harness.Record) bool { return r.Event != nil && r.Event.State == harness.Idle })
	c.Send(harness.Command{ID: 2, Command: harness.CmdInput, Text: "work"})
	c.Reply(2)
	c.Until(func(r harness.Record) bool { return r.Event != nil && r.Event.State == harness.Working })
	selected := harness.Selection{Model: "haiku", Effort: "max"}
	c.Send(harness.Command{ID: 3, Command: harness.CmdConfigure, Selection: &selected})
	// An unrelated control response must not settle the flag override.
	if r := c.Reply(3); r.Error != "" || r.Selection == nil || *r.Selection != selected {
		t.Fatal(r)
	}
	c.Send(harness.Command{ID: 4, Command: harness.CmdInput, Text: "continue"})
	replied, used := false, false
	c.Until(func(r harness.Record) bool {
		replied = replied || r.Reply != nil && r.Reply.ID == 4
		if r.Event != nil && r.Event.Usage != nil {
			if r.Event.Usage.Window != 900 {
				t.Fatal(r.Event.Usage)
			}
			used = true
		}
		return replied && used
	})

	c.Send(harness.Command{ID: 5, Command: harness.CmdConfigure, Selection: &harness.Selection{Model: "bad", Effort: "max"}})
	if r := c.Reply(5); !strings.Contains(r.Error, `does not offer model "bad"`) || r.Selection != nil {
		t.Fatal(r)
	}
	c.Send(harness.Command{ID: 6, Command: harness.CmdConfigure, Selection: &harness.Selection{Model: "haiku", Effort: "high"}})
	if r := c.Reply(6); !strings.Contains(r.Error, `does not accept effort "high"`) || r.Selection != nil {
		t.Fatal(r)
	}
	// An empty effort clears the override instead of keeping max.
	cleared := harness.Selection{Model: "opus"}
	c.Send(harness.Command{ID: 7, Command: harness.CmdConfigure, Selection: &cleared})
	if r := c.Reply(7); r.Error != "" || r.Selection == nil || *r.Selection != cleared {
		t.Fatal(r)
	}
}
