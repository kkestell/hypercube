// Package holdtest runs the real holder in a test binary's helper process.
package holdtest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Path starts this package's test binary in holder mode.
var Path string

// Main serves holder-mode invocations or runs the tests. Passing hold.Main
// avoids an import cycle with the hold package's own tests. The helper inherits
// the test binary's race instrumentation without a nested Go build.
func Main(m *testing.M, holder func([]string) error) {
	if len(os.Args) > 1 && os.Args[1] == "hold" {
		if err := holder(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "hypercube-hold-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	Path = filepath.Join(dir, "hypercube-hold")
	exe, err := os.Executable()
	if err == nil {
		script := fmt.Sprintf("#!/bin/sh\nexec '%s' hold \"$@\"\n", strings.ReplaceAll(exe, "'", "'\\''"))
		err = os.WriteFile(Path, []byte(script), 0o755)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	// Holders inherit this, so a race-enabled holder exits at once instead of
	// sleeping a second to report races.
	_ = os.Setenv("GORACE", "atexit_sleep_ms=0")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
