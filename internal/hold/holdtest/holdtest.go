// Package holdtest lets a test binary serve as the holder. Holders start
// os.Executable, which in a test is the test binary itself.
package holdtest

import (
	"fmt"
	"os"
	"testing"

	"github.com/kkestell/hypercube/internal/hold"
)

// Main runs the holder when the test binary is started as `hold`, and the
// tests otherwise. Call it from TestMain.
func Main(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "hold" {
		if err := hold.Main(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	// Holders inherit this, so a race-enabled holder exits at once instead of
	// sleeping a second to report races.
	_ = os.Setenv("GORACE", "atexit_sleep_ms=0")
	os.Exit(m.Run())
}
