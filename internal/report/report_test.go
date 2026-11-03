package report

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func open(t *testing.T, sessions ...store.Session) *store.DB {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	db, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Tx(func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "w", Path: "/repo"}); err != nil {
			return err
		}
		for _, s := range sessions {
			s.Workspace = "w"
			if err := tx.AddSession(s); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestInstructions(t *testing.T) {
	db := open(t, store.Session{ID: "p", Title: "Build", Dir: "/repo"}, store.Session{ID: "q", Title: "Fix"}, store.Session{ID: "c", Parent: "p", Name: "builder", Title: "Build it", Harness: "test", Dir: "/repo/child", Branch: "hypercube/p/builder", Ended: true})
	profiles := []harness.Profile{{Name: "planner", Description: "design", Harness: "test", Model: "m", Effort: "low"}}
	if err := db.Read(func(tx *store.Tx) error {
		p, _ := tx.Session("p")
		child, _ := tx.Session("c")
		parentText, err := Instructions(tx, p, profiles)
		if err != nil {
			return err
		}
		childText, err := Instructions(tx, child, nil)
		if err != nil {
			return err
		}
		for _, want := range []string{"Hypercube session p", "Children:", "builder (Build it): test", "set_title", "ended", "across all workspaces in this app", "list_sessions", "workspace paths and parent IDs", "planner (test, m, low): design", "create_session", "end_session"} {
			if !strings.Contains(parentText, want) {
				t.Errorf("parent missing %q: %s", want, parentText)
			}
		}
		for _, want := range []string{"child session of Hypercube session p", `titled "Build"`, "send_message to p", "Your parent merges it", "set_status", "not your parent's agent"} {
			if !strings.Contains(childText, want) {
				t.Errorf("child missing %q: %s", want, childText)
			}
		}
		if strings.Contains(parentText, "Other top-level sessions:") || strings.Contains(parentText, "q (Fix)") {
			t.Error("instructions include a stale peer snapshot:", parentText)
		}
		if !strings.Contains(parentText, "top-level sessions in this workspace share") || !strings.Contains(childText, "the only session you can message") || strings.Contains(childText, "list_sessions") {
			t.Error("instructions changed workspace sharing or child scope")
		}
		child.Branch = ""
		text, err := Instructions(tx, child, nil)
		if err != nil {
			return err
		}
		if !strings.Contains(text, "Handle branches and commits as your parent tells you") {
			t.Error(text)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestFormatAttributesMessages(t *testing.T) {
	db := open(t, store.Session{ID: "p"}, store.Session{ID: "q", Title: "Fix parser"}, store.Session{ID: "c", Parent: "p", Name: "builder"})
	if err := db.Read(func(tx *store.Tx) error {
		for _, c := range []struct{ from, to, address string }{{"c", "p", "builder"}, {"p", "c", "p"}} {
			got := Format(tx, store.Message{From: c.from, To: c.to, Text: "hello"})
			if !strings.HasPrefix(got, "Message from "+c.address+":\nhello") || !strings.HasSuffix(got, "send_message tool to "+c.address+".") {
				t.Error(got)
			}
		}
		got := Format(tx, store.Message{From: "q", To: "p", Text: "hello"})
		if !strings.HasPrefix(got, "Message from q (\"Fix parser\", workspace /repo):\nhello") || !strings.HasSuffix(got, "send_message tool to q.") {
			t.Error(got)
		}
		if got := Format(tx, store.Message{From: "gone", Text: "hi"}); got != "Message from gone:\nhi" {
			t.Error(got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestFailureMessage(t *testing.T) {
	if FailureMessage(" rate limited ") != "My turn failed: rate limited" || FailureMessage(" ") != "My turn failed." || !strings.HasSuffix(FailureMessage(strings.Repeat("é", 2500)), "…") {
		t.Error("failure text")
	}
}

func TestTitle(t *testing.T) {
	long := strings.Repeat("a", 250)
	for _, c := range []struct{ name, text, want string }{
		{"first line", "Fix the parser\nand the lexer", "Fix the parser"},
		{"blank lines and spacing", "\n  \n  Fix \t the   parser  \nmore", "Fix the parser"},
		{"long line", long, strings.Repeat("a", 199) + "…"},
		{"exactly 200", long[:200], long[:200]},
		{"non-ASCII", strings.Repeat("é", 250), strings.Repeat("é", 199) + "…"},
		{"only whitespace", " \n\t ", "Image"},
		{"no text", "", "Image"},
	} {
		if got := Title(c.text); got != c.want {
			t.Errorf("%s: title = %q, want %q", c.name, got, c.want)
		}
	}
}
