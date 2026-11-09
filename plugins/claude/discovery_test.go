package main

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
)

func TestStartChecksSelection(t *testing.T) {
	for name, tc := range map[string]struct {
		model, effort, want string
	}{
		"unknown model":      {"sonnet", "high", `does not offer model "sonnet"`},
		"unsupported effort": {"haiku", "high", `does not accept effort "high"`},
		"effort without one": {"claude-plain", "low", `does not accept effort "low"`},
		"no effort":          {"opus", "", `needs an effort`},
	} {
		t.Run(name, func(t *testing.T) {
			fakeClaude(t, nil)
			a := startArgs
			a.Model, a.Effort = tc.model, tc.effort
			e := &emitted{}
			c, err := startClaude(a, e.emit)
			if err == nil {
				c.proc.Stop()
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("start = %v, want %q", err, tc.want)
			}
		})
	}
	// A resolved model name starts with its effort.
	args := fakeClaude(t, nil)
	a := startArgs
	a.Model, a.Effort = "claude-haiku-resolved", "low"
	start(t, a)
	data, err := os.ReadFile(args)
	if err != nil || !strings.Contains(string(data), "--effort low") || !strings.Contains(string(data), "--model claude-haiku-resolved") {
		t.Fatalf("arguments %s, %v", data, err)
	}
}

func TestModels(t *testing.T) {
	args := fakeClaude(t, nil)
	dir := t.TempDir()
	got, err := models(context.Background(), harness.ModelsRequest{Dir: dir})
	want := harness.Catalog{Default: "default", Models: []harness.Model{
		{ID: "default", Label: "Default (recommended)", Resolved: "claude-opus-resolved"},
		{ID: "opus", Label: "Opus", Resolved: "claude-opus-resolved"},
		{ID: "haiku", Label: "Haiku", Resolved: "claude-haiku-resolved"},
		{ID: "claude-plain", Label: "Plain"},
	}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %+v, %v", got, err)
	}
	data, _ := os.ReadFile(args)
	if strings.Contains(string(data), "--mcp-config") || strings.Contains(string(data), "--model") {
		t.Fatalf("discovery arguments %s", data)
	}
	for model, efforts := range map[string][]string{"haiku": {"low", "max"}, "claude-plain": nil} {
		got, err := models(context.Background(), harness.ModelsRequest{Dir: dir, Model: model})
		if err != nil || !reflect.DeepEqual(got.Efforts, efforts) {
			t.Fatalf("models for %s = %+v, %v", model, got, err)
		}
	}
	if _, err := models(context.Background(), harness.ModelsRequest{Dir: dir, Model: "sonnet"}); err == nil {
		t.Fatal("listed efforts for a model that is not offered")
	}
}
