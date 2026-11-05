package main

import (
	"bytes"
	"fmt"
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
	for _, tc := range []struct {
		name, selection, override, want string
		fail                            bool
	}{
		{name: "no switcher", want: ".codex"},
		{name: "default", selection: "unset CODEX_HOME\n", want: ".codex"},
		{name: "Star Tribune", selection: "export CODEX_HOME=\"$HOME/.codex-strib\"\n", want: ".codex-strib"},
		{name: "explicit override", selection: "export CODEX_HOME=\"$HOME/.codex-strib\"\n", override: "explicit", want: "explicit"},
		{name: "override ignores invalid selection", selection: "invalid", override: "explicit", want: "explicit"},
		{name: "invalid selection", selection: "invalid", fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("CODEX_HOME", "")
			if tc.selection != "" {
				writeStorageFile(t, filepath.Join(home, ".config", "neuron", "codex-profile.zsh"), []byte(tc.selection))
			}
			if tc.override != "" {
				t.Setenv("CODEX_HOME", filepath.Join(home, tc.override))
			}
			got, err := sourceHome()
			if tc.fail {
				if err == nil {
					t.Fatal("invalid selection accepted")
				}
			} else if err != nil || got != filepath.Join(home, tc.want) {
				t.Fatalf("%q: %v", got, err)
			}
		})
	}
	t.Run("relative override", func(t *testing.T) {
		t.Setenv("CODEX_HOME", "relative-codex")
		want, err := filepath.Abs("relative-codex")
		if err != nil {
			t.Fatal(err)
		}
		if got, err := sourceHome(); err != nil || got != want {
			t.Fatalf("%q: %v", got, err)
		}
	})
	t.Run("selection read failure", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("CODEX_HOME", "")
		if err := os.MkdirAll(filepath.Join(home, ".config", "neuron", "codex-profile.zsh"), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := sourceHome(); err == nil {
			t.Fatal("selection read failure ignored")
		}
	})
}

func TestDefaultStorageWithoutSwitcherKeepsLegacyHome(t *testing.T) {
	user := t.TempDir()
	t.Setenv("HOME", user)
	t.Setenv("CODEX_HOME", "")
	source := filepath.Join(user, ".codex")
	writeStorageFile(t, filepath.Join(source, "auth.json"), []byte("default account"))
	args := harness.StartArgs{StateDir: t.TempDir()}
	home, err := prepareStorage(args)
	if err != nil || home != filepath.Join(args.StateDir, "codex") {
		t.Fatalf("%q: %v", home, err)
	}
	if target, err := os.Readlink(filepath.Join(home, "auth.json")); err != nil || target != filepath.Join(source, "auth.json") {
		t.Fatalf("%q: %v", target, err)
	}
}

func TestAccountSelectionKeepsSeparateHomesAndPrivateResumes(t *testing.T) {
	user := t.TempDir()
	t.Setenv("HOME", user)
	t.Setenv("CODEX_HOME", "")
	selection := filepath.Join(user, ".config", "neuron", "codex-profile.zsh")
	for _, name := range []string{".codex", ".codex-strib"} {
		writeStorageFile(t, filepath.Join(user, name, "auth.json"), []byte(name))
		writeStorageFile(t, filepath.Join(user, name, "config.toml"), []byte(name))
	}
	args := harness.StartArgs{StateDir: t.TempDir()}
	defaultHome, err := prepareStorage(args)
	if err != nil {
		t.Fatal(err)
	}
	writeStorageFile(t, selection, []byte("export CODEX_HOME=\"$HOME/.codex-strib\"\n"))
	stribHome, err := prepareStorage(args)
	if err != nil || stribHome == defaultHome {
		t.Fatalf("account homes not separated: %q: %v", stribHome, err)
	}
	writeStorageFile(t, selection, []byte("unset CODEX_HOME\n"))
	if home, err := prepareStorage(args); err != nil || home != defaultHome {
		t.Fatalf("default home changed: %q: %v", home, err)
	}
	for _, tc := range []struct{ home, source, id string }{
		{defaultHome, ".codex", "default-thread"},
		{stribHome, ".codex-strib", "strib-thread"},
	} {
		for _, name := range []string{"auth.json", "config.toml"} {
			if target, err := os.Readlink(filepath.Join(tc.home, name)); err != nil || target != filepath.Join(user, tc.source, name) {
				t.Fatalf("account link changed: %q: %v", target, err)
			}
		}
		writeStorageFile(t, filepath.Join(tc.home, "sessions", "2026", "11", "04", "rollout-"+tc.id+".jsonl"), []byte(tc.id))
		writeStorageFile(t, filepath.Join(user, tc.source, "skills", "new", "SKILL.md"), []byte(tc.source))
	}
	// Private resumes must not consult the selection for new conversations.
	writeStorageFile(t, selection, []byte("invalid selection"))
	t.Setenv("CODEX_HOME", filepath.Join(user, "explicit-other-account"))
	for _, tc := range []struct{ id, home, source string }{{"default-thread", defaultHome, ".codex"}, {"strib-thread", stribHome, ".codex-strib"}} {
		args.Conversation = tc.id
		if home, err := prepareStorage(args); err != nil || home != tc.home {
			t.Fatalf("private resume changed account: %q: %v", home, err)
		}
		if skill, err := os.ReadFile(filepath.Join(tc.home, "skills", "new", "SKILL.md")); err != nil || string(skill) != tc.source {
			t.Fatalf("resumed skill from wrong account: %q: %v", skill, err)
		}
	}
	t.Setenv("CODEX_HOME", "")
	if _, err := prepareStorage(args); err != nil {
		t.Fatalf("invalid selection broke private resume: %v", err)
	}
}

