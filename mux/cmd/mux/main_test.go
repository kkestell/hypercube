package main

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kkestell/hypercube/mux/client"
	"golang.org/x/sys/unix"
)

var muxBin string

// TestMain builds the mux binary. Run the tests through the mux Makefile, which
// sets PKG_CONFIG_PATH for the build.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "muxbin")
	if err != nil {
		panic(err)
	}
	muxBin = filepath.Join(dir, "mux")
	if out, err := exec.Command("go", "build", "-o", muxBin, ".").CombinedOutput(); err != nil {
		os.Stderr.Write(out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// newState points XDG_STATE_HOME at a fresh directory and, when the test ends,
// stops the server listening there. The directory is outside t.TempDir()
// because a socket path must stay under 104 bytes on macOS. Pane processes die
// when the server exits and the PTY masters close.
func newState(t *testing.T) {
	dir, err := os.MkdirTemp("", "mux")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", dir)
	t.Cleanup(func() {
		// The server has no exit path until task 3.
		if pid, err := serverPID(client.SocketPath()); err == nil {
			syscall.Kill(pid, syscall.SIGTERM)
		}
		os.RemoveAll(dir)
	})
}

// serverPID finds the process listening on the socket from the peer of a
// connection to it.
func serverPID(path string) (int, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	raw, err := conn.(*net.UnixConn).SyscallConn()
	if err != nil {
		return 0, err
	}
	var pid int
	var sockErr error
	err = raw.Control(func(fd uintptr) {
		pid, sockErr = unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
	})
	if err != nil {
		return 0, err
	}
	return pid, sockErr
}

// runMux runs the mux command line against the test's state directory.
func runMux(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command(muxBin, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("mux %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// dial connects to the server. A hung test closes the connection instead of
// blocking.
func dial(t *testing.T) *client.Conn {
	t.Helper()
	c, err := client.Dial(client.SocketPath())
	if err != nil {
		t.Fatal(err)
	}
	stop := time.AfterFunc(10*time.Second, func() { c.Close() })
	t.Cleanup(func() { stop.Stop(); c.Close() })
	return c
}

// startServer runs the server directly and returns a connection to it.
func startServer(t *testing.T) *client.Conn {
	t.Helper()
	newState(t)
	cmd := exec.Command(muxBin, "server")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go cmd.Wait()
	for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if c, err := client.Dial(client.SocketPath()); err == nil {
			c.Close()
			return dial(t)
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not start")
		}
	}
}

// waitCapture polls the pane's screen until it contains every want.
func waitCapture(t *testing.T, c *client.Conn, pane string, want ...string) string {
	t.Helper()
	var text string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		reply, err := c.Call(client.Request{Type: "capture", Pane: pane})
		if err != nil {
			t.Fatal(err)
		}
		text = reply.Text
		missing := false
		for _, w := range want {
			missing = missing || !strings.Contains(text, w)
		}
		if !missing {
			return text
		}
	}
	t.Fatalf("screen never showed %q:\n%s", want, text)
	return ""
}

func TestSpawnCommandStartsServer(t *testing.T) {
	newState(t)
	out := runMux(t, "spawn", "sh", "-c", "echo ready; sleep 60")
	if !regexp.MustCompile(`^s\d+ w\d+ p\d+\n$`).MatchString(out) {
		t.Fatalf("spawn printed %q", out)
	}
	if _, err := os.Stat(client.SocketPath()); err != nil {
		t.Fatal(err)
	}
	pane := strings.Fields(out)[2]
	waitCapture(t, dial(t), pane, "ready")
}

func TestSpawnDirAndEnv(t *testing.T) {
	t.Setenv("MUX_TEST_LEAK", "1")
	c := startServer(t)
	dir, err := filepath.EvalSymlinks(t.TempDir()) // /var is a symlink on macOS
	if err != nil {
		t.Fatal(err)
	}
	reply, err := c.Call(client.Request{
		Type: "spawn",
		Dir:  dir,
		Argv: []string{"sh", "-c", "pwd; stty size; env; echo done"},
		Env:  []string{"PATH=/usr/bin:/bin", "MUX_TEST=hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := waitCapture(t, c, reply.Pane, "done")
	for _, want := range []string{dir, "24 80", "MUX_TEST=hello"} {
		if !strings.Contains(text, want) {
			t.Errorf("screen lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "MUX_TEST_LEAK") {
		t.Errorf("the server's environment leaked into the pane:\n%s", text)
	}
}

func TestAttachSizeAndDetach(t *testing.T) {
	c := startServer(t)
	reply, err := c.Call(client.Request{Type: "spawn", Argv: []string{"sh"}, Env: os.Environ()})
	if err != nil {
		t.Fatal(err)
	}

	a := dial(t)
	if _, err := a.Call(client.Request{Type: "attach", Session: reply.Session, Cols: 100, Rows: 30}); err != nil {
		t.Fatal(err)
	}
	if out, err := a.ReadOutput(); err != nil || len(out.Frame) == 0 {
		t.Fatalf("first output = %+v, %v; want a frame", out, err)
	}
	if err := a.Send(client.Request{Type: "input", Data: []byte("stty size\r")}); err != nil {
		t.Fatal(err)
	}
	waitCapture(t, c, reply.Pane, "30 100")

	if err := a.Send(client.Request{Type: "input", Data: []byte("\x02d")}); err != nil {
		t.Fatal(err)
	}
	for {
		out, err := a.ReadOutput()
		if err != nil {
			t.Fatalf("stream ended before detached: %v", err)
		}
		if out.Detached {
			break
		}
	}
	if out, err := a.ReadOutput(); err == nil {
		t.Fatalf("stream continued after detached: %+v", out)
	}
}

// Palette colors reach the outer terminal as palette colors, so its theme
// shows.
func TestAttachKeepsPaletteColors(t *testing.T) {
	c := startServer(t)
	reply, err := c.Call(client.Request{
		Type: "spawn",
		Argv: []string{"sh", "-c", `printf '\033[31mRED\033[0m'; sleep 60`},
		Env:  []string{"PATH=/usr/bin:/bin"},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitCapture(t, c, reply.Pane, "RED")

	a := dial(t)
	if _, err := a.Call(client.Request{Type: "attach", Session: reply.Session, Cols: 20, Rows: 3}); err != nil {
		t.Fatal(err)
	}
	out, err := a.ReadOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out.Frame), ";31;49mRED") {
		t.Fatalf("frame does not draw RED in palette red: %q", out.Frame)
	}
}

func TestErrors(t *testing.T) {
	c := startServer(t)
	for name, req := range map[string]client.Request{
		"capture of an unknown pane": {Type: "capture", Pane: "p99"},
		"spawn with no command":      {Type: "spawn", Env: os.Environ()},
		"unknown request type":       {Type: "bogus"},
	} {
		if _, err := c.Call(req); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
