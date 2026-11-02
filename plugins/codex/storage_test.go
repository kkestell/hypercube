package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
)

func writeStorageFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSourceHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")
	if got, err := sourceHome(); err != nil || got != filepath.Join(home, ".codex") {
		t.Fatalf("%q: %v", got, err)
	}
	inherited := t.TempDir()
	t.Setenv("CODEX_HOME", inherited)
	if got, err := sourceHome(); err != nil || got != inherited {
		t.Fatalf("%q: %v", got, err)
	}
}

func TestStorageSharesInputsAndIsolatesOutputs(t *testing.T) {
	source := t.TempDir()
	t.Setenv("CODEX_HOME", source)
	for _, name := range sharedInputs {
		path := filepath.Join(source, name)
		switch name {
		case "skills", "agents", "rules", "hooks", "plugins":
			path = filepath.Join(path, "entry")
		}
		writeStorageFile(t, path, []byte(name))
	}
	writeStorageFile(t, filepath.Join(source, "state_5.sqlite"), []byte("private user history"))
	writeStorageFile(t, filepath.Join(source, "unknown-input"), []byte("unrelated"))
	args := harness.StartArgs{StateDir: t.TempDir()}
	home, err := prepareStorage(args)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range sharedInputs {
		got, err := os.Readlink(filepath.Join(home, name))
		if err != nil || got != filepath.Join(source, name) {
			t.Fatalf("%s: %q %v", name, got, err)
		}
	}
	if got, err := os.ReadFile(filepath.Join(home, "skills", "entry")); err != nil || string(got) != "skills" {
		t.Fatalf("shared skill directory: %q %v", got, err)
	}
	for _, name := range []string{"state_5.sqlite", "sessions", "unknown-input"} {
		if _, err := os.Lstat(filepath.Join(home, name)); !os.IsNotExist(err) {
			t.Fatalf("copied unrelated storage %s: %v", name, err)
		}
	}
	// Native file-auth refresh resolves the link before atomic replacement.
	// Keep that same shared target and observe subsequent config/auth changes.
	for _, name := range []string{"auth.json", "config.toml"} {
		path := filepath.Join(home, name)
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			t.Fatal(err)
		}
		writeStorageFile(t, target+".tmp", []byte("refreshed"))
		if err := os.Rename(target+".tmp", target); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "refreshed" {
			t.Fatalf("%s: %q %v", name, got, err)
		}
		if _, err := os.Readlink(path); err != nil {
			t.Fatal(err)
		}
	}
	if again, err := prepareStorage(args); err != nil || again != home {
		t.Fatalf("repeat setup: %q %v", again, err)
	}
}

func TestStorageImportsOnlyRequestedRolloutAndPreservesPrivateResume(t *testing.T) {
	source := t.TempDir()
	t.Setenv("CODEX_HOME", source)
	relative := filepath.Join("sessions", "2026", "11", "02", "rollout-2026-11-02T12-00-00-saved.jsonl")
	content := bytes.Repeat([]byte("{\"complete\":\"history\"}\n"), 10000)
	writeStorageFile(t, filepath.Join(source, relative), content)
	writeStorageFile(t, filepath.Join(source, "sessions", "2026", "11", "02", "rollout-other.jsonl"), []byte("other"))
	args := harness.StartArgs{StateDir: t.TempDir(), Conversation: "saved"}
	home, err := prepareStorage(args)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(home, relative))
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("rollout changed: %v", err)
	}
	if matches, _ := filepath.Glob(filepath.Join(home, "sessions", "*", "*", "*", "*.jsonl")); len(matches) != 1 {
		t.Fatalf("imported unrelated rollouts: %v", matches)
	}
	// Once private, its continued native history wins over the old source.
	writeStorageFile(t, filepath.Join(home, relative), []byte("continued"))
	if _, err := prepareStorage(args); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(home, relative)); string(got) != "continued" {
		t.Fatalf("overwrote private history: %q", got)
	}
	args.Conversation = "missing"
	if _, err := prepareStorage(args); err == nil {
		t.Fatal("missing conversation accepted")
	}
}

func TestStorageReportsSetupFailures(t *testing.T) {
	source := t.TempDir()
	t.Setenv("CODEX_HOME", source)
	if _, err := prepareStorage(harness.StartArgs{}); err == nil {
		t.Fatal("missing state directory accepted")
	}
	state := filepath.Join(t.TempDir(), "file")
	writeStorageFile(t, state, []byte("not a directory"))
	if _, err := prepareStorage(harness.StartArgs{StateDir: state}); err == nil {
		t.Fatal("unwritable storage accepted")
	}
	state = t.TempDir()
	writeStorageFile(t, filepath.Join(source, "config.toml"), []byte("user config"))
	writeStorageFile(t, filepath.Join(state, "codex", "config.toml"), []byte("unexpected file"))
	if _, err := prepareStorage(harness.StartArgs{StateDir: state}); err == nil {
		t.Fatal("overwrote existing input")
	}
}

func TestConcurrentStorageSetup(t *testing.T) {
	source := t.TempDir()
	t.Setenv("CODEX_HOME", source)
	for _, name := range sharedInputs {
		writeStorageFile(t, filepath.Join(source, name), []byte(name))
	}
	for range 10 {
		args := harness.StartArgs{StateDir: t.TempDir()}
		start := make(chan struct{})
		errs := make(chan error, 16)
		var workers sync.WaitGroup
		for range 16 {
			workers.Go(func() {
				<-start
				_, err := prepareStorage(args)
				errs <- err
			})
		}
		close(start)
		workers.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}
