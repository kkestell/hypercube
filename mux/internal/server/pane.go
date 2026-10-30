package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/creack/pty"
	"go.mitchellh.com/libghostty"
)

const (
	defaultCols = 80
	defaultRows = 24
	scrollback  = 10000
)

// pane is a process on a PTY whose output feeds a terminal. The server's mutex
// guards alive, status, killed, cols, rows, term, and rs; the terminal and
// render state are not safe for concurrent use.
type pane struct {
	id     string
	cmd    *exec.Cmd
	ptmx   *os.File
	term   *libghostty.Terminal
	rs     *libghostty.RenderState
	alive  bool
	status int  // the exit status once alive is false
	killed bool // the pane left the tree; free it once its process ends
	cols   uint16
	rows   uint16
}

// startPane runs argv with exactly env and starts a goroutine that feeds the
// process's output to the pane's terminal.
func (s *Server) startPane(id, dir string, argv, env []string, cols, rows uint16) (*pane, error) {
	path, err := lookPath(argv[0], env)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(path, argv[1:]...)
	cmd.Dir = dir
	// A nil Env would inherit the server's environment.
	cmd.Env = append([]string{}, env...)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		return nil, err
	}

	p := &pane{id: id, cmd: cmd, ptmx: ptmx, alive: true, cols: cols, rows: rows}
	// The callback runs with the server's mutex held. A full PTY input buffer
	// could block it; that is unlikely.
	p.term, err = libghostty.NewTerminal(
		libghostty.WithSize(cols, rows),
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
			// The read fails once every holder of the PTY's terminal side has
			// closed it. A background child that keeps it open delays the exit
			// until that child ends; that is unlikely.
			p.cmd.Wait()
			s.mu.Lock()
			p.alive = false
			p.status = exitStatus(p.cmd.ProcessState)
			if p.killed {
				p.free()
			}
			s.mu.Unlock()
			return
		}
	}
}

// hangUp is called once, as the pane leaves the tree, with the server's mutex
// held. It hangs up the pane's process group, which a shell passes on to its
// jobs, and frees the pane once the process has ended. Closing the PTY master
// would not hang up the process, because Close waits for readPane's pending
// Read. A process that ignores SIGHUP keeps running with no pane; that is
// unlikely.
func (p *pane) hangUp() {
	p.killed = true
	if !p.alive {
		p.free()
		return
	}
	syscall.Kill(-p.cmd.Process.Pid, syscall.SIGHUP)
}

// free releases the pane's PTY, terminal, and render state. Exactly one of
// hangUp and readPane calls it, with the server's mutex held, after the
// process has ended and readPane has stopped reading.
func (p *pane) free() {
	p.ptmx.Close()
	p.term.Close()
	p.rs.Close()
}

// exitStatus returns the process's exit code, or 128 plus the signal number if
// a signal killed it.
func exitStatus(state *os.ProcessState) int {
	ws := state.Sys().(syscall.WaitStatus)
	if ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ws.ExitStatus()
}

// resize must be called with the server's mutex held.
func (p *pane) resize(cols, rows uint16) error {
	if err := p.term.Resize(cols, rows, 0, 0); err != nil {
		return err
	}
	if err := pty.Setsize(p.ptmx, &pty.Winsize{Rows: rows, Cols: cols}); err != nil {
		return err
	}
	p.cols, p.rows = cols, rows
	return nil
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
