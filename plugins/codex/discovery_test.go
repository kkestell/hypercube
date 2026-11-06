package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/plugins/internal/plugin"
)

func TestConnectChecksSelection(t *testing.T) {
	for name, tc := range map[string]struct {
		model, effort, want string
	}{
		"unknown model":  {"other", "low", `does not offer model "other"`},
		"unknown effort": {"m", "max", `does not accept effort "max"`},
	} {
		t.Run(name, func(t *testing.T) {
			h := newAgent(t)
			h.server.handle("initialize", func(map[string]any) (any, *plugin.RPCError) { return map[string]any{}, nil })
			err := h.connect(context.Background(), harness.StartArgs{Model: tc.model, Effort: tc.effort, Dir: t.TempDir()})
			if err == nil || !strings.Contains(err.Error(), tc.want) || len(h.server.sent("thread/start")) != 0 {
				t.Fatalf("connect = %v, want %q", err, tc.want)
			}
		})
	}
	// An empty effort starts with the model's default, which becomes the
	// session's accepted selection.
	h := newAgent(t)
	h.server.handle("initialize", func(map[string]any) (any, *plugin.RPCError) { return map[string]any{}, nil })
	h.server.handle("thread/start", func(map[string]any) (any, *plugin.RPCError) {
		return map[string]any{"thread": map[string]any{"id": "thread-1"}}, nil
	})
	if err := h.connect(context.Background(), harness.StartArgs{Model: "m", Dir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	config := h.server.sent("thread/start")[0]["config"].(map[string]any)
	if config["model_reasoning_effort"] != "medium" || h.selection != (harness.Selection{Model: "m", Effort: "medium"}) {
		t.Fatalf("config %+v, selection %+v", config, h.selection)
	}
	// Choosing the default again replaces an explicit effort.
	h.selection.Effort = "low"
	got, err := h.configure(harness.Selection{Model: "m"})
	if err != nil || *got != (harness.Selection{Model: "m", Effort: "medium"}) || h.selection != *got {
		t.Fatalf("configure = %+v, %v", got, err)
	}
	if _, err := h.configure(harness.Selection{Model: "m", Effort: "max"}); err == nil || h.selection.Effort != "medium" {
		t.Fatalf("configure accepted an unknown effort: %v, %+v", err, h.selection)
	}
}

func TestModels(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	bin := t.TempDir()
	// Two pages of models; the second page names the hidden model.
	script := `#!/bin/sh
while IFS= read -r line; do
 id=$(printf '%s' "$line" | sed -n 's/.*"id":\([0-9]*\),.*/\1/p')
 case "$line" in
 *'"method":"initialize"'*) printf '{"id":%s,"result":{}}\n' "$id";;
 *'"method":"model/list"'*'"cursor":"p2"'*) printf '{"id":%s,"result":{"data":[{"id":"hid","model":"hid","hidden":true},{"id":"b","model":"b","displayName":"B"}],"nextCursor":null}}\n' "$id";;
 *'"method":"model/list"'*) printf '{"id":%s,"result":{"data":[{"id":"a","model":"a","displayName":"A","isDefault":true,"defaultReasoningEffort":"high","supportedReasoningEfforts":[{"reasoningEffort":"low"},{"reasoningEffort":"high"}]}],"nextCursor":"p2"}}\n' "$id";;
 *'"method":"thread/'*) exit 9;;
 esac
done
`
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	dir := t.TempDir()
	got, err := models(context.Background(), harness.ModelsRequest{Dir: dir})
	// A model Codex hides from its own picker is still a valid selection.
	want := harness.Catalog{Models: []harness.Model{{ID: "a", Label: "A"}, {ID: "hid"}, {ID: "b", Label: "B"}}, Default: "a"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %+v, %v", got, err)
	}
	got, err = models(context.Background(), harness.ModelsRequest{Dir: dir, Model: "a"})
	if err != nil || !reflect.DeepEqual(got.Efforts, []string{"low", "high"}) || got.DefaultEffort != "high" {
		t.Fatalf("models for a = %+v, %v", got, err)
	}
	if _, err := models(context.Background(), harness.ModelsRequest{Dir: dir, Model: "c"}); err == nil {
		t.Fatal("listed efforts for a model that is not offered")
	}
}
