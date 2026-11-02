package hold

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "parent" {
		_, err := Start(os.Args[2], []string{"/bin/cat"}, os.Environ(), "", nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("ready")
		time.Sleep(time.Hour)
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "hold" {
		if err := Main(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	_ = os.Setenv("GORACE", "atexit_sleep_ms=0")
	os.Exit(m.Run())
}
