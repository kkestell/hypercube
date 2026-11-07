package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
)

func TestStreamFailureEndsTheOwnedServer(t *testing.T) {
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"server.connected\",\"data\":{}}\n\n")
		w.(http.Flusher).Flush()
	}))
	defer native.Close()
	fakeExecutable(t, native.URL)
	errEvent := make(chan string, 1)
	ag := newAgent(func(e harness.Event) {
		for _, it := range e.Items {
			if it.Kind == harness.KindError {
				errEvent <- it.Text
			}
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var err error
	ag.server, err = launch(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer ag.stop()
	_ = ag.subscribe(ctx)
	select {
	case text := <-errEvent:
		if !strings.Contains(text, "event stream") {
			t.Fatal(text)
		}
	case <-ctx.Done():
		t.Fatal("lost stream failure")
	}
	select {
	case <-ag.server.proc.Done():
	case <-ctx.Done():
		t.Fatal("owned server kept running without its transcript connection")
	}
}

func TestCatalogWaitsForPluginActivation(t *testing.T) {
	h := connected(t)
	var reads int
	h.reply = func(w http.ResponseWriter, r *http.Request, _ map[string]any) bool {
		if r.URL.Path != "/api/plugin" {
			return false
		}
		reads++
		if reads == 1 {
			fmt.Fprint(w, `{"data":[]}`)
			h.wire <- map[string]any{"type": "plugin.updated", "data": map[string]any{}}
			return true
		}
		return false
	}
	ctx, cancel := context.WithTimeout(h.ctx, 5*time.Second)
	defer cancel()
	if err := h.begin(ctx, harness.StartArgs{Dir: "/work", Model: "p/m"}); err != nil {
		t.Fatal(err)
	}
	if len(h.sent("/api/plugin")) != 2 {
		t.Fatal("did not wait for native plugin activation")
	}
}

func TestSlashCommandsKeepNativeSteerDelivery(t *testing.T) {
	h := connected(t)
	for _, text := range []string{"/compact", "/review these changes", "/unknown text"} {
		if err := h.input(text, nil); err != nil {
			t.Fatal(err)
		}
	}
	if c := h.sent("/api/session/s1/compact"); len(c) != 1 || c[0].Body["delivery"] != "steer" {
		t.Fatal(c)
	}
	if c := h.sent("/api/session/s1/command"); len(c) != 1 || c[0].Body["delivery"] != "steer" || c[0].Body["name"] != "review" || c[0].Body["text"] != "these changes" {
		t.Fatal(c)
	}
	if c := h.sent("/api/session/s1/prompt"); len(c) != 1 || c[0].Body["text"] != "/unknown text" {
		t.Fatal(c)
	}
}
