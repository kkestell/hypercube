package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

const maxSearchLines = 1000
const searchBatchSize = 4

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
	for start := 0; start < len(files) && count < maxSearchLines; start += searchBatchSize {
		if r.Context().Err() != nil {
			return
		}
		end := min(start+searchBatchSize, len(files))
		batch := make([][]searchLine, end-start)
		var workers sync.WaitGroup
		for i, path := range files[start:end] {
			workers.Add(1)
			go func() {
				defer workers.Done()
				batch[i] = searchFileLines(r.Context(), root, path, pattern)
			}()
		}
		workers.Wait()
		for i, lines := range batch {
			if len(lines) == 0 {
				continue
			}
			lines = lines[:min(len(lines), maxSearchLines-count)]
			results = append(results, searchFile{Path: files[start+i], Lines: lines})
			count += len(lines)
			if count == maxSearchLines {
				break
			}
		}
	}
	if r.Context().Err() != nil {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func searchFileLines(ctx context.Context, root *os.Root, path string, pattern *regexp.Regexp) []searchLine {
	f, err := root.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil
	}
	content, err := readText(f, info.Size())
	if err != nil {
		return nil
	}
	var lines []searchLine
	for n, text := range strings.Split(string(content), "\n") {
		if ctx.Err() != nil {
			return nil
		}
		text = strings.TrimSuffix(text, "\r")
		match := pattern.FindStringIndex(text)
		if match == nil {
			continue
		}
		lines = append(lines, searchLine{Line: n + 1, Parts: searchPreview(text, pattern, match)})
		if len(lines) == maxSearchLines {
			break
		}
	}
	return lines
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
