package session

import (
	"testing"

	"github.com/kkestell/hypercube/internal/harness/harnesstest"
	"github.com/kkestell/hypercube/internal/hold"
	"github.com/kkestell/hypercube/internal/hold/holdtest"
	"github.com/kkestell/hypercube/internal/workspace/gittest"
)

func TestMain(m *testing.M) {
	gittest.Isolate()
	harnesstest.Run()
	holdtest.Main(m, hold.Main)
}
