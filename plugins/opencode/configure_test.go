package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/harness/harnesstest"
)

func TestServeConfigure(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
model=old
level=default
held=
options() {
 printf '{"id":%s,"result":{"sessionId":"s","configOptions":[{"id":"model","type":"select","currentValue":"%s","options":[{"value":"old"},{"value":"new"},{"value":"partial"}]},{"id":"effort","type":"select","currentValue":"%s","options":[{"value":"default"},{"value":"low"},{"value":"high"}]}]}}\n' "$id" "$model" "$level"
}
while IFS= read -r line; do
 id=$(printf '%s' "$line" | sed -n 's/.*"id":\([0-9]*\),.*/\1/p')
 case "$line" in
 *'"method":"initialize"'*) printf '{"id":%s,"result":{"protocolVersion":1}}\n' "$id";;
 *'"method":"session/new"'*) options;;
 *'"method":"session/set_config_option"'*)
  case "$line" in
  *'"configId":"model"'*)
   case "$line" in *'"value":"new"'*) model=new;; *'"value":"partial"'*) model=partial;; esac
   level=default;;
  *'"configId":"effort"'*)
   case "$line" in *'"value":"high"'*) level=high;; *'"value":"low"'*) level=low;; esac;;
  esac
  options;;
 *'"method":"session/prompt"'*)
  case "$line" in
  *'"text":"work"'*) held=$id;;
  *'"text":"queued"'*)
   echo '{"method":"session/update","params":{"sessionId":"s","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"queued reply"}}}}'
   printf '{"id":%s,"result":{"stopReason":"end_turn"}}\n' "$id";;
  *) exit 4;;
  esac;;
 *'"method":"session/cancel"'*) printf '{"id":%s,"result":{"stopReason":"cancelled"}}\n' "$held";;
 esac
done
`
	if err := os.WriteFile(filepath.Join(dir, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	c := harnesstest.Serve(t, serve)
	c.Send(harness.Command{ID: 1, Command: harness.CmdStart, Start: &harness.StartArgs{Model: "old", Effort: "low", Dir: t.TempDir(), StateDir: t.TempDir()}})
	if r := c.Reply(1); r.Error != "" {
		t.Fatal(r)
	}
	c.Send(harness.Command{ID: 2, Command: harness.CmdInput, Text: "work"})
	c.Reply(2)
	selected := harness.Selection{Model: "new", Effort: "high"}
	c.Send(harness.Command{ID: 3, Command: harness.CmdConfigure, Selection: &selected})
	if r := c.Reply(3); r.Error != "" || r.Selection == nil || *r.Selection != selected {
		t.Fatal(r)
	}
	c.Send(harness.Command{ID: 4, Command: harness.CmdInput, Text: "queued"})
	if r := c.Reply(4); r.Error != "" {
		t.Fatal(r)
	}
	c.Send(harness.Command{ID: 5, Command: harness.CmdConfigure, Selection: &harness.Selection{Model: "partial", Effort: "max"}})
	if r := c.Reply(5); r.Error == "" || r.Selection == nil || *r.Selection != (harness.Selection{Model: "partial", Effort: "default"}) {
		t.Fatal(r)
	}
	c.Send(harness.Command{ID: 6, Command: harness.CmdConfigure, Selection: &harness.Selection{Model: "unknown", Effort: "high"}})
	if r := c.Reply(6); r.Error == "" || r.Selection != nil {
		t.Fatal(r)
	}
	c.Send(harness.Command{ID: 7, Command: harness.CmdInterrupt})
	replied, queued := false, false
	c.Until(func(r harness.Record) bool {
		replied = replied || r.Reply != nil && r.Reply.ID == 7
		if r.Event != nil {
			for _, it := range r.Event.Items {
				queued = queued || it.Text == "queued reply"
			}
		}
		return replied && queued
	})
}
