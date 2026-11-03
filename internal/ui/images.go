package ui

import (
	"mime"
	"net/http"
	"path"
	"strings"
)

// readImage serves preview images within the same directory boundary as files.
func (s *server) readImage(w http.ResponseWriter, r *http.Request) {
	root, err := s.sessionRoot(r.URL.Query().Get("session"))
	if fileError(w, err) {
		return
	}
	defer root.Close()
	name := r.URL.Query().Get("path")
	file, err := root.Open(name)
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
