package store

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
)

func TestOpenUpgradesDatabaseBeforeDurableLaunches(t *testing.T) {
	db := open(t)
	seed(t, db, "w", "/repo", "s", "child")
	tx(t, db, func(tx *Tx) error {
		s, err := tx.Session("s")
		if err != nil {
			return err
		}
		s.Title, s.Conversation = "Saved session", "conversation"
		if err := tx.SaveSession(s); err != nil {
			return err
		}
		if _, err := tx.PutEntry("s", Entry{Seq: 1, Item: "reply", Data: []byte(`{"kind":"text","text":"Saved reply"}`)}); err != nil {
			return err
		}
		return tx.Queue(Message{From: "s-child", To: "s", Text: "Saved message"})
	})
	// Recreate the schema of the previous version, keeping its saved data.
	if _, err := db.sql.Exec(`
DROP TABLE commands;
ALTER TABLE sessions DROP COLUMN launch;
ALTER TABLE sessions DROP COLUMN position;
ALTER TABLE sessions DROP COLUMN ready;
ALTER TABLE sessions DROP COLUMN quick;`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		again, err := Open()
		if err != nil {
			t.Fatal(err)
		}
		err = again.Read(func(tx *Tx) error {
			sessions, err := tx.Sessions("w")
			if err != nil {
				return err
			}
			if len(sessions) != 1 {
				t.Fatalf("sessions after upgrade = %+v", sessions)
			}
			s := sessions[0]
			if s.ID != "s" || s.Title != "Saved session" || s.Conversation != "conversation" || !s.Running || s.Launch != "" || s.Position != 0 || s.Ready != 0 || s.Quick != 0 {
				t.Errorf("session after upgrade = %+v", s)
			}
			children, err := tx.Children("s")
			if err != nil {
				return err
			}
			if len(children) != 1 || children[0].ID != "s-child" {
				t.Errorf("children after upgrade = %+v", children)
			}
			entries, err := tx.Transcript("s")
			if err != nil {
				return err
			}
			if len(entries) != 1 || entries[0].Item != "reply" {
				t.Errorf("transcript after upgrade = %+v", entries)
			}
			messages, err := tx.Queued("s")
			if err != nil {
				return err
			}
			if len(messages) != 1 || messages[0].Text != "Saved message" {
				t.Errorf("messages after upgrade = %+v", messages)
			}
			_, err = tx.Commands("s")
			return err
		})
		if closeErr := again.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

// legacyData is a database written by the version that kept a session's
// agents in an agents table. Session s1 has a lead and two workers; s2 has a
// lead and one worker in a folder that is not a Git checkout. The queued and
// transcript rows cover messages between leads, between a lead and its
// workers, and from a sender the database does not know.
const legacyData = `
INSERT INTO workspaces VALUES ('w1', '/repo', 0, 10), ('w2', '/plain', 1, 20);
INSERT INTO sessions VALUES ('s1', 'w1', 'Fix parser', 1, 1, 1, 100, 900), ('s2', 'w2', 'Other', 0, 0, 0, 200, 300);
INSERT INTO agents VALUES
 ('l1', 's1', 'lead', 'lead', 'claude', 'opus', 'high', '/repo', '', 'conv-l1', 'idle', 1, 'thinking', 5, 10, 200, 1.5, 101),
 ('wk1', 's1', 'scribe', 'worker', 'codex', 'gpt', 'xhigh', '/repo/.worktrees/s1-scribe', 'hypercube/s1/scribe', 'conv-wk1', 'busy', 0, '', 0, 20, 400, 0.25, 110),
 ('wk2', 's1', 'direct', 'worker', 'pi', 'free', 'low', '/repo/pkg', '', '', 'exited', 0, '', 0, 0, 0, 0, 120),
 ('l2', 's2', 'lead', 'lead', 'opencode', 'deep', 'max', '/plain', '', 'conv-l2', 'exited', 0, '', 0, 7, 70, 0.07, 201),
 ('wk3', 's2', 'helper', 'worker', 'claude', 'haiku', 'medium', '/plain/sub', '', 'conv-wk3', 'idle', 0, '', 0, 3, 30, 0.03, 210);
INSERT INTO transcript VALUES
 ('l1', 1, 'a', '{"kind":"user","text":"Hello","images":[{"mime":"image/png","data":"AAAA"}],"done":true}'),
 ('l1', 2, 'b', '{"kind":"user","from":"s2/lead","text":"From another lead","images":[{"mime":"image/png","data":"BBBB"}]}'),
 ('l1', 3, 'c', '{"kind":"user","from":"s1/scribe","text":"From a worker"}'),
 ('l1', 4, 'd', '{"kind":"user","from":"gone/lead","text":"From a deleted session"}'),
 ('wk1', 1, 'e', '{"kind":"user","from":"s1/lead","text":"Do the work"}'),
 ('wk2', 1, 'f', '{"kind":"text","text":"Done"}'),
 ('l2', 1, 'a', '{"kind":"text","text":"Same item ID and position as another stream"}');
INSERT INTO messages VALUES
 (7, 'l2', 'l1', 'Lead to lead'),
 (9, 'wk1', 'l1', 'Worker to lead'),
 (11, 'someone', 'wk2', 'Unknown sender'),
 (12, 'l1', 'wk1', 'Lead to worker');`

func usage(context, window int, cost float64) harness.Usage {
	return harness.Usage{Context: context, Window: window, Cost: cost}
}

// writeLegacy creates a database from the previous version's schema, with the
// rows in data. The fixture is the schema from origin commit 38f11db, before
// leads and workers became nested sessions.
func writeLegacy(t *testing.T, data string) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	ddl, err := os.ReadFile("testdata/origin-schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", dbPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(string(ddl) + data); err != nil {
		t.Fatal(err)
	}
	raw.Close()
}

func openLegacy(t *testing.T, data string) *DB {
	t.Helper()
	writeLegacy(t, data)
	return reopen(t)
}

func reopen(t *testing.T) *DB {
	t.Helper()
	db, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func jsonEqual(t *testing.T, got []byte, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("item = %s, want %s", got, want)
	}
}

func TestOpenUpgradesAgentsDatabase(t *testing.T) {
	db := openLegacy(t, legacyData)
	check := func(db *DB) {
		t.Helper()
		err := db.Read(func(tx *Tx) error {
			roots, err := tx.Sessions("")
			if err != nil {
				return err
			}
			want := []Session{
				{ID: "s1", Workspace: "w1", Title: "Fix parser", Running: true, AwaitingUser: true, Unseen: true,
					Harness: "claude", Model: "opus", Effort: "high", Dir: "/repo", Conversation: "conv-l1",
					Activity: Idle, Attention: true, Usage: usage(10, 200, 1.5), Created: 100, LastActive: 900},
				{ID: "s2", Workspace: "w2", Title: "Other", Harness: "opencode", Model: "deep", Effort: "max",
					Dir: "/plain", Conversation: "conv-l2", Activity: Exited, Usage: usage(7, 70, 0.07), Created: 200, LastActive: 300},
			}
			if !reflect.DeepEqual(roots, want) {
				t.Errorf("sessions = %+v\nwant %+v", roots, want)
			}
			children, err := tx.Children("s1")
			if err != nil {
				return err
			}
			wantChildren := []Session{
				{ID: "wk1", Workspace: "w1", Parent: "s1", Name: "scribe", Title: "scribe", Harness: "codex", Model: "gpt", Effort: "xhigh",
					Dir: "/repo/.worktrees/s1-scribe", Branch: "hypercube/s1/scribe", Conversation: "conv-wk1", Activity: Busy,
					Usage: usage(20, 400, 0.25), Created: 110, LastActive: 110},
				{ID: "wk2", Workspace: "w1", Parent: "s1", Name: "direct", Title: "direct", Harness: "pi", Model: "free", Effort: "low",
					Dir: "/repo/pkg", Activity: Exited, Created: 120, LastActive: 120},
			}
			if !reflect.DeepEqual(children, wantChildren) {
				t.Errorf("children of s1 = %+v\nwant %+v", children, wantChildren)
			}
			if children, err = tx.Children("s2"); err != nil || len(children) != 1 || children[0].ID != "wk3" || children[0].Dir != "/plain/sub" {
				t.Errorf("children of s2 = %+v, %v", children, err)
			}

			// Each agent's transcript follows it, and a sender's session/name
			// address becomes its session ID.
			wantItems := map[string][]string{
				"s1": {
					`{"kind":"user","text":"Hello","images":[{"mime":"image/png","data":"AAAA"}],"done":true}`,
					`{"kind":"user","from":"s2","text":"From another lead","images":[{"mime":"image/png","data":"BBBB"}]}`,
					`{"kind":"user","from":"wk1","text":"From a worker"}`,
					`{"kind":"user","from":"gone/lead","text":"From a deleted session"}`,
				},
				"wk1": {`{"kind":"user","from":"s1","text":"Do the work"}`},
				"wk2": {`{"kind":"text","text":"Done"}`},
				"s2":  {`{"kind":"text","text":"Same item ID and position as another stream"}`},
			}
			for session, items := range wantItems {
				entries, err := tx.Transcript(session)
				if err != nil {
					return err
				}
				if len(entries) != len(items) {
					t.Errorf("transcript of %s has %d items, want %d", session, len(entries), len(items))
					continue
				}
				for i, e := range entries {
					if e.Seq != int64(i+1) {
						t.Errorf("transcript of %s item %d has seq %d", session, i, e.Seq)
					}
					jsonEqual(t, e.Data, items[i])
				}
			}

			wantQueued := map[string][]Message{
				"s1":  {{ID: 7, From: "s2", To: "s1", Text: "Lead to lead"}, {ID: 9, From: "wk1", To: "s1", Text: "Worker to lead"}},
				"wk1": {{ID: 12, From: "s1", To: "wk1", Text: "Lead to worker"}},
				"wk2": {{ID: 11, From: "someone", To: "wk2", Text: "Unknown sender"}},
			}
			for session, messages := range wantQueued {
				if got, err := tx.Queued(session); err != nil || !reflect.DeepEqual(got, messages) {
					t.Errorf("queued for %s = %+v, %v\nwant %+v", session, got, err, messages)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	check(db)

	// The old tables are gone, and the new ones accept writes that cascade.
	var tables []string
	err := db.Read(func(tx *Tx) error {
		rows, err := tx.tx.Query(`SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err
			}
			tables = append(tables, name)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"agents", "old_sessions", "old_messages", "old_transcript"} {
		if slices.Contains(tables, name) {
			t.Errorf("table %s remains after the upgrade", name)
		}
	}
	tx(t, db, func(tx *Tx) error {
		if err := tx.AddSession(Session{ID: "wk4", Workspace: "w1", Parent: "s1", Name: "fresh"}); err != nil {
			return err
		}
		if _, err := tx.PutEntry("wk4", Entry{Seq: 1, Item: "a", Data: []byte(`{"kind":"text"}`)}); err != nil {
			return err
		}
		if err := tx.Queue(Message{From: "s1", To: "wk4", Text: "New"}); err != nil {
			return err
		}
		if _, err := tx.AddCommand(Command{Session: "wk4", Command: "send"}); err != nil {
			return err
		}
		return tx.DeleteSession("s1")
	})
	err = db.Read(func(tx *Tx) error {
		var rows int
		for _, query := range []string{
			`SELECT COUNT(*) FROM sessions WHERE id IN ('s1', 'wk1', 'wk2', 'wk4')`,
			`SELECT COUNT(*) FROM transcript WHERE session IN ('s1', 'wk1', 'wk2', 'wk4')`,
			`SELECT COUNT(*) FROM messages WHERE recipient IN ('s1', 'wk1', 'wk2', 'wk4')`,
			`SELECT COUNT(*) FROM commands`,
		} {
			if err := tx.tx.QueryRow(query).Scan(&rows); err != nil || rows != 0 {
				t.Errorf("%s = %d, %v after deleting s1", query, rows, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOpenAgentsUpgradeRunsOnce(t *testing.T) {
	db := openLegacy(t, legacyData)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	again := reopen(t)
	err := again.Read(func(tx *Tx) error {
		roots, err := tx.Sessions("")
		if err != nil || len(roots) != 2 {
			t.Errorf("sessions after reopening = %+v, %v", roots, err)
		}
		entries, err := tx.Transcript("s1")
		if err != nil || len(entries) != 4 {
			t.Errorf("transcript after reopening = %+v, %v", entries, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOpenRefusesAgentsDatabaseItCannotConvert(t *testing.T) {
	// s2 has no lead agent.
	writeLegacy(t, strings.Replace(legacyData, ` ('l2', 's2', 'lead', 'lead', 'opencode'`, ` ('l2', 's1', 'extra', 'worker', 'opencode'`, 1))
	if _, err := Open(); err == nil || !strings.Contains(err.Error(), "session s2 does not have exactly one lead agent") {
		t.Fatalf("Open error = %v", err)
	}
	raw, err := sql.Open("sqlite", dbPath())
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var agents int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM agents`).Scan(&agents); err != nil || agents != 5 {
		t.Errorf("agents after refused upgrade = %d, %v", agents, err)
	}
}
