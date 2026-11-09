package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/config"
	"github.com/kkestell/hypercube/internal/harness"
)

// createCurrentDatabase creates the installed schema in a new state folder
// and runs setup, raw SQL, against it.
func createCurrentDatabase(t *testing.T, setup string) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	ddl, err := os.ReadFile("testdata/current-schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", dbPath())
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(string(ddl) + setup)
	closeErr := raw.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("creating current database: %v, close: %v", err, closeErr)
	}
}

// writeConfig sets a new config folder holding config.json with source, and
// returns the file's path.
func writeConfig(t *testing.T, source string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "config")
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "hypercube", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// hasModelEntriesTable reports whether the database still has the table
// earlier builds saved model entries in.
func hasModelEntriesTable(t *testing.T, db *DB) bool {
	t.Helper()
	var found bool
	if err := db.Read(func(tx *Tx) error {
		return tx.tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE name = 'model_entries')`).Scan(&found)
	}); err != nil {
		t.Fatal(err)
	}
	return found
}

func TestOpenCurrentDatabasePreservesState(t *testing.T) {
	configPath := writeConfig(t, `{"theme":"dark","models":[{"harness":"opencode","model":"provider/model","title":"My model","efforts":["low","high"],"default_effort":"high"}]}`)
	createCurrentDatabase(t, `
INSERT INTO model_entries VALUES ('a', 'opencode', 'provider/model', 'Other title', '[]', '');
INSERT INTO model_entries VALUES ('b', 'claude', 'opus', 'Opus', '["low","high"]', 'high');`)
	db := reopen(t)
	if hasModelEntriesTable(t, db) {
		t.Fatal("model_entries remains after opening")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	wantModels := []config.ModelEntry{
		{Harness: "opencode", Model: "provider/model", Title: "My model", Efforts: []string{"low", "high"}, DefaultEffort: "high"},
		{Harness: "claude", Model: "opus", Title: "Opus", Efforts: []string{"low", "high"}, DefaultEffort: "high"},
	}
	if cfg.Theme != "dark" || !reflect.DeepEqual(cfg.Models, wantModels) {
		t.Fatalf("moved config = %+v", cfg)
	}
	moved, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	workspaces := []Workspace{
		{ID: "w", Path: "/repo", Created: 10},
		{ID: "plain", Path: "/plain", NoGit: true, Created: 20},
	}
	root := Session{ID: "root", Workspace: "w", Title: "Saved session", Unseen: true,
		Harness: "opencode", Model: "provider/model", Effort: "high", Dir: "/worktree", Branch: "hypercube/root",
		Conversation: "root-conversation", Activity: Idle, Attention: true,
		Usage: harness.Usage{Context: 100, Window: 200, Cost: 1.5}, Created: 100,
		Launch: "root-launch", Position: 20, Ready: 30, Quick: 2, Base: "base-commit"}
	child := Session{ID: "child", Workspace: "w", Parent: root.ID, Name: "worker", Title: "Saved child",
		Harness: "opencode", Model: "provider/model", Effort: "low", Dir: "/child", Branch: "hypercube/child",
		Fork: "root-conversation", Activity: Busy, InputPending: true, Created: 110, Launch: "child-launch"}
	entries := map[string][]Entry{
		root.ID: {
			{Seq: 1, Item: "prompt", Data: []byte(`{"id":"prompt","kind":"user","text":"Saved prompt","done":true}`)},
			{Seq: 2, Item: "reply", Data: []byte(`{"id":"reply","kind":"text","text":"Saved reply","done":true}`)},
		},
		child.ID: {{Seq: 1, Item: "brief", Data: []byte(`{"id":"brief","kind":"user","text":"Saved brief","done":true}`)}},
	}
	message := Message{ID: 1, From: root.ID, To: child.ID, Text: "Queued report", Report: true}
	command := Command{Session: child.ID, Command: "input", Message: message.ID, Seq: 2,
		Item: []byte(`{"id":"input","kind":"user","text":"Queued report","done":true}`)}
	tx(t, db, func(tx *Tx) error {
		for _, w := range workspaces {
			if err := tx.AddWorkspace(w); err != nil {
				return err
			}
		}
		for _, s := range []Session{root, child} {
			if err := tx.AddSession(s); err != nil {
				return err
			}
			for _, e := range entries[s.ID] {
				if _, err := tx.PutEntry(s.ID, e); err != nil {
					return err
				}
			}
		}
		if err := tx.Queue(message); err != nil {
			return err
		}
		var err error
		command.ID, err = tx.AddCommand(command)
		if err != nil {
			return err
		}
		command.Line = fmt.Sprintf("{\"id\":%d,\"command\":\"input\",\"text\":\"Queued report\"}\n", command.ID)
		if err := tx.SetCommandLine(command.ID, command.Line); err != nil {
			return err
		}
		return nil
	})
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	again := reopen(t)
	if after, err := os.ReadFile(configPath); err != nil || string(after) != string(moved) {
		t.Fatalf("reopening changed config.json: %s, %v", after, err)
	}
	if err := again.Read(func(tx *Tx) error {
		gotWorkspaces, err := tx.Workspaces()
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(gotWorkspaces, workspaces) {
			t.Errorf("workspaces = %+v, want %+v", gotWorkspaces, workspaces)
		}
		for _, want := range []Session{root, child} {
			got, err := tx.Session(want.ID)
			if err != nil {
				return err
			}
			if got != want {
				t.Errorf("session = %+v, want %+v", got, want)
			}
			gotEntries, err := tx.Transcript(want.ID)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(gotEntries, entries[want.ID]) {
				t.Errorf("%s transcript = %+v, want %+v", want.ID, gotEntries, entries[want.ID])
			}
			if seq, err := tx.LastSeq(want.ID); err != nil || seq != 2 {
				t.Errorf("%s last sequence = %d, %v; want 2", want.ID, seq, err)
			}
		}
		messages, err := tx.Queued(child.ID)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(messages, []Message{message}) {
			t.Errorf("queued = %+v, want %+v", messages, message)
		}
		commands, err := tx.Commands(child.ID)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(commands, []Command{command}) {
			t.Errorf("commands = %+v, want %+v", commands, command)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// The installed schema requires last_active even after reopening.
	next := Session{ID: "next", Workspace: "plain", Dir: "/plain", Activity: Exited, Created: 120}
	tx(t, again, func(tx *Tx) error { return tx.AddSession(next) })
	if err := again.Read(func(tx *Tx) error {
		got, err := tx.Session(next.ID)
		if err == nil && got != next {
			t.Errorf("new session = %+v, want %+v", got, next)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestOpenAddsTipToSavedSessions(t *testing.T) {
	writeConfig(t, `{}`)
	createCurrentDatabase(t, `
INSERT INTO workspaces VALUES ('w', '/repo', 0, 10);
INSERT INTO sessions (id, workspace, title, ended, harness, model, effort, dir, branch, activity, created, base, last_active)
 VALUES ('archived', 'w', 'Archived session', 1, 'opencode', 'provider/model', 'high', '/worktree', 'hypercube/archived', 'exited', 100, 'base-commit', 100);`)
	db := reopen(t)
	want := Session{ID: "archived", Workspace: "w", Title: "Archived session", Ended: true,
		Harness: "opencode", Model: "provider/model", Effort: "high", Dir: "/worktree", Branch: "hypercube/archived",
		Activity: Exited, Created: 100, Base: "base-commit"}
	read := func() (s Session) {
		t.Helper()
		if err := db.Read(func(tx *Tx) error { var err error; s, err = tx.Session(want.ID); return err }); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if got := read(); got != want {
		t.Fatalf("saved session = %+v, want %+v", got, want)
	}
	want.Tip = "tip-commit"
	tx(t, db, func(tx *Tx) error { return tx.SaveSession(want) })
	if got := read(); got != want {
		t.Fatalf("session with a tip = %+v, want %+v", got, want)
	}
}

func TestOpenKeepsModelEntriesWhenConfigIsInvalid(t *testing.T) {
	configPath := writeConfig(t, `{"theme":"blue"}`)
	createCurrentDatabase(t, `INSERT INTO model_entries VALUES ('b', 'claude', 'opus', 'Opus', '[]', '');`)
	if db, err := Open(); err == nil {
		db.Close()
		t.Fatal("opened with an invalid config.json")
	} else if !strings.Contains(err.Error(), configPath) {
		t.Fatalf("error does not name config.json: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	db := reopen(t)
	if hasModelEntriesTable(t, db) {
		t.Fatal("model_entries remains after opening")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if want := []config.ModelEntry{{Harness: "claude", Model: "opus", Title: "Opus", Efforts: []string{}}}; !reflect.DeepEqual(cfg.Models, want) {
		t.Fatalf("moved models = %+v", cfg.Models)
	}
}
