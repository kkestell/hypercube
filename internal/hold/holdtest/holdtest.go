// Package holdtest builds hypercube-hold for tests that start holders.
package holdtest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Path is the hypercube-hold executable that Main built.
var Path string

// Main builds hypercube-hold into a temporary directory, runs the tests, and
// removes the directory. A race-enabled test binary gets a race-enabled
// holder. Call it from TestMain.
func Main(m *testing.M) {
	dir, err := os.MkdirTemp("", "hypercube-hold-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	Path = filepath.Join(dir, "hypercube-hold")
	args := []string{"build", "-o", Path}
	if race {
		args = append(args, "-race")
	}
	args = append(args, "github.com/kkestell/hypercube/cmd/hypercube-hold")
	if out, err := exec.Command("go", args...).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build hypercube-hold: %v\n%s", err, out)
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
