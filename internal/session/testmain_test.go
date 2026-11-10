package session

import (
	"testing"

	"github.com/kkestell/hypercube/internal/harness/harnesstest"
	"github.com/kkestell/hypercube/internal/hold"
	"github.com/kkestell/hypercube/internal/hold/holdtest"
)

func TestMain(m *testing.M) {
	harnesstest.Run()
	holdtest.Main(m, hold.Main)
}
