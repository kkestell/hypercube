package statusline

import (
	"bytes"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/store"
)

func TestStatuslineAndApprovalAnswer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	t.Setenv("HYPERCUBE_AGENT_ID", "o")
	t.Setenv("HYPERCUBE_STATE", path)
	if err := store.Update(path, func(s *store.State) error { s.Agents = []store.Agent{{ID: "o", Attention: true}}; return nil }); err != nil {
		t.Fatal(err)
	}
	source := `{"model":{"display_name":"Opus"},"thinking":{"enabled":true},"effort":{"level":"high"},"context_window":{"total_input_tokens":45300,"context_window_size":200000,"used_percentage":22.6},"rate_limits":{"five_hour":{"used_percentage":34},"seven_day":{"used_percentage":71.5}}}`
	ansi := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	for _, showLimits := range []bool{false, true} {
		var out bytes.Buffer
		if err := Run(showLimits, strings.NewReader(source), &out); err != nil {
			t.Fatal(err)
		}
		want := "✳︎ Opus :: high 45.3k/200k (22%)"
		if showLimits {
			want += " | 5h ▪▪▪······· | 7d ▪▪▪▪▪▪▪···"
		}
		if got := ansi.ReplaceAllString(out.String(), ""); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
	s, _ := store.Load(path)
	if s.Agents[0].Attention {
		t.Fatal("answered approval is still flagged")
	}
	for _, source := range []string{`null`, `{}`, `{"model":null,"thinking":null}`, `malformed`} {
		var out bytes.Buffer
		if err := Run(false, strings.NewReader(source), &out); err != nil {
			t.Fatal(err)
		}
		if got := ansi.ReplaceAllString(out.String(), ""); got != "✳︎  ::  0/0 (0%)" {
			t.Fatal(got)
		}
	}
}
