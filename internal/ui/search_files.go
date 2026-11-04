package ui

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const maxSearchLines = 1000

type searchLine struct {
	Line  int      `json:"line"`
	Parts []string `json:"parts"`
}
type searchFile struct {
	Path  string       `json:"path"`
	Lines []searchLine `json:"lines"`
}

func (s *server) searchFiles(w http.ResponseWriter, r *http.Request) {
	root, err := s.sessionRoot(r.URL.Query().Get("session"))
	if fileError(w, err) {
		return
	}
	defer root.Close()
	query := r.URL.Query().Get("q")
	results := []searchFile{}
	if query == "" {
		json.NewEncoder(w).Encode(results)
		return
	}
	files, err := mentionFileList(r.Context(), root)
	if fileError(w, err) {
		return
	}
	pattern, err := regexp.Compile("(?i)" + regexp.QuoteMeta(query))
	if fileError(w, err) {
		return
	}
	count := 0
	for _, path := range files {
		if r.Context().Err() != nil {
			return
		}
		f, err := root.Open(path)
		if err != nil {
			continue
		}
		reader := bufio.NewReaderSize(f, 8000)
		prefix, _ := reader.Peek(8000)
		if strings.ContainsRune(string(prefix), 0) {
			f.Close()
			continue
		}
		var lines []searchLine
		for n := 1; ; n++ {
			text, readErr := reader.ReadString('\n')
			if readErr != nil && readErr != io.EOF {
				break
			}
			if text == "" && readErr == io.EOF {
				break
			}
			text = strings.TrimSuffix(strings.TrimSuffix(text, "\n"), "\r")
			indexes := pattern.FindAllStringIndex(text, -1)
			if len(indexes) == 0 {
				if readErr == io.EOF {
					break
				}
				continue
			}
			parts := []string{}
			last := 0
			for _, m := range indexes {
				parts = append(parts, text[last:m[0]], text[m[0]:m[1]])
				last = m[1]
			}
			parts = append(parts, text[last:])
			lines = append(lines, searchLine{Line: n, Parts: parts})
			count++
			if count == maxSearchLines {
				break
			}
		}
		f.Close()
		if len(lines) > 0 {
			results = append(results, searchFile{Path: path, Lines: lines})
		}
		if count >= maxSearchLines {
			break
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}
