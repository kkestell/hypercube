package limits

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Render reads the rate limits from the most recently written Codex log and
// leaves out windows that have reset.
func TestRenderCodexNewestLog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	t.Setenv("TMUX", "")
	now := time.Now()
	write := func(name string, mod time.Time, used int, resets int64) {
		dir := filepath.Join(home, "sessions", "2026", "01", "01")
		os.MkdirAll(dir, 0o755)
		line := fmt.Sprintf(`{"type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":%d,"window_minutes":300,"resets_at":%d},"secondary":{"used_percent":9,"window_minutes":10080,"resets_at":%d}}}}`+"\n", used, resets, now.Unix()-5)
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(line), 0o644)
		os.Chtimes(p, mod, mod)
	}
	write("old.jsonl", now.Add(-time.Hour), 11, now.Unix()+3600)
	write("new.jsonl", now, 22, now.Unix()+7500)
	if got, want := Render(), "codex 5h 22% ↻2h5m"; got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestDuration(t *testing.T) {
	for s, want := range map[int64]string{59: "0m", 47 * 60: "47m", 2*3600 + 13*60: "2h13m", 3 * 3600: "3h", 3*86400 + 4*3600: "3d4h", 2 * 86400: "2d"} {
		if got := duration(s); got != want {
			t.Errorf("duration(%d) = %q, want %q", s, got, want)
		}
	}
}
