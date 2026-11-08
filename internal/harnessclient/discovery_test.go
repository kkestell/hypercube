package harnessclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
)

// plugin writes a plugin script into dir.
func plugin(t *testing.T, dir, name, script string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, pluginPrefix+name), []byte("#!/bin/sh\n"+script), mode); err != nil {
		t.Fatal(err)
	}
}

func TestDescribe(t *testing.T) {
	dir := t.TempDir()
	plugin(t, dir, "one", `echo '{"name":"One","installed":true}'`, 0o755)
	plugin(t, dir, "two", `echo '{"name":"Two","installed":false}'`, 0o755)
	plugin(t, dir, "broken", `echo 'hypercube-harness-broken: no luck' >&2; exit 1`, 0o755)
	plugin(t, dir, "plain", `echo '{}'`, 0o644)
	if err := os.WriteFile(filepath.Join(dir, "hypercube"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Describe(context.Background(), dir)
	want := []harness.Harness{
		{ID: "broken", Error: "harness broken: no luck"},
		{ID: "one", Description: harness.Description{Name: "One", Installed: true}},
		{ID: "two", Description: harness.Description{Name: "Two"}},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Describe = %+v, %v", got, err)
	}
	if _, err := Find(dir, "plain"); err == nil {
		t.Fatal("found a plugin that is not executable")
	}
	if _, err := Find(dir, "../one"); err == nil {
		t.Fatal("found a plugin outside the plugin directory")
	}
}

func TestModels(t *testing.T) {
	dir := t.TempDir()
	req := harness.ModelsRequest{Dir: t.TempDir(), Model: "a/b"}
	for name, tc := range map[string]struct {
		output string
		want   string // the error, or empty when the catalog is valid
	}{
		"valid":            {`{"models":[{"id":"a/b","label":"B","provider":"A"},{"id":"c"}],"default":"c","model":"a/b","efforts":["low","high"],"default_effort":"high"}`, ""},
		"no efforts":       {`{"models":[{"id":"a/b"}],"model":"a/b"}`, ""},
		"empty ID":         {`{"models":[{"id":""}],"model":"a/b"}`, "a model has no ID"},
		"duplicate ID":     {`{"models":[{"id":"c"},{"id":"c"}],"model":"a/b"}`, `model "c" is listed twice`},
		"unlisted default": {`{"models":[{"id":"c"}],"default":"d","model":"a/b"}`, `default model "d" is not listed`},
		"other model":      {`{"models":[{"id":"c"}],"model":"c"}`, `the catalog is for model "c"`},
		"bad effort":       {`{"models":[{"id":"a/b"}],"model":"a/b","efforts":["low"],"default_effort":"max"}`, `default effort "max" is not listed`},
		"malformed":        {`{`, "malformed models output"},
	} {
		t.Run(name, func(t *testing.T) {
			plugin(t, dir, "fake", "cat >/dev/null\necho '"+tc.output+"'", 0o755)
			c, err := Models(context.Background(), dir, "fake", req)
			if tc.want == "" && (err != nil || c.Model != "a/b") || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("Models = %+v, %v; want %q", c, err, tc.want)
			}
		})
	}
	if _, err := Models(context.Background(), dir, "fake", harness.ModelsRequest{Dir: "relative"}); err == nil {
		t.Fatal("accepted a relative directory")
	}
	if _, err := Models(context.Background(), dir, "missing", req); err == nil {
		t.Fatal("asked a plugin that is not installed")
	}
}

// Cancelling a probe lets the plugin clean up, then ends its native
// processes, also when the plugin ignores the request to stop.
func TestModelsCancel(t *testing.T) {
	cancelGrace = time.Second
	for name, trap := range map[string]string{
		"cleans up": `trap 'echo cleaned > "$CLEANED"; exit 1' TERM`,
		"ignores":   `trap '' TERM`,
	} {
		t.Run(name, func(t *testing.T) {
			dir, state := t.TempDir(), t.TempDir()
			pidFile, cleaned := filepath.Join(state, "pid"), filepath.Join(state, "cleaned")
			t.Setenv("CLEANED", cleaned)
			// The plugin's own child stands for the native harness it starts.
			plugin(t, dir, "slow", trap+"\nsleep 600 &\necho $! > '"+pidFile+"'\nwhile :; do wait; done\n", 0o755)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() {
				_, err := Models(ctx, dir, "slow", harness.ModelsRequest{Dir: t.TempDir()})
				done <- err
			}()
			var pid string
			for pid == "" {
				data, _ := os.ReadFile(pidFile)
				pid = strings.TrimSpace(string(data))
			}
			cancel()
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled Models = %v", err)
			}
			if _, err := os.Stat(cleaned); (err == nil) != (name == "cleans up") {
				t.Fatalf("cleanup ran: %v", err)
			}
			deadline := time.Now().Add(10 * time.Second)
			for processExists(pid) {
				if time.Now().After(deadline) {
					t.Fatalf("process %s outlived the cancelled probe", pid)
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}

func processExists(pid string) bool {
	n, err := strconv.Atoi(pid)
	return err == nil && syscall.Kill(n, 0) == nil
}
