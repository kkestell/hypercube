package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
)

// supportedImages are the formats the transcript shows.
var supportedImages = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true}

// maxImageFile limits the images read from disk.
const maxImageFile = 10 << 20

// image is the picture of a content block, if it is an inline one.
func (b contentBlock) image() (harness.Image, bool) {
	if b.ImageURL != "" {
		mime, data, ok := strings.Cut(strings.TrimPrefix(b.ImageURL, "data:"), ";base64,")
		if ok && strings.HasPrefix(b.ImageURL, "data:") && supportedImages[mime] && data != "" {
			return harness.Image{MIMEType: mime, Data: data}, true
		}
		return harness.Image{}, false
	}
	if b.Type == "image" && supportedImages[b.MIMEType] && b.Data != "" {
		return harness.Image{MIMEType: b.MIMEType, Data: b.Data}, true
	}
	return harness.Image{}, false
}

// fileImage reads an image file, which Codex wrote or viewed, for the transcript.
func fileImage(path string) (harness.Image, bool) {
	if path == "" {
		return harness.Image{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		return harness.Image{}, false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxImageFile+1))
	if err != nil || len(data) > maxImageFile {
		return harness.Image{}, false
	}
	mime := http.DetectContentType(data)
	if !supportedImages[mime] {
		return harness.Image{}, false
	}
	return harness.Image{MIMEType: mime, Data: base64.StdEncoding.EncodeToString(data)}, true
}

// rawImage is generated image data that Codex sent inline as base64.
func rawImage(b64 string) (harness.Image, bool) {
	head := b64
	if len(head) > 64 {
		head = head[:64]
	}
	head = head[:len(head)/4*4]
	data, err := base64.StdEncoding.DecodeString(head)
	if err != nil {
		return harness.Image{}, false
	}
	mime := http.DetectContentType(data)
	return harness.Image{MIMEType: mime, Data: b64}, supportedImages[mime]
}

// generatedImage shows an image Codex generated: the file it saved, else the
// data it sent inline. A generation that failed is an error.
func (ag *agent) generatedImage(ev *harness.Event, parent string, it threadItem) {
	var inline string
	_ = json.Unmarshal(it.Result, &inline) // the file's identity alone leaves it empty
	img, ok := fileImage(it.SavedPath)
	if !ok {
		img, ok = rawImage(inline)
	}
	switch {
	case ok:
		ag.complete(ev, harness.Item{ID: it.ID, Parent: parent, Kind: harness.KindText, Images: []harness.Image{img}})
	case it.Status == "failed" || len(it.Failure) > 0 && string(it.Failure) != "null":
		ag.finish(it.ID)
		ag.fail(ev, "Image generation failed.")
	default:
		ag.finish(it.ID)
	}
}
