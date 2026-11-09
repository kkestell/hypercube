package main

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
)

func TestStartChecksThinkingLevel(t *testing.T) {
	fakePi(t, nil)
	a := startArgs(t)
	a.Effort = "xhigh"
	if err := startFails(t, a); !strings.Contains(err.Error(), `does not accept thinking level "xhigh"`) {
		t.Fatal(err)
	}
	// A model with levels needs one.
	fakePi(t, map[string]string{"FAKE_LEVEL": "low"})
	a.Effort = ""
	if err := startFails(t, a); !strings.Contains(err.Error(), "needs a thinking level") {
		t.Fatal(err)
	}
	argsFile := fakePi(t, map[string]string{"FAKE_LEVEL": "low"})
	a.Effort = "low"
	ag, _ := start(t, a)
	data, _ := os.ReadFile(argsFile)
	if !strings.Contains(string(data), "--thinking low") || ag.started != (harness.Selection{Model: a.Model, Effort: "low"}) {
		t.Fatalf("arguments %s, started %+v", data, ag.started)
	}
}

func TestModels(t *testing.T) {
	argsFile := fakePi(t, nil)
	dir := t.TempDir()
	got, err := models(context.Background(), harness.ModelsRequest{Dir: dir})
	want := harness.Catalog{Default: "openrouter/x/m", Models: []harness.Model{
		{ID: "openrouter/x/m", Label: "M", Provider: "openrouter"}, {ID: "openai/plain", Label: "Plain", Provider: "openai"},
	}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %+v, %v", got, err)
	}
	data, _ := os.ReadFile(argsFile)
	if string(data) != "--mode rpc --no-session\n" {
		t.Fatalf("discovery arguments %q", data)
	}
	got, err = models(context.Background(), harness.ModelsRequest{Dir: dir, Model: "openrouter/x/m"})
	if err != nil || !reflect.DeepEqual(got.Efforts, []string{"off", "low", "high"}) {
		t.Fatalf("models for x/m = %+v, %v", got, err)
	}
	data, _ = os.ReadFile(argsFile)
	if string(data) != "--mode rpc --no-session --provider openrouter --model x/m\n" {
		t.Fatalf("discovery arguments %q", data)
	}
	// Pi falls back to another model for a name it does not know.
	if _, err := models(context.Background(), harness.ModelsRequest{Dir: dir, Model: "openai/other"}); err == nil || !strings.Contains(err.Error(), "instead of requested") {
		t.Fatal(err)
	}
}