func TestPrivateResumeRemembersAnInitiallyEmptySource(t *testing.T) {
	source := t.TempDir()
	t.Setenv("CODEX_HOME", source)
	args := harness.StartArgs{StateDir: t.TempDir()}
	home, err := prepareStorage(args)
	if err != nil {
		t.Fatal(err)
	}
	writeStorageFile(t, filepath.Join(home, "sessions", "2026", "11", "04", "rollout-thread.jsonl"), []byte("history"))
	writeStorageFile(t, filepath.Join(source, "skills", "new", "SKILL.md"), []byte("original source"))
	other := t.TempDir()
	writeStorageFile(t, filepath.Join(other, "skills", "new", "SKILL.md"), []byte("other account"))
	t.Setenv("CODEX_HOME", other)
	args.Conversation = "thread"
	if got, err := prepareStorage(args); err != nil || got != home {
		t.Fatalf("private resume changed home: %q: %v", got, err)
	}
	if skill, err := os.ReadFile(filepath.Join(home, "skills", "new", "SKILL.md")); err != nil || string(skill) != "original source" {
		t.Fatalf("resumed skill = %q: %v", skill, err)
	}
}

func TestLegacyEmptyPrivateHomeUsesOnlyAMatchingSource(t *testing.T) {
	for _, matching := range []bool{true, false} {
		t.Run(fmt.Sprint(matching), func(t *testing.T) {
			source := t.TempDir()
			t.Setenv("CODEX_HOME", source)
			args := harness.StartArgs{StateDir: t.TempDir()}
			home, err := prepareStorage(args)
			if err != nil {
				t.Fatal(err)
			}
			// Older empty homes did not record their source directory.
			if err := os.Remove(filepath.Join(home, ".hypercube-source")); err != nil {
				t.Fatal(err)
			}
			writeStorageFile(t, filepath.Join(home, "sessions", "2026", "11", "04", "rollout-thread.jsonl"), []byte("history"))
			writeStorageFile(t, filepath.Join(source, "skills", "new", "SKILL.md"), []byte("original source"))
			if !matching {
				other := t.TempDir()
				writeStorageFile(t, filepath.Join(other, "skills", "new", "SKILL.md"), []byte("other account"))
				t.Setenv("CODEX_HOME", other)
			}
			args.Conversation = "thread"
			if got, err := prepareStorage(args); err != nil || got != home {
				t.Fatalf("private resume changed home: %q: %v", got, err)
			}
			skill, err := os.ReadFile(filepath.Join(home, "skills", "new", "SKILL.md"))
			if matching && (err != nil || string(skill) != "original source") {
				t.Fatalf("matching source not shared: %q: %v", skill, err)
			}
			if !matching && !os.IsNotExist(err) {
				t.Fatalf("unidentified home rebound to another account: %q: %v", skill, err)
			}
		})
	}
}

func TestDefaultLaunchDoesNotChangeLegacyOverrideLinks(t *testing.T) {
	user := t.TempDir()
	t.Setenv("HOME", user)
	t.Setenv("CODEX_HOME", "")
	args := harness.StartArgs{StateDir: t.TempDir()}
	legacy := filepath.Join(args.StateDir, "codex")
	otherAuth := filepath.Join(t.TempDir(), "auth.json")
	writeStorageFile(t, otherAuth, []byte("existing account"))
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(otherAuth, filepath.Join(legacy, "auth.json")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config.toml", "auth.json"} {
		writeStorageFile(t, filepath.Join(user, ".codex", name), []byte("default account"))
	}
	home, err := prepareStorage(args)
	if err != nil || home == legacy {
		t.Fatalf("legacy override home reused: %q: %v", home, err)
	}
	if target, err := os.Readlink(filepath.Join(legacy, "auth.json")); err != nil || target != otherAuth {
		t.Fatalf("legacy credential link changed: %q: %v", target, err)
	}
	if _, err := os.Lstat(filepath.Join(legacy, "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("legacy home polluted with another account's config: %v", err)
	}
	writeStorageFile(t, filepath.Join(legacy, "sessions", "2026", "11", "04", "rollout-legacy-thread.jsonl"), []byte("legacy history"))
	args.Conversation = "legacy-thread"
	if got, err := prepareStorage(args); err != nil || got != legacy {
		t.Fatalf("legacy override resume changed: %q: %v", got, err)
	}
}

func TestNativeResumeDoesNotSearchAnotherAccount(t *testing.T) {
	user := t.TempDir()
	t.Setenv("HOME", user)
	t.Setenv("CODEX_HOME", "")
	writeStorageFile(t, filepath.Join(user, ".codex", "sessions", "2026", "11", "04", "rollout-personal-thread.jsonl"), []byte("personal history"))
	selection := filepath.Join(user, ".config", "neuron", "codex-profile.zsh")
	writeStorageFile(t, selection, []byte("export CODEX_HOME=\"$HOME/.codex-strib\"\n"))
	args := harness.StartArgs{StateDir: t.TempDir(), Conversation: "personal-thread"}
	if _, err := prepareStorage(args); err == nil {
		t.Fatal("imported another account's native conversation")
	}
	writeStorageFile(t, selection, []byte("unset CODEX_HOME\n"))
	if _, err := prepareStorage(args); err != nil {
		t.Fatalf("native resume with its own account failed: %v", err)
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
	args := harness.StartArgs{StateDir: state}
	home, err := prepareStorage(args)
	if err != nil {
		t.Fatal(err)
	}
	writeStorageFile(t, filepath.Join(source, "config.toml"), []byte("user config"))
	writeStorageFile(t, filepath.Join(home, "config.toml"), []byte("unexpected file"))
	if _, err := prepareStorage(args); err == nil {
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
