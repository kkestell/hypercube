package session

import (
	"slices"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

// approve asks the session's agent to request approval and waits for the
// request, which it returns.
func (f *fixture) approve(id string) string {
	f.t.Helper()
	before := f.openApprovals(id)
	f.input(id, "!approve")
	f.until("an approval request", func() bool { return len(f.openApprovals(id)) == len(before)+1 })
	for _, r := range f.openApprovals(id) {
		known := false
		for _, b := range before {
			known = known || b == r
		}
		if !known {
			return r
		}
	}
	panic("unreachable")
}

// openApprovals lists the IDs of the session's open approval requests.
func (f *fixture) openApprovals(id string) (ids []string) {
	for _, r := range f.requests() {
		if r.Session == id && r.Item.Grant != "" {
			ids = append(ids, r.Item.ID)
		}
	}
	return ids
}

// answersOf counts the answers the agent's plugin has received.
func (f *fixture) answersOf(id string) int { return len(f.log(id, "answer")) }

func TestAlwaysAllowInSession(t *testing.T) {
	f := newPlainFixture(t)
	a, b := f.session(), f.session()
	first, _ := f.approve(a.ID), f.approve(a.ID)
	f.approve(b.ID)
	if err := f.rt.AlwaysAllow(a.ID, first, false); err != nil {
		t.Fatal(err)
	}
	// One grant answers every open request for the tool in the session, and
	// leaves the other session asking.
	f.until("both requests answered", func() bool { return f.finishedTools(a.ID) == 2 })
	allowed := func() {
		t.Helper()
		for _, e := range f.log(a.ID, "answer") {
			if !slices.Equal(e.Choices, []string{"Allow"}) {
				t.Errorf("answer %+v, want the choices [Allow]", e)
			}
		}
	}
	allowed()
	if len(f.openApprovals(b.ID)) != 1 || f.answersOf(b.ID) != 0 {
		t.Fatalf("the other session was answered: %v", f.log(b.ID, "answer"))
	}

	// A later request is answered without being shown.
	changes, cancel := f.rt.Subscribe()
	defer cancel()
	f.input(a.ID, "!approve")
	f.until("the request answered", func() bool { return f.answersOf(a.ID) == 3 })
	f.until("the tool finished", func() bool { return f.finishedTools(a.ID) == 3 })
	allowed()
	if f.storedSession(a.ID).Attention || len(f.openApprovals(a.ID)) != 0 {
		t.Fatal("the automatic request asked for attention")
	}
	if got := f.approvalItems(a.ID); got != 2 {
		t.Fatalf("%d approvals in the transcript, want the 2 that were answered by hand", got)
	}
	for len(changes) > 0 {
		if c := <-changes; c.Item.Kind == harness.KindAttention {
			t.Fatalf("published %+v", c.Item)
		}
	}

	// The grant lasts across restarts.
	f.restart()
	f.until("the session running", func() bool { return f.idle(a.ID) })
	f.input(a.ID, "!approve")
	f.until("the request answered", func() bool { return f.finishedTools(a.ID) == 4 })
	if len(f.openApprovals(a.ID)) != 0 {
		t.Fatal("the request was shown after a restart")
	}
}

func TestAlwaysAllowInAllSessions(t *testing.T) {
	f := newPlainFixture(t)
	a, b := f.session(), f.session()
	request := f.approve(a.ID)
	f.approve(b.ID)
	if err := f.rt.AlwaysAllow(a.ID, request, true); err != nil {
		t.Fatal(err)
	}
	f.until("both sessions answered", func() bool { return f.answersOf(a.ID) == 1 && f.answersOf(b.ID) == 1 })
	f.input(b.ID, "!approve")
	f.until("the request answered", func() bool { return f.answersOf(b.ID) == 2 })
	if len(f.requests()) != 0 {
		t.Fatalf("requests = %+v", f.requests())
	}
	if err := f.db.Tx(func(tx *store.Tx) error { return tx.DeleteGrant("", "test", "Tool") }); err != nil {
		t.Fatal(err)
	}
	f.approve(b.ID)
}

func TestAlwaysAllowNeedsAnOpenApproval(t *testing.T) {
	f := newPlainFixture(t)
	s := f.session()
	f.input(s.ID, "!ask")
	f.until("a question", func() bool { return len(f.requests()) == 1 })
	if err := f.rt.AlwaysAllow(s.ID, f.requests()[0].Item.ID, true); err == nil {
		t.Error("a question was always allowed")
	}
	request := f.approve(s.ID)
	if err := f.rt.Answer(s.ID, request, []string{"Deny"}, ""); err != nil {
		t.Fatal(err)
	}
	f.until("the answer", func() bool { return len(f.openApprovals(s.ID)) == 0 })
	if err := f.rt.AlwaysAllow(s.ID, request, false); err == nil {
		t.Error("a finished request was always allowed")
	}
}

// finishedTools counts the session's finished tool items.
func (f *fixture) finishedTools(id string) (n int) {
	for _, it := range f.transcript(id) {
		if it.Kind == harness.KindTool && it.Done {
			n++
		}
	}
	return n
}

// approvalItems counts the approval requests in the session's transcript.
func (f *fixture) approvalItems(id string) (n int) {
	for _, it := range f.transcript(id) {
		if it.Grant != "" {
			n++
		}
	}
	return n
}
