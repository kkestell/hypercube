package session

import (
	"fmt"
	"github.com/kkestell/hypercube/internal/hold"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "hold" {
		if err := hold.Main(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	_ = os.Setenv("GORACE", "atexit_sleep_ms=0")
	os.Exit(m.Run())
}
