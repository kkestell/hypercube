// Package limits shows Claude Code's and Codex's usage limits in the tmux
// status bar.
package limits

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kkestell/hypercube/internal/tmux"
	"github.com/kkestell/hypercube/internal/workspace"
)

// claudeOption is the global tmux option that holds Claude Code's latest
// rate limits as JSON.
const claudeOption = "@hypercube_claude_limits"

type window struct {
	label   string
	percent float64
	resets  int64
}

type claudeWindow struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
}

type claudeLimits struct {
	FiveHour *claudeWindow `json:"five_hour,omitempty"`
	SevenDay *claudeWindow `json:"seven_day,omitempty"`
}

// Statusline is the status line command of Claude Code panes. Claude Code
// gives rate limits only to status line commands. It saves them for the tmux
// status bar and prints the context usage and the branch of the agent's
// checkout, with * when it has uncommitted changes.
func Statusline(stdin io.Reader, stdout io.Writer) error {
	var in struct {
		RateLimits    *claudeLimits `json:"rate_limits"`
		ContextWindow struct {
			UsedPercentage float64 `json:"used_percentage"`
		} `json:"context_window"`
		Workspace struct {
			CurrentDir string `json:"current_dir"`
		} `json:"workspace"`
	}
	if err := json.NewDecoder(stdin).Decode(&in); err != nil {
		return err
	}
	if in.RateLimits != nil {
		saved, _ := json.Marshal(in.RateLimits)
		if err := tmux.SetGlobal(claudeOption, string(saved)); err != nil {
			return err
		}
	}
	line := fmt.Sprintf("ctx %.0f%%", in.ContextWindow.UsedPercentage)
	if branch, err := workspace.Branch(in.Workspace.CurrentDir); err == nil && branch != "" {
		if dirty, err := workspace.Dirty(in.Workspace.CurrentDir); err == nil && dirty {
			branch += "*"
		}
		line += " · " + branch
	}
	_, err := fmt.Fprint(stdout, line)
	return err
}

// Render returns the status bar text: each harness's limits that have not
// reset yet.
func Render() string {
	now := time.Now().Unix()
	var parts []string
	for _, h := range []struct {
		name    string
		windows []window
	}{{"claude", claude()}, {"codex", codex()}} {
		var shown []string
		for _, w := range h.windows {
			if w.resets > now {
				shown = append(shown, fmt.Sprintf("%s %.0f%% ↻%s", w.label, w.percent, duration(w.resets-now)))
			}
		}
		if len(shown) > 0 {
			parts = append(parts, h.name+" "+strings.Join(shown, "  "))
		}
	}
	return strings.Join(parts, " · ")
}

func claude() []window {
	var l claudeLimits
	if json.Unmarshal([]byte(tmux.Global(claudeOption)), &l) != nil {
		return nil
	}
	var windows []window
	if w := l.FiveHour; w != nil {
		windows = append(windows, window{"5h", w.UsedPercentage, w.ResetsAt})
	}
	if w := l.SevenDay; w != nil {
		windows = append(windows, window{"7d", w.UsedPercentage, w.ResetsAt})
	}
	return windows
}

// codex reads the rate limits from the most recently written Codex session
// log.
func codex() []window {
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		h, _ := os.UserHomeDir()
		home = filepath.Join(h, ".codex")
	}
	var newest string
	var newestTime time.Time
	filepath.WalkDir(filepath.Join(home, "sessions"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().After(newestTime) {
			newest, newestTime = path, info.ModTime()
		}
		return nil
	})
	if newest == "" {
		return nil
	}

	f, err := os.Open(newest)
	if err != nil {
		return nil
	}
	defer f.Close()
	// The latest token count is near the end of the log.
	if info, err := f.Stat(); err == nil && info.Size() > 1<<20 {
		f.Seek(-1<<20, io.SeekEnd)
	}
	type codexWindow struct {
		UsedPercent   float64 `json:"used_percent"`
		WindowMinutes int     `json:"window_minutes"`
		ResetsAt      int64   `json:"resets_at"`
	}
	var last struct {
		Payload struct {
			RateLimits struct {
				Primary   *codexWindow `json:"primary"`
				Secondary *codexWindow `json:"secondary"`
			} `json:"rate_limits"`
		} `json:"payload"`
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(nil, 16<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if bytes.Contains(line, []byte(`"type":"token_count"`)) && bytes.Contains(line, []byte(`"rate_limits":{`)) {
			json.Unmarshal(line, &last)
		}
	}

	var windows []window
	for _, w := range []*codexWindow{last.Payload.RateLimits.Primary, last.Payload.RateLimits.Secondary} {
		if w != nil {
			windows = append(windows, window{duration(int64(w.WindowMinutes) * 60), w.UsedPercent, w.ResetsAt})
		}
	}
	return windows
}

// duration formats seconds as 3d4h, 2h13m, or 47m.
func duration(s int64) string {
	d, h, m := s/86400, s%86400/3600, s%3600/60
	switch {
	case d > 0 && h > 0:
		return fmt.Sprintf("%dd%dh", d, h)
	case d > 0:
		return fmt.Sprintf("%dd", d)
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh%dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dm", m)
}
