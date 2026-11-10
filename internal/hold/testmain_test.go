package hold

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/hold/holdtest"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "parent" {
		_, err := Start(os.Args[2], os.Args[3], []string{"/bin/cat"}, os.Environ(), "", nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("ready")
		time.Sleep(time.Hour)
		os.Exit(0)
	}
	holdtest.Main(m, Main)
}
