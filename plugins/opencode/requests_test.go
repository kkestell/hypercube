package main

import (
	"net/http"
	"testing"
	"time"
)

func TestNativeChildQuestionCanBeAnswered(t *testing.T) {
	h := connected(t)
	h.feed(t, "form.created", map[string]any{"form": map[string]any{"id": "child-form", "sessionID": "child", "fields": []any{map[string]any{"key": "q", "type": "string", "title": "Question?"}}}})
	if err := h.answer("form:child-form/0", nil, "answer"); err != nil {
		t.Fatal(err)
	}
	if len(h.sent("/api/session/child/form/child-form/reply")) != 1 {
		t.Fatal("answer went to the wrong session")
	}
	if len(h.states()) > 0 {
		t.Fatal("child question changed main execution state")
	}
}

func TestAnswerCanReturnAfterNativeCompletion(t *testing.T) {
	h := connected(t)
	h.feed(t, "form.created", map[string]any{"form": map[string]any{"id": "f", "sessionID": "s1", "fields": []any{map[string]any{"key": "q", "type": "string"}}}})
	called, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	h.reply = func(w http.ResponseWriter, r *http.Request, _ map[string]any) bool {
		if r.URL.Path != "/api/session/s1/form/f/reply" {
			return false
		}
		close(called)
		<-release
		w.WriteHeader(204)
		return true
	}
	done := make(chan error, 1)
	go func() { done <- h.answer("form:f/0", nil, "answer") }()
	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("answer was not sent")
	}
	h.feed(t, "form.replied", map[string]any{"id": "f"})
	h.feed(t, "session.execution.succeeded", nil)
	if h.item("form:f/0").Done {
		t.Fatal("completion discarded the in-flight answer")
	}
	// Use a token to release the handler without closing the cleanup channel.
	release <- struct{}{}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if it := h.item("form:f/0"); !it.Done || it.Answer != "answer" {
		t.Fatal(it)
	}
}
