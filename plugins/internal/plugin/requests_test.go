package plugin

import (
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
)

func items(ids ...string) []harness.Item {
	var out []harness.Item
	for _, id := range ids {
		out = append(out, harness.Item{ID: id, Kind: harness.KindAttention})
	}
	return out
}

func TestRequestsAnswerItemsInAnyOrder(t *testing.T) {
	var q Requests[string]
	name := "first"
	q.Add(&name, items("a", "b"))

	o, ok := q.Get("b")
	if !ok || o.Request != &name || o.Index != 1 || o.Last {
		t.Fatalf("Get(b) = %+v, %v", o, ok)
	}
	done := q.Done("b", "yes")
	if !done.Done || done.Answer != "yes" || done.ID != "b" {
		t.Fatalf("Done(b) = %+v", done)
	}
	if _, ok := q.Get("b"); ok {
		t.Fatal("b is still open after it was answered")
	}
	o, ok = q.Get("a")
	if !ok || o.Index != 0 || !o.Last {
		t.Fatalf("Get(a) = %+v, %v", o, ok)
	}
	q.Done("a", "no")
	if q.Find(func(*string) bool { return true }) != nil {
		t.Fatal("a request with every item answered is still tracked")
	}
}

func TestRequestsCloseFinishesOnlyOpenItemsOfMatches(t *testing.T) {
	var q Requests[string]
	first, second := "first", "second"
	q.Add(&first, items("a", "b"))
	q.Add(&second, items("c"))
	q.Done("a", "x")

	got, closed := q.Close(func(r *string) bool { return r == &first })
	if len(got) != 1 || got[0].ID != "b" || !got[0].Done || got[0].Answer != "" {
		t.Fatalf("closed items = %+v", got)
	}
	if len(closed) != 1 || closed[0] != &first {
		t.Fatalf("closed requests = %v", closed)
	}
	if _, ok := q.Get("b"); ok {
		t.Fatal("b is still open after its request closed")
	}
	if _, ok := q.Get("c"); !ok {
		t.Fatal("a request that did not match was closed")
	}
	if r := q.Find(func(r *string) bool { return *r == "second" }); r != &second {
		t.Fatalf("Find = %v", r)
	}
	got, closed = q.Close(func(*string) bool { return true })
	if len(got) != 1 || got[0].ID != "c" || len(closed) != 1 {
		t.Fatalf("closing the rest = %+v, %v", got, closed)
	}
}
