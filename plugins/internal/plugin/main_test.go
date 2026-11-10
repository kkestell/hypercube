package plugin

import (
	"io"
	"strings"
	"testing"
)

func TestGenerateWithoutSupportFailsWithTheProduct(t *testing.T) {
	p := Plugin{Product: "Stand-in", Executable: "sh"}
	err := p.run("generate", strings.NewReader(`{"model":"m","prompt":"p"}`), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "Stand-in does not run utility prompts") {
		t.Fatalf("generate = %v", err)
	}
}
