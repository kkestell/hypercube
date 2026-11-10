package terminal

import (
	"testing"

	"github.com/kkestell/hypercube/internal/hold"
	"github.com/kkestell/hypercube/internal/hold/holdtest"
)

func TestMain(m *testing.M) { holdtest.Main(m, hold.Main) }
