package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildRendersPages(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "content/index.md"), "---\ntitle: Home\n---\n# Hello\n")
	write(t, filepath.Join(dir, "templates/page.html"), `{{define "page.html"}}<title>{{.Title}}</title>{{.Body}}{{end}}`)
	site := Site{Content: filepath.Join(dir, "content"), Templates: filepath.Join(dir, "templates"), Out: filepath.Join(dir, "public")}
	n, err := site.Build()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("built %d pages, want 1", n)
	}
	got, err := os.ReadFile(filepath.Join(dir, "public/index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "<title>Home</title><h1>Hello</h1>\n"; string(got) != want {
		t.Errorf("index.html = %q, want %q", got, want)
	}
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
