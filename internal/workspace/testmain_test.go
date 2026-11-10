package workspace

import (
	"os"
	"testing"

	"github.com/kkestell/hypercube/internal/workspace/gittest"
)

func TestMain(m *testing.M) {
	gittest.Isolate()
	os.Exit(m.Run())
}
