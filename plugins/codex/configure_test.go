package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/harness/harnesstest"
)

func TestServeConfigure(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	dir := t.TempDir()
	script := `#!/bin/sh
n=0
while IFS= read -r line; do
 id=$(printf '%s' "$line" | sed -n 's/.*"id":\([0-9]*\),.*/\1/p')
 case "$line" in
 *'"method":"initialize"'*) printf '{"id":%s,"result":{}}\n' "$id";;
 *'"method":"thread/start"'*) printf '{"id":%s,"result":{"thread":{"id":"th"}}}\n' "$id";;
 *'"method":"turn/start"'*)
  n=$((n+1)); turn="tu$n"
  case "$line" in
  *'"model":"bad"'*) printf '{"id":%s,"error":{"code":-32602,"message":"model refused"}}\n' "$id";;
  *'"model":"old"'*|*'"effort":"high"'*'"model":"new"'*) printf '{"id":%s,"result":{"turn":{"id":"%s"}}}\n' "$id" "$turn";;
  *) exit 4;;
  esac;;
 *'"method":"turn/steer"'*)
  case "$line" in *'"model"'*|*'"effort"'*) exit 5;; esac
  printf '{"id":%s,"result":{}}\n' "$id"
  printf '{"method":"turn/completed","params":{"threadId":"th","turn":{"id":"%s","status":"completed"}}}\n' "$turn";;
 esac
done
`
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	c := harnesstest.Serve(t, serve)
	c.Send(harness.Command{ID: 1, Command: harness.CmdStart, Start: &harness.StartArgs{Model: "old", Effort: "low", Dir: t.TempDir(), StateDir: t.TempDir()}})
	if r := c.Reply(1); r.Error != "" {
		t.Fatal(r)
	}
	c.Send(harness.Command{ID: 2, Command: harness.CmdInput, Text: "work"})
	if r := c.Reply(2); r.Error != "" {
		t.Fatal(r)
	}
	selected := harness.Selection{Model: "new", Effort: "high"}
	c.Send(harness.Command{ID: 3, Command: harness.CmdConfigure, Selection: &selected})
	if r := c.Reply(3); r.Error != "" || r.Selection == nil || *r.Selection != selected {
		t.Fatal(r)
	}
	c.Send(harness.Command{ID: 4, Command: harness.CmdInput, Text: "steer"})
	replied, idle := false, false
	c.Until(func(r harness.Record) bool {
		if r.Reply != nil && r.Reply.ID == 4 {
			if r.Reply.Error != "" {
				t.Fatal(r.Reply)
			}
			replied = true
		}
		idle = idle || r.Event != nil && r.Event.State == harness.Idle
		return replied && idle
	})
	c.Send(harness.Command{ID: 5, Command: harness.CmdInput, Text: "next turn"})
	if r := c.Reply(5); r.Error != "" {
		t.Fatal(r)
	}
	c.Send(harness.Command{ID: 6, Command: harness.CmdInput, Text: "finish"})
	replied, idle = false, false
	c.Until(func(r harness.Record) bool {
		replied = replied || r.Reply != nil && r.Reply.ID == 6
		idle = idle || r.Event != nil && r.Event.State == harness.Idle
		return replied && idle
	})
	c.Send(harness.Command{ID: 7, Command: harness.CmdConfigure, Selection: &harness.Selection{Model: "bad", Effort: "high"}})
	if r := c.Reply(7); r.Error != "" || r.Selection.Model != "bad" {
		t.Fatal(r)
	}
	c.Send(harness.Command{ID: 8, Command: harness.CmdInput, Text: "refused"})
	if r := c.Reply(8); !strings.Contains(r.Error, "model refused") {
		t.Fatal(r)
	}
}
