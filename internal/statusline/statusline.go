// Package statusline renders Claude Code's native status line and clears its
// approval indicator when the harness reports that the dialog was answered.
package statusline

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/kkestell/hypercube/internal/mux"
	"github.com/kkestell/hypercube/internal/store"
)

func Run(limits bool, input io.Reader, out io.Writer) error {
	id, path := os.Getenv("HYPERCUBE_AGENT_ID"), os.Getenv("HYPERCUBE_STATE")
	if id != "" && path != "" {
		if s, err := store.Load(path); err == nil {
			if a, err := s.Agent(id); err == nil && a.Attention {
				_ = store.Update(path, func(s *store.State) error {
					a, err := s.Agent(id)
					if err == nil {
						a.Attention = false
					}
					return err
				})
				mux.RefreshAttention(path, id)
			}
		}
	}
	var status map[string]any
	_ = json.NewDecoder(input).Decode(&status)
	value := func(parts ...string) any {
		var v any = status
		for _, part := range parts {
			obj, _ := v.(map[string]any)
			v = obj[part]
		}
		return v
	}
	text := func(v any) string { s, _ := v.(string); return s }
	number := func(v any) float64 { n, _ := v.(float64); return n }
	paint := func(color, text string) string { return "\x1b[" + color + "m" + text + "\x1b[0m" }
	line := paint("38;2;218;96;0", "✳︎ ") + paint("38;2;218;96;0", text(value("model", "display_name"))) + paint("38;2;255;255;255", " :: ")
	if value("thinking", "enabled") == true {
		line += paint("38;2;255;151;69", text(value("effort", "level")))
	}
	line += fmt.Sprintf(" %s/%s (%d%%)", tokens(number(value("context_window", "total_input_tokens"))), tokens(number(value("context_window", "context_window_size"))), int(number(value("context_window", "used_percentage"))))
	if limits {
		line += fmt.Sprintf(" | 5h %s | 7d %s", bar(number(value("rate_limits", "five_hour", "used_percentage"))), bar(number(value("rate_limits", "seven_day", "used_percentage"))))
	}
	_, err := fmt.Fprint(out, line)
	return err
}

func tokens(n float64) string {
	if n < 0 {
		n = 0
	}
	whole := int64(n)
	if whole < 1000 {
		return fmt.Sprint(whole)
	}
	unit, scale := "k", int64(1000)
	if whole >= 1000000 {
		unit, scale = "M", 1000000
	}
	if tenth := whole % scale / (scale / 10); tenth != 0 {
		return fmt.Sprintf("%d.%d%s", whole/scale, tenth, unit)
	}
	return fmt.Sprintf("%d%s", whole/scale, unit)
}

func bar(percent float64) string {
	filled := (int(math.Max(0, math.Min(100, percent)))*10 + 50) / 100
	return strings.Repeat("▪", filled) + strings.Repeat("·", 10-filled)
}
