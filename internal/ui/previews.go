package ui

import (
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

type filePreview struct {
	MIME string `json:"mime"`
	URL  string `json:"url"`
}

func previewMIME(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	// TypeScript source takes precedence over MPEG transport streams (.ts, .mts).
	switch ext {
	case ".ts", ".tsx", ".mts", ".cts":
		return ""
	}
	typ := mime.TypeByExtension(ext)
	base, _, _ := mime.ParseMediaType(typ)
	if strings.HasPrefix(base, "image/") || strings.HasPrefix(base, "audio/") || strings.HasPrefix(base, "video/") || base == "application/pdf" || base == "text/html" {
		return base
	}
	return ""
}

// previewURL keeps the file's absolute path in the URL so relative resources
// in an HTML document resolve alongside it, including outside the session dir.
func (s *server) previewURL(session, name, etag string) (string, error) {
	if !filepath.IsAbs(name) {
		dir, err := s.sessionDir(session)
		if err != nil {
			return "", err
		}
		name = filepath.Join(dir, name)
	}
	u := url.URL{Path: "/preview/" + session + filepath.ToSlash(name), RawQuery: url.Values{"v": {etag}}.Encode()}
	return u.String(), nil
}

// readPreview streams browser content, including byte ranges for media and
// PDF readers. It uses the same absolute-file access as an agent's open_file.
func (s *server) readPreview(w http.ResponseWriter, r *http.Request) {
	name := "/" + r.PathValue("path")
	file, err := s.openFile(r.PathValue("session"), name)
	if fileError(w, err) {
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if fileError(w, err) {
		return
	}
	if !info.Mode().IsRegular() {
		http.Error(w, "cannot preview this file", http.StatusBadRequest)
		return
	}
	typ := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	if typ != "" {
		w.Header().Set("Content-Type", typ)
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Local documents must not run scripts on the app's origin. PDF readers
	// need their browser's built-in viewer, which a sandbox would disable.
	if previewMIME(name) != "application/pdf" {
		w.Header().Set("Content-Security-Policy", "sandbox")
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, name, info.ModTime(), file)
}
