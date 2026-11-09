package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
)

func TestOpenCurrentDatabasePreservesState(t *testing.T) {
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
	_, err = raw.Exec(string(ddl))
	closeErr := raw.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("creating current database: %v, close: %v", err, closeErr)
	}
	db := reopen(t)
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
	model := ModelEntry{Harness: "opencode", Model: "provider/model", Title: "My model",
		Efforts: []string{"low", "high"}, DefaultEffort: "high"}
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
		if err := tx.SaveModelEntry(model); err != nil {
			return err
		}
		models, err := tx.ModelEntries()
		if err != nil {
			return err
		}
		if len(models) != 1 || models[0].ID == "" {
			t.Fatalf("saved models = %+v", models)
		}
		model.ID = models[0].ID
		return nil
	})
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	again := reopen(t)
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
		models, err := tx.ModelEntries()
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(models, []ModelEntry{model}) {
			t.Errorf("models = %+v, want %+v", models, model)
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
