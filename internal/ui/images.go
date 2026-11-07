package ui

import (
	"encoding/base64"
	"mime"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
)

// readImage serves preview images by the same rules as files.
func (s *server) readImage(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("path")
	file, err := s.openFile(r.URL.Query().Get("session"), name)
	if fileError(w, err) {
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if fileError(w, err) {
		return
	}
	if !info.Mode().IsRegular() {
		http.Error(w, "cannot read an image from this file", http.StatusBadRequest)
		return
	}
	contentType := mime.TypeByExtension(strings.ToLower(path.Ext(name)))
	if !strings.HasPrefix(contentType, "image/") {
		http.Error(w, "file is not an image", http.StatusUnsupportedMediaType)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// An SVG opened directly must not run scripts on the app's origin.
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, name, info.ModTime(), file)
}

// readTranscriptImage serves an image of a transcript item. Transcript
// messages link to images instead of carrying them, so a session's
// screenshots load only when the page shows them.
func (s *server) readTranscriptImage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	index, err := strconv.Atoi(q.Get("index"))
	if err != nil {
		http.Error(w, "invalid image index", http.StatusBadRequest)
		return
	}
	img, err := s.rt.TranscriptImage(q.Get("session"), q.Get("item"), index)
	if fileError(w, err) {
		return
	}
	data, err := base64.StdEncoding.DecodeString(img.Data)
	if err != nil || !slices.Contains([]string{"image/png", "image/jpeg", "image/gif", "image/webp"}, img.MIMEType) {
		http.Error(w, "transcript image is not a PNG, JPEG, GIF, or WebP picture", http.StatusUnsupportedMediaType)
		return
	}
	w.Header().Set("Content-Type", img.MIMEType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}

// imageLinks returns the addresses of an item's images.
func imageLinks(session, item string, images []harness.Image) []string {
	links := make([]string, len(images))
	for i := range images {
		links[i] = "/transcript-image?" + url.Values{"session": {session}, "item": {item}, "index": {strconv.Itoa(i)}}.Encode()
	}
	return links
}
