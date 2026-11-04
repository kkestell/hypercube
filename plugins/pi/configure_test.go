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
level=low
window=100
while IFS= read -r line; do
 id=$(printf '%s' "$line" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
 case "$line" in
 *'"type":"get_state"'*) printf '{"id":"%s","type":"response","success":true,"data":{"model":{"provider":"p","id":"%s","contextWindow":%s},"thinkingLevel":"%s","sessionId":"s"}}\n' "$id" "$model" "$window" "$level";;
 *'"type":"set_model"'*)
  case "$line" in
  *'"modelId":"new"'*'"provider":"p"'*) model=new; window=900;;
  *'"modelId":"partial"'*) model=partial; window=1200;;
  *) printf '{"id":"%s","type":"response","success":false,"error":"bad model"}\n' "$id"; continue;;
  esac
  printf '{"id":"%s","type":"response","success":true}\n' "$id";;
 *'"type":"set_thinking_level"'*)
  case "$line" in
  *'"level":"high"'*) level=medium; printf '{"id":"%s","type":"response","success":true}\n' "$id";;
  *) printf '{"id":"%s","type":"response","success":false,"error":"bad effort"}\n' "$id";;
  esac;;
 *'"type":"prompt"'*)
  case "$line" in *'"streamingBehavior":"steer"'*) ;; *) exit 4;; esac
  printf '{"id":"%s","type":"response","success":true}\n' "$id"
  case "$line" in
  *'"message":"work"'*) echo '{"type":"agent_start"}';;
  *) echo '{"type":"agent_settled"}';;
  esac;;
 *'"type":"get_session_stats"'*) printf '{"id":"%s","type":"response","success":true,"data":{"cost":1}}\n' "$id";;
 esac
done
`
	if err := os.WriteFile(filepath.Join(dir, "pi"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	c := harnesstest.Serve(t, serve)
	c.Send(harness.Command{ID: 1, Command: harness.CmdStart, Start: &harness.StartArgs{Model: "p/old", Effort: "low", Dir: t.TempDir(), StateDir: t.TempDir()}})
	if r := c.Reply(1); r.Error != "" {
		t.Fatal(r)
	}
	c.Until(func(r harness.Record) bool { return r.Event != nil && r.Event.State == harness.Idle })
	c.Send(harness.Command{ID: 2, Command: harness.CmdInput, Text: "work"})
	replied, working := false, false
	c.Until(func(r harness.Record) bool {
		replied = replied || r.Reply != nil && r.Reply.ID == 2
		working = working || r.Event != nil && r.Event.State == harness.Working
		return replied && working
	})
	c.Send(harness.Command{ID: 3, Command: harness.CmdConfigure, Selection: &harness.Selection{Model: "p/new", Effort: "high"}})
	if r := c.Reply(3); r.Error != "" || r.Selection == nil || *r.Selection != (harness.Selection{Model: "p/new", Effort: "medium"}) {
		t.Fatal(r)
	}
	c.Send(harness.Command{ID: 4, Command: harness.CmdInput, Text: "steer"})
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
	c.Send(harness.Command{ID: 5, Command: harness.CmdConfigure, Selection: &harness.Selection{Model: "p/partial", Effort: "bad"}})
	if r := c.Reply(5); r.Error == "" || r.Selection == nil || *r.Selection != (harness.Selection{Model: "p/partial", Effort: "medium"}) {
		t.Fatal(r)
	}
	c.Send(harness.Command{ID: 6, Command: harness.CmdConfigure, Selection: &harness.Selection{Model: "p/unknown", Effort: "high"}})
	if r := c.Reply(6); r.Error == "" || r.Selection != nil {
		t.Fatal(r)
	}
}
