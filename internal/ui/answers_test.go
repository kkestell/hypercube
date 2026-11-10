package ui

import (
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/harness/harnesstest"
)

func TestQuestionSelectionsReachPlugin(t *testing.T) {
	t.Parallel()
	a := newApp(t)
	p := a.page()
	p.send(`{"select":"` + a.session.ID + `"}`)
	for _, multi := range []bool{false, true} {
		prompt := "!ask"
		if multi {
			prompt = "!multiask"
		}
		a.input(prompt, http.StatusOK)
		var item harness.Item
		p.until("the question", func() bool {
			var found bool
			item, found = p.find(a.session.ID, func(it harness.Item) bool { return it.Kind == harness.KindAttention && !it.Done })
			return found
		})
		if item.Multi != multi {
			t.Fatalf("question = %+v", item)
		}
		answer := func(choices []string, status int) {
			t.Helper()
			q := url.Values{"action": {"answer"}, "id": {a.session.ID}, "request": {item.ID}, "choice": choices}
			a.post("/session?"+q.Encode(), "", status)
		}
		for _, choices := range [][]string{{"unknown"}, {"yes", "yes"}, nil} {
			answer(choices, http.StatusConflict)
		}
		choices := []string{"yes"}
		if multi {
			choices = append(choices, "with, punctuation")
		} else {
			answer([]string{"yes", "no"}, http.StatusConflict)
		}
		answer(choices, http.StatusOK)
		p.until("the answered question", func() bool {
			it, found := p.find(a.session.ID, func(it harness.Item) bool { return it.ID == item.ID })
			return found && it.Done
		})
		entries := harnesstest.Log(t, a.logs, a.session.ID, "answer")
		if !slices.Equal(entries[len(entries)-1].Choices, choices) {
			t.Fatalf("plugin answers = %+v", entries)
		}
	}
}
