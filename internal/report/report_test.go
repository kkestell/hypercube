package report

import (
	"path/filepath"
	"strings"
	"testing"

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
	db := open(t, store.Session{ID: "p", Title: "Build", Dir: "/repo"}, store.Session{ID: "q", Title: "Fix"}, store.Session{ID: "c", Parent: "p", Name: "builder", Title: "Build it", Harness: "test", Model: "m", Dir: "/repo/child", Branch: "hypercube/p/builder", Ended: true})
	if err := db.Read(func(tx *store.Tx) error {
		p, _ := tx.Session("p")
		child, _ := tx.Session("c")
		parentText, err := Instructions(tx, p, []string{"test", "other"}, "Use m for design.")
		if err != nil {
			return err
		}
		childText, err := Instructions(tx, child, nil, "Use m for design.")
		if err != nil {
			return err
		}
		for _, want := range []string{"Hypercube session p", "Children:", "builder (Build it): test m, default effort", "set_title", "ended", "across all workspaces in this app", "list_sessions", "workspace paths and parent IDs", ChildSettings, "Installed harnesses: test, other.", "guidance for choosing child models:\nUse m for design.", "create_session", "end_session"} {
			if !strings.Contains(parentText, want) {
				t.Errorf("parent missing %q: %s", want, parentText)
			}
		}
		for _, want := range []string{"agent of child session builder", "created by Hypercube session p", `titled "Build"`, "send_message to p", "Your parent merges it"} {
			if !strings.Contains(childText, want) {
				t.Errorf("child missing %q: %s", want, childText)
			}
		}
		if strings.Contains(childText, "Use m for design") || strings.Contains(parentText, "profile") {
			t.Error("a child got the guidance, or a parent was told about profiles")
		}
		if strings.Contains(parentText, "q (Fix)") {
			t.Error("instructions include a stale peer snapshot:", parentText)
		}
		if !strings.Contains(parentText, "share your checkout") || !strings.Contains(childText, "the only session you can message") || strings.Contains(childText, "list_sessions") {
			t.Error("instructions changed workspace sharing or child scope")
		}
		child.Branch = ""
		text, err := Instructions(tx, child, nil, "")
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

func TestWorktreeSessionInstructions(t *testing.T) {
	db := open(t, store.Session{ID: "p", Title: "Build", Dir: "/repo/.hypercube/p", Branch: "hypercube/session-p", Base: "abc"})
	if err := db.Read(func(tx *store.Tx) error {
		p, _ := tx.Session("p")
		text, err := Instructions(tx, p, nil, "")
		if err != nil {
			return err
		}
		for _, want := range []string{"your own worktree on branch hypercube/session-p", "original checkout at /repo", "child sessions' branches merge into your branch", "when the user asks you to merge"} {
			if !strings.Contains(text, want) {
				t.Errorf("missing %q: %s", want, text)
			}
		}
		if strings.Contains(text, "share your checkout") || strings.Contains(text, "You work in the original checkout") {
			t.Error("a worktree session is told it shares the checkout:", text)
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
		if got := Format(tx, store.Message{From: "gone", Text: "hi"}); !strings.HasPrefix(got, "Message from gone:\nhi\n\n") {
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
