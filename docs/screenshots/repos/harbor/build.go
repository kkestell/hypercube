package main

import (
	"bytes"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
)

// Site is a folder of Markdown pages, the templates they render with, and
// the folder the HTML goes to.
type Site struct {
	Content   string
	Templates string
	Out       string
}

// Page is one rendered page.
type Page struct {
	Title string
	Body  template.HTML
}

// Build renders every page under Content into Out and returns how many it
// wrote.
func (s Site) Build() (int, error) {
	tmpl, err := template.ParseGlob(filepath.Join(s.Templates, "*.html"))
	if err != nil {
		return 0, err
	}
	if err := os.RemoveAll(s.Out); err != nil {
		return 0, err
	}
	n := 0
	err = filepath.WalkDir(s.Content, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".md" {
			return err
		}
		if err := s.render(tmpl, path); err != nil {
			return err
		}
		n++
		return nil
	})
	return n, err
}

func (s Site) render(tmpl *template.Template, path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	meta, body := frontMatter(src)
	var html bytes.Buffer
	if err := goldmark.Convert(body, &html); err != nil {
		return err
	}
	rel, _ := filepath.Rel(s.Content, path)
	out := filepath.Join(s.Out, strings.TrimSuffix(rel, ".md")+".html")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	name := meta["template"]
	if name == "" {
		name = "page.html"
	}
	return tmpl.ExecuteTemplate(f, name, Page{Title: meta["title"], Body: template.HTML(html.String())})
}

// frontMatter splits "key: value" lines between --- fences from the page's
// body.
func frontMatter(src []byte) (map[string]string, []byte) {
	meta := map[string]string{}
	rest, ok := bytes.CutPrefix(src, []byte("---\n"))
	if !ok {
		return meta, src
	}
	head, body, ok := bytes.Cut(rest, []byte("\n---\n"))
	if !ok {
		return meta, src
	}
	for _, line := range strings.Split(string(head), "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok {
			meta[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return meta, body
}
