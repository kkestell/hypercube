package harness

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestCheckImages(t *testing.T) {
	png := "\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 16)
	gif := "GIF89a" + strings.Repeat("\x00", 16)
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	big := b64(png + strings.Repeat("\x00", 6<<20))
	exact := func(n int) string { return b64(png + strings.Repeat("\x00", n-len(png))) }
	half := MaxImageBytes/2 - 1 // an odd size, so its base64 is padded
	tests := []struct {
		name   string
		images []Image
		ok     bool
	}{
		{"none", nil, true},
		{"png and gif", []Image{{"image/png", b64(png)}, {"image/gif", b64(gif)}}, true},
		{"unsupported type", []Image{{"image/svg+xml", b64("<svg/>")}}, false},
		{"type does not match bytes", []Image{{"image/jpeg", b64(png)}}, false},
		{"not base64", []Image{{"image/png", "!!"}}, false},
		{"data url", []Image{{"image/png", "data:image/png;base64," + b64(png)}}, false},
		{"empty", []Image{{"image/png", ""}}, false},
		{"exactly the limit", []Image{{"image/png", exact(MaxImageBytes)}}, true},
		{"the limit in two images", []Image{{"image/png", exact(MaxImageBytes / 2)}, {"image/png", exact(MaxImageBytes / 2)}}, true},
		{"padded images at the limit", []Image{{"image/png", exact(half)}, {"image/png", exact(MaxImageBytes - half)}}, true},
		{"one byte over the limit", []Image{{"image/png", exact(MaxImageBytes + 1)}}, false},
		{"one image over the limit", []Image{{"image/png", b64(png + strings.Repeat("\x00", MaxImageBytes))}}, false},
		{"images over the limit in all", []Image{{"image/png", big}, {"image/png", big}}, false},
	}
	for _, tc := range tests {
		if err := CheckImages(tc.images); (err == nil) != tc.ok {
			t.Errorf("%s: err = %v, want ok = %v", tc.name, err, tc.ok)
		}
	}
}
