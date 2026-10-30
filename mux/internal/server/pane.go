package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/creack/pty"
	"go.mitchellh.com/libghostty"
)

const (
	defaultCols = 80
	defaultRows = 24
	scrollback  = 10000
)

// pane is a process on a PTY whose output feeds a terminal. The server's mutex
// guards term and rs, which are not safe for concurrent use.
type pane struct {
	id   string
	cmd  *exec.Cmd
	ptmx *os.File
	term *libghostty.Terminal
	rs   *libghostty.RenderState
}

// startPane runs argv with exactly env and starts a goroutine that feeds the
// process's output to the pane's terminal.
func (s *Server) startPane(id, dir string, argv, env []string) (*pane, error) {
	path, err := lookPath(argv[0], env)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(path, argv[1:]...)
	cmd.Dir = dir
	// A nil Env would inherit the server's environment.
	cmd.Env = append([]string{}, env...)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: defaultRows, Cols: defaultCols})
	if err != nil {
		return nil, err
	}

	p := &pane{id: id, cmd: cmd, ptmx: ptmx}
	// The callback runs with the server's mutex held. A full PTY input buffer
	// could block it; that is unlikely.
	p.term, err = libghostty.NewTerminal(
		libghostty.WithSize(defaultCols, defaultRows),
		libghostty.WithMaxScrollbackLines(scrollback),
		libghostty.WithWritePty(func(_ *libghostty.Terminal, data []byte) {
			ptmx.Write(data)
		}),
	)
	if err == nil {
		p.rs, err = libghostty.NewRenderState()
	}
	if err != nil {
		ptmx.Close()
		cmd.Process.Kill()
		cmd.Wait()
		return nil, err
	}
	go s.readPane(p)
	return p, nil
}

func (s *Server) readPane(p *pane) {
	buf := make([]byte, 32*1024)
	for {
		n, err := p.ptmx.Read(buf)
		if n > 0 {
			s.mu.Lock()
			p.term.VTWrite(buf[:n])
			s.wake(p)
			s.mu.Unlock()
		}
		if err != nil {
			// The process exited. Task 2 records its status.
			p.cmd.Wait()
			return
		}
	}
}

// resize must be called with the server's mutex held.
func (p *pane) resize(cols, rows uint16) error {
	if err := p.term.Resize(cols, rows, 0, 0); err != nil {
		return err
	}
	return pty.Setsize(p.ptmx, &pty.Winsize{Rows: rows, Cols: cols})
}

// capture must be called with the server's mutex held.
func (p *pane) capture() (string, error) {
	f, err := libghostty.NewFormatter(p.term,
		libghostty.WithFormatterFormat(libghostty.FormatterFormatPlain),
		libghostty.WithFormatterUnwrap(true),
		libghostty.WithFormatterTrim(true),
	)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return f.FormatString()
}

// lookPath finds name in the PATH from env, not the server's, which is what
// exec.Command would use.
func lookPath(name string, env []string) (string, error) {
	if strings.Contains(name, "/") {
		return name, nil
	}
	for _, kv := range env {
		path, ok := strings.CutPrefix(kv, "PATH=")
		if !ok {
			continue
		}
		for _, dir := range filepath.SplitList(path) {
			candidate := filepath.Join(dir, name)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
				return candidate, nil
			}
		}
		break
	}
	return "", fmt.Errorf("%s: not found in PATH", name)
}
