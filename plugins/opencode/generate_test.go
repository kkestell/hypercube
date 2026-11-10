package main

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
)

func TestGenerateSendsThePromptToTheChosenModel(t *testing.T) {
	h := connected(t)
	log := owned(t, h)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	text, err := generate(ctx, harness.GenerateRequest{Model: "p/m", Effort: "low", Prompt: "Name this"})
	if err != nil {
		t.Fatal(err)
	}
	if text != "Fix the parser" {
		t.Fatalf("text: %q", text)
	}
	calls := h.sent("/api/experimental/generate/text")
	if len(calls) != 1 {
		t.Fatalf("generate calls: %+v", calls)
	}
	model, _ := calls[0].Body["model"].(map[string]any)
	if calls[0].Body["prompt"] != "Name this" || model["providerID"] != "p" || model["id"] != "m" || model["variant"] != "low" {
		t.Fatalf("body: %+v", calls[0].Body)
	}
	if len(h.sent("/api/session")) != 0 {
		t.Fatal("a session was created")
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	if lines[0] != "serve --stdio --port 0" || lines[2] != "" || !strings.Contains(lines[3], "hypercube-utility-") {
		t.Fatalf("launch: %q", lines)
	}
	if _, err := os.Stat(lines[3]); !os.IsNotExist(err) {
		t.Fatalf("temporary directory remains: %v", err)
	}
}

func TestGenerateKeepsSlashesInTheModelID(t *testing.T) {
	h := connected(t)
	owned(t, h)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := generate(ctx, harness.GenerateRequest{Model: "q/b/c", Prompt: "Name this"}); err != nil {
		t.Fatal(err)
	}
	model, _ := h.sent("/api/experimental/generate/text")[0].Body["model"].(map[string]any)
	if model["providerID"] != "q" || model["id"] != "b/c" {
		t.Fatalf("model: %+v", model)
	}
	if _, ok := model["variant"]; ok {
		t.Fatalf("variant sent: %+v", model)
	}
}

func TestGenerateRejectsASelectionTheServerDoesNotOffer(t *testing.T) {
	for name, tt := range map[string]struct {
		req  harness.GenerateRequest
		want string
	}{
		"effort":  {harness.GenerateRequest{Model: "p/m", Effort: "minimal"}, `does not offer effort "minimal"`},
		"missing": {harness.GenerateRequest{Model: "p/missing"}, `does not offer model "p/missing"`},
		"none":    {harness.GenerateRequest{Model: "p/m"}, "needs an effort"},
	} {
		t.Run(name, func(t *testing.T) {
			h := connected(t)
			owned(t, h)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			tt.req.Prompt = "Name this"
			if _, err := generate(ctx, tt.req); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error: %v", err)
			}
			if len(h.sent("/api/experimental/generate/text")) != 0 {
				t.Fatal("the prompt was sent")
			}
		})
	}
}

func TestGenerateReportsTheServersMessage(t *testing.T) {
	h := connected(t)
	owned(t, h)
	h.reply = func(w http.ResponseWriter, r *http.Request, _ map[string]any) bool {
		if r.URL.Path != "/api/experimental/generate/text" {
			return false
		}
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"_tag":"InvalidRequestError","message":"Variant unavailable for p/m: low"}`))
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := generate(ctx, harness.GenerateRequest{Model: "p/m", Effort: "low", Prompt: "Name this"})
	if err == nil || !strings.Contains(err.Error(), "Variant unavailable for p/m: low") {
		t.Fatalf("error: %v", err)
	}
}

func TestGenerateStopsWhenCancelled(t *testing.T) {
	h := connected(t)
	log := owned(t, h)
	arrived := make(chan struct{})
	h.reply = func(w http.ResponseWriter, r *http.Request, _ map[string]any) bool {
		if r.URL.Path != "/api/experimental/generate/text" {
			return false
		}
		close(arrived)
		<-r.Context().Done()
		return true
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := generate(ctx, harness.GenerateRequest{Model: "p/m", Effort: "low", Prompt: "Name this"})
		done <- err
	}()
	<-arrived
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("no error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("generate did not return")
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	dir := strings.Split(string(data), "\n")[3]
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("temporary directory remains: %v", err)
	}
}
