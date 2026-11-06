package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
)

func TestBeginReportsTheSelectedEffort(t *testing.T) {
	// An empty effort keeps the effort OpenCode selects with the model.
	h := connected(t)
	defaultHandlers(h, true)
	if err := h.begin(context.Background(), harness.StartArgs{Model: "p/m", Dir: "/work"}); err != nil {
		t.Fatal(err)
	}
	if set := h.acp.sent("session/set_config_option"); len(set) != 1 || h.started != (harness.Selection{Model: "p/m", Effort: "p/m"}) {
		t.Fatalf("set %+v, started %+v", set, h.started)
	}
	// A saved "default" for a model without an effort setting becomes empty.
	h = connected(t)
	defaultHandlers(h, false)
	if err := h.begin(context.Background(), harness.StartArgs{Model: "p/m", Effort: "default", Dir: "/work"}); err != nil {
		t.Fatal(err)
	}
	if h.started != (harness.Selection{Model: "p/m"}) {
		t.Fatalf("started %+v", h.started)
	}
}

func TestModels(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "log")
	script := `#!/bin/sh
model=p/a
options() {
 printf '{"jsonrpc":"2.0","id":%s,"result":{"sessionId":"ses_probe","configOptions":[{"id":"model","type":"select","currentValue":"%s","options":[{"value":"p/a","name":"A"},{"group":"q","name":"Q Provider","options":[{"value":"q/b/c","name":"B"}]}]}%s]}}\n' "$id" "$model" "$effort"
}
while IFS= read -r line; do
 printf '%s\n' "$line" >> "$FAKE_LOG"
 id=$(printf '%s' "$line" | sed -n 's/.*"id":\([0-9]*\),.*/\1/p')
 case "$line" in
 *'"method":"initialize"'*) printf '{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":1,"agentCapabilities":{"sessionCapabilities":{"delete":{}}}}}\n' "$id";;
 *'"method":"session/new"'*) effort=; options;;
 *'"method":"session/set_config_option"'*) model=q/b/c; effort=',{"id":"effort","type":"select","currentValue":"medium","options":[{"value":"low"},{"value":"medium"}]}'; options;;
 *'"method":"session/delete"'*) printf '{"jsonrpc":"2.0","id":%s,"result":{}}\n' "$id";;
 *) exit 7;;
 esac
done
`
	if err := os.WriteFile(filepath.Join(bin, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_LOG", log)
	dir := t.TempDir()
	got, err := models(context.Background(), harness.ModelsRequest{Dir: dir})
	want := harness.Catalog{Default: "p/a", Models: []harness.Model{{ID: "p/a", Label: "A", Provider: "p"}, {ID: "q/b/c", Label: "B", Provider: "Q Provider"}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %+v, %v", got, err)
	}
	got, err = models(context.Background(), harness.ModelsRequest{Dir: dir, Model: "q/b/c"})
	if err != nil || !reflect.DeepEqual(got.Efforts, []string{"low", "medium"}) || got.DefaultEffort != "medium" {
		t.Fatalf("models for q/b/c = %+v, %v", got, err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	sent := string(data)
	if strings.Count(sent, `"method":"session/delete"`) != 2 || strings.Contains(sent, "session/prompt") || !strings.Contains(sent, `"mcpServers":[]`) {
		t.Fatalf("discovery sent:\n%s", sent)
	}
}

// The temporary session is deleted when discovery is cancelled after OpenCode
// opened it, and a failed deletion fails discovery.
func TestModelsDeletesItsSession(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "log")
	script := `#!/bin/sh
while IFS= read -r line; do
 printf '%s\n' "$line" >> "$FAKE_LOG"
 id=$(printf '%s' "$line" | sed -n 's/.*"id":\([0-9]*\),.*/\1/p')
 case "$line" in
 *'"method":"initialize"'*) printf '{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":1,"agentCapabilities":{"sessionCapabilities":{"delete":{}}}}}\n' "$id";;
 *'"method":"session/new"'*) printf '{"jsonrpc":"2.0","id":%s,"result":{"sessionId":"probe","configOptions":[{"id":"model","type":"select","currentValue":"p/a","options":[{"value":"p/a"}]}]}}\n' "$id";;
 *'"method":"session/set_config_option"'*) ;;
 *'"method":"session/delete"'*)
  if [ -n "$FAKE_DELETE_FAILS" ]; then printf '{"jsonrpc":"2.0","id":%s,"error":{"code":-32603,"message":"locked"}}\n' "$id"
  else printf '{"jsonrpc":"2.0","id":%s,"result":{}}\n' "$id"; fi;;
 esac
done
`
	if err := os.WriteFile(filepath.Join(bin, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_LOG", log)
	t.Setenv("FAKE_DELETE_FAILS", "")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := models(ctx, harness.ModelsRequest{Dir: t.TempDir(), Model: "p/a"})
		done <- err
	}()
	for {
		data, _ := os.ReadFile(log)
		if strings.Contains(string(data), "session/set_config_option") {
			break
		}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled models = %v", err)
	}
	if data, _ := os.ReadFile(log); !strings.Contains(string(data), `"method":"session/delete","params":{"sessionId":"probe"}`) {
		t.Fatalf("cancelled discovery sent:\n%s", data)
	}
	t.Setenv("FAKE_DELETE_FAILS", "1")
	if c, err := models(context.Background(), harness.ModelsRequest{Dir: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "deleting OpenCode's discovery session probe") || len(c.Models) != 0 {
		t.Fatalf("models with a failed deletion = %+v, %v", c, err)
	}
}
