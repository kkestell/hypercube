package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/coder/websocket"
	"github.com/kkestell/hypercube/internal/terminal"
)

// terminal connects the page to the session's shell. Binary messages carry the
// shell's output to the page and the page's input to the shell. The page sends
// {"cols": n, "rows": n} to resize it, and the shell's end sends {"exit": n}.
func (s *server) terminal(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	q := r.URL.Query()
	size := func(name string, fallback int) int {
		n, err := strconv.Atoi(q.Get(name))
		if err != nil || n < 1 || n > 1000 {
			return fallback
		}
		return n
	}
	cols, rows := size("cols", 80), size("rows", 24)
	target := requestTarget(r)
	var sh *terminal.Shell
	if target.workspace != "" {
		sh, err = s.rt.WorkspaceShell(target.workspace, cols, rows)
	} else {
		sh, err = s.rt.Shell(target.session, cols, rows)
	}
	if err != nil {
		// Text marks the shell as ended, so the page does not reconnect.
		_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"exit":-1}`))
		_ = conn.Close(websocket.StatusInternalError, "shell startup failed")
		return
	}
	conn.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	_ = sh.Resize(cols, rows)
	out, err := sh.Output()
	if err != nil {
		return
	}
	defer out.Close()
	go func() { <-ctx.Done(); _ = out.Close() }()
	go func() {
		defer cancel()
		for {
			kind, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if kind == websocket.MessageBinary {
				if sh.Write(data) != nil {
					return
				}
				continue
			}
			var resize struct{ Cols, Rows int }
			if json.Unmarshal(data, &resize) == nil && resize.Cols > 0 && resize.Rows > 0 && resize.Cols <= 1000 && resize.Rows <= 1000 {
				_ = sh.Resize(resize.Cols, resize.Rows)
			}
		}
	}()
	buf := make([]byte, 32<<10)
	for {
		n, err := out.Read(buf)
		if n > 0 && conn.Write(ctx, websocket.MessageBinary, buf[:n]) != nil {
			return
		}
		// The output ends when the shell ends, when it is stopped and removed,
		// or when the page goes away and these writes fail.
		if err != nil {
			exit, _ := json.Marshal(map[string]int{"exit": sh.ExitCode()})
			_ = conn.Write(ctx, websocket.MessageText, exit)
			_ = conn.Close(websocket.StatusNormalClosure, "")
			return
		}
	}
}
