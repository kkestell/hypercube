package ui

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
)

func TestBrowserPreviews(t *testing.T) {
	dir := t.TempDir()
	db := open(t)
	change(t, db, func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "w", Path: dir}); err != nil {
			return err
		}
		return tx.AddSession(store.Session{ID: "s", Workspace: "w", Dir: dir})
	})
	h := handler(t, db)
	get := func(address string, headers map[string]string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("GET", "http://localhost:4280"+address, nil)
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		return res
	}
	for _, tc := range []struct {
		name, mime, content string
		source              bool
	}{
		{"picture #1.PNG", "image/png", "\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", maxFile), false},
		{"sound.mp3", "audio/mpeg", "ID3\x00sound", false},
		{"clip.mp4", "video/mp4", "\x00\x00\x00\x18ftypmp42", false},
		{"document.pdf", "application/pdf", "%PDF-1.7\nexample", false},
		{"index.html", "text/html", `<html><img src="picture%20%231.PNG"></html>`, true},
		{"drawing.svg", "image/svg+xml", `<svg xmlns="http://www.w3.org/2000/svg"/>`, true},
		{"utf16.html", "text/html", "\xff\xfe<\x00h\x001\x00>\x00H\x00i\x00<\x00/\x00h\x001\x00>\x00", false},
		{"large.html", "text/html", strings.Repeat("x", maxFile+1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name := filepath.Join(dir, tc.name)
			if err := os.WriteFile(name, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			address := "/file?" + url.Values{"session": {"s"}, "path": {tc.name}}.Encode()
			res := get(address, nil)
			if res.Code != 200 || res.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("metadata = %d %v %s", res.Code, res.Header(), res.Body)
			}
			var metadata struct {
				Text    *string     `json:"text"`
				Preview filePreview `json:"preview"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &metadata); err != nil {
				t.Fatal(err)
			}
			if metadata.Preview.MIME != tc.mime || (metadata.Text != nil) != tc.source {
				t.Fatalf("metadata = %+v", metadata)
			}
			if metadata.Text != nil && *metadata.Text != tc.content {
				t.Fatal("source text changed")
			}
			etag := res.Header().Get("ETag")
			if got := get(address, map[string]string{"If-None-Match": etag}); got.Code != 304 {
				t.Fatalf("conditional read = %d", got.Code)
			}
			raw := get(metadata.Preview.URL, nil)
			if raw.Code != 200 || raw.Body.String() != tc.content || !strings.HasPrefix(raw.Header().Get("Content-Type"), tc.mime) {
				t.Fatalf("preview = %d %v (%d bytes)", raw.Code, raw.Header(), raw.Body.Len())
			}
			if raw.Header().Get("X-Content-Type-Options") != "nosniff" || raw.Header().Get("Cache-Control") != "no-cache" {
				t.Fatalf("preview headers = %v", raw.Header())
			}
			if tc.mime != "application/pdf" && raw.Header().Get("Content-Security-Policy") != "sandbox" {
				t.Fatal("preview is not sandboxed")
			}
			part := get(metadata.Preview.URL, map[string]string{"Range": "bytes=0-3"})
			if part.Code != 206 || part.Body.String() != tc.content[:4] {
				t.Fatalf("range = %d %s", part.Code, part.Body)
			}
			if tc.mime == "text/html" && tc.source {
				base, err := url.Parse(metadata.Preview.URL)
				if err != nil {
					t.Fatal(err)
				}
				ref, _ := url.Parse("picture%20%231.PNG")
				if image := get(base.ResolveReference(ref).String(), nil); image.Code != 200 || !strings.HasPrefix(image.Header().Get("Content-Type"), "image/png") {
					t.Fatalf("relative image = %d %v", image.Code, image.Header())
				}
			}
		})
	}
	for _, tc := range []struct {
		address string
		status  int
	}{
		{"/preview/missing/etc/hosts", 404},
		{"/preview/s" + dir + "/missing.png", 404},
		{"/preview/s" + dir + "/", 400},
	} {
		if res := get(tc.address, nil); res.Code != tc.status {
			t.Errorf("%s = %d %s; want %d", tc.address, res.Code, res.Body, tc.status)
		}
	}
}
