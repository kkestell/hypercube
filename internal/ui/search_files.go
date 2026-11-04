package ui

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
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
		info, err := f.Stat()
		if err != nil {
			f.Close()
			continue
		}
		content, err := readText(f, info.Size())
		f.Close()
		if err != nil {
			continue
		}
		var lines []searchLine
		for n, text := range strings.Split(string(content), "\n") {
			text = strings.TrimSuffix(text, "\r")
			match := pattern.FindStringIndex(text)
			if match == nil {
				continue
			}
			lines = append(lines, searchLine{Line: n + 1, Parts: searchPreview(text, pattern, match)})
			count++
			if count == maxSearchLines {
				break
			}
		}
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

// searchPreview limits both the displayed text and the number of marked matches.
func searchPreview(text string, pattern *regexp.Regexp, first []int) []string {
	runes := []rune(text)
	matchStart := utf8.RuneCountInString(text[:first[0]])
	matchEnd := utf8.RuneCountInString(text[:first[1]])
	start := max(0, matchStart-100)
	end := min(len(runes), start+200)
	matchEnd = min(matchEnd, end)
	parts := []string{string(runes[start:matchStart]), string(runes[matchStart:matchEnd])}
	rest := string(runes[matchEnd:end])
	last := 0
	for _, match := range pattern.FindAllStringIndex(rest, -1) {
		parts = append(parts, rest[last:match[0]], rest[match[0]:match[1]])
		last = match[1]
	}
	return append(parts, rest[last:])
}
