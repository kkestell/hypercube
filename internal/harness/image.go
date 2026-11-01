package harness

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// MaxImageBytes is the most image data, decoded, that one prompt carries.
const MaxImageBytes = 10 << 20

// CheckImages reports whether a prompt's images are PNG, JPEG, GIF, or WebP
// pictures: valid base64 whose bytes match the MIME type, within MaxImageBytes
// in all.
func CheckImages(images []Image) error {
	total := 0
	for i, img := range images {
		switch img.MIMEType {
		case "image/png", "image/jpeg", "image/gif", "image/webp":
		default:
			return fmt.Errorf("image %d: unsupported type %q; use PNG, JPEG, GIF, or WebP", i+1, img.MIMEType)
		}
		// The decoded size is exact once the padding is not counted.
		if total += base64.StdEncoding.DecodedLen(len(img.Data)) - strings.Count(img.Data[max(0, len(img.Data)-2):], "="); total > MaxImageBytes {
			return errors.New("images are larger than 10 MiB in all")
		}
		data, err := base64.StdEncoding.DecodeString(img.Data)
		if err != nil {
			return fmt.Errorf("image %d: invalid base64", i+1)
		}
		if http.DetectContentType(data) != img.MIMEType {
			return fmt.Errorf("image %d is not %s", i+1, img.MIMEType)
		}
	}
	return nil
}
