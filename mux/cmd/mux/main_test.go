package main

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
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
		// Tests that leave sessions running stop the server here.
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

// runMuxFails runs the mux command line, fails the test if it succeeds, and
// returns its output.
func runMuxFails(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command(muxBin, args...).CombinedOutput()
	if err == nil {
		t.Fatalf("mux %v succeeded:\n%s", args, out)
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

// waitExited polls list until the pane has exited and returns its status.
func waitExited(t *testing.T, c *client.Conn, pane string) int {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		reply, err := c.Call(client.Request{Type: "list"})
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range reply.Sessions {
			for _, w := range s.Windows {
				for _, p := range w.Panes {
					if p.ID == pane && !p.Alive {
						return p.Status
					}
				}
			}
		}
	}
	t.Fatalf("pane %s never exited", pane)
	return 0
}

func spawnPane(t *testing.T, c *client.Conn, req client.Request) client.Reply {
	t.Helper()
	req.Type = "spawn"
	req.Env = []string{"PATH=/usr/bin:/bin"}
	reply, err := c.Call(req)
	if err != nil {
		t.Fatal(err)
	}
	return reply
}

func respawnPane(t *testing.T, c *client.Conn, pane string, argv []string) {
	t.Helper()
	_, err := c.Call(client.Request{Type: "respawn", Pane: pane, Argv: argv, Env: []string{"PATH=/usr/bin:/bin"}})
	if err != nil {
		t.Fatal(err)
	}
}

// panePID reads the PID that a pane started with killable prints.
func panePID(t *testing.T, c *client.Conn, pane string) int {
	t.Helper()
	text := waitCapture(t, c, pane, "pid=")
	pid, err := strconv.Atoi(regexp.MustCompile(`pid=(\d+)`).FindStringSubmatch(text)[1])
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

// waitGone polls until no process has the PID. readPane reaps pane processes,
// so none lingers as a zombie.
func waitGone(t *testing.T, pid int) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if syscall.Kill(pid, 0) == syscall.ESRCH {
			return
		}
	}
	t.Fatalf("process %d never ended", pid)
}

// killable runs a process that the test can find and a kill must end.
var killable = []string{"sh", "-c", "echo pid=$$; exec sleep 60"}

func killTarget(t *testing.T, c *client.Conn, req client.Request) {
	t.Helper()
	req.Type = "kill"
	if _, err := c.Call(req); err != nil {
		t.Fatal(err)
	}
}

// attachAt attaches a new client to the session and reads its first frame.
func attachAt(t *testing.T, session string, cols, rows int) *client.Conn {
	t.Helper()
	a := dial(t)
	if _, err := a.Call(client.Request{Type: "attach", Session: session, Cols: cols, Rows: rows}); err != nil {
		t.Fatal(err)
	}
	if out, err := a.ReadOutput(); err != nil || len(out.Frame) == 0 {
		t.Fatalf("first output = %+v, %v; want a frame", out, err)
	}
	return a
}

// checkSize reads the pane's size by respawning it, which keeps its size, and
// repeats until the size is want. A resize request has no reply, so the size
// may take a few tries to arrive.
func checkSize(t *testing.T, c *client.Conn, pane, want string) {
	t.Helper()
	var text string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		respawnPane(t, c, pane, []string{"sh", "-c", "echo size=$(stty size); exec sleep 60"})
		text = waitCapture(t, c, pane, "size=")
		if slices.Contains(strings.Split(text, "\n"), "size="+want) {
			return
		}
	}
	t.Fatalf("pane %s size never became %s:\n%s", pane, want, text)
}

// waitDetached reads output until the server reports the detach.
func waitDetached(t *testing.T, a *client.Conn) {
	t.Helper()
	for {
		out, err := a.ReadOutput()
		if err != nil {
			t.Fatalf("stream ended before detached: %v", err)
		}
		if out.Detached {
			return
		}
	}
}

func checkList(t *testing.T, c *client.Conn, want []client.Session) {
	t.Helper()
	list, err := c.Call(client.Request{Type: "list"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(list.Sessions, want) {
		t.Errorf("list = %+v, want %+v", list.Sessions, want)
	}
}

func alive(id string) client.Pane { return client.Pane{ID: id, Alive: true} }

func TestSpawnCommandStartsServer(t *testing.T) {
	newState(t)
	out := runMux(t, "spawn", "sh", "-c", "echo ready; sleep 60")
	if !regexp.MustCompile(`^s\d+-[0-9a-f]{4} w\d+-[0-9a-f]{4} p\d+-[0-9a-f]{4}\n$`).MatchString(out) {
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
	waitCapture(t, c, reply.Pane, "28 100")

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

func TestLayoutSizes(t *testing.T) {
	c := startServer(t)
	sleep := client.Request{Argv: []string{"sleep", "60"}}
	main := spawnPane(t, c, sleep)
	sleep.Window = main.Window
	stack := []string{
		spawnPane(t, c, sleep).Pane,
		spawnPane(t, c, sleep).Pane,
		spawnPane(t, c, sleep).Pane,
	}
	sleep.Window = ""
	sleep.Session = main.Session
	second := spawnPane(t, c, sleep)

	a := attachAt(t, main.Session, 100, 30)
	checkSize(t, c, main.Pane, "28 40")
	checkSize(t, c, stack[0], "9 59")
	checkSize(t, c, stack[1], "9 59")
	checkSize(t, c, stack[2], "8 59")
	checkSize(t, c, second.Pane, "28 100")

	if err := a.Send(client.Request{Type: "resize", Cols: 120, Rows: 40}); err != nil {
		t.Fatal(err)
	}
	checkSize(t, c, main.Pane, "38 48")
	checkSize(t, c, stack[2], "12 71")

	if err := a.Send(client.Request{Type: "input", Data: []byte("\x02d")}); err != nil {
		t.Fatal(err)
	}
	waitDetached(t, a)
	checkSize(t, c, main.Pane, "24 80")
	checkSize(t, c, stack[2], "24 80")
	checkSize(t, c, second.Pane, "24 80")
}

func TestLayoutSkipsPanes(t *testing.T) {
	c := startServer(t)
	sleep := client.Request{Argv: []string{"sleep", "60"}}
	main := spawnPane(t, c, sleep)
	sleep.Window = main.Window
	stack := []string{
		spawnPane(t, c, sleep).Pane,
		spawnPane(t, c, sleep).Pane,
		spawnPane(t, c, sleep).Pane,
	}

	attachAt(t, main.Session, 100, 6)
	checkSize(t, c, main.Pane, "4 40")
	checkSize(t, c, stack[0], "2 59")
	checkSize(t, c, stack[1], "1 59")
	checkSize(t, c, stack[2], "24 80")
}

func TestLayoutFollowsSpawnAndKill(t *testing.T) {
	c := startServer(t)
	main := spawnPane(t, c, client.Request{Argv: []string{"sleep", "60"}})
	attachAt(t, main.Session, 100, 30)
	checkSize(t, c, main.Pane, "28 100")

	added := spawnPane(t, c, client.Request{Argv: []string{"sleep", "60"}, Window: main.Window})
	checkSize(t, c, main.Pane, "28 40")
	checkSize(t, c, added.Pane, "28 59")

	killTarget(t, c, client.Request{Pane: added.Pane})
	checkSize(t, c, main.Pane, "28 100")
}

func TestInputGoesToMainPane(t *testing.T) {
	c := startServer(t)
	main := spawnPane(t, c, client.Request{Argv: []string{"cat"}})
	other := spawnPane(t, c, client.Request{Argv: []string{"cat"}, Window: main.Window})
	a := attachAt(t, main.Session, 100, 30)

	if err := a.Send(client.Request{Type: "input", Data: []byte("routed\r")}); err != nil {
		t.Fatal(err)
	}
	waitCapture(t, c, main.Pane, "routed")
	reply, err := c.Call(client.Request{Type: "capture", Pane: other.Pane})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(reply.Text, "routed") {
		t.Errorf("the other pane received the input:\n%s", reply.Text)
	}

	killTarget(t, c, client.Request{Pane: main.Pane})
	if err := a.Send(client.Request{Type: "input", Data: []byte("second\r")}); err != nil {
		t.Fatal(err)
	}
	waitCapture(t, c, other.Pane, "second")
}

func TestCursorAndOffsets(t *testing.T) {
	c := startServer(t)
	main := spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", "printf abc; exec sleep 60"}})
	spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", "printf STACK; exec sleep 60"}, Window: main.Window})
	a := attachAt(t, main.Session, 100, 30)

	for {
		out, err := a.ReadOutput()
		if err != nil {
			t.Fatalf("stream ended before both panes showed: %v", err)
		}
		frame := out.Frame
		if !bytes.Contains(frame, []byte("abc")) || !bytes.Contains(frame, []byte("STACK")) {
			continue
		}
		if !bytes.Contains(frame, []byte("\x1b[2;42H")) {
			t.Errorf("frame does not start the stacked pane at row 3, column 42: %q", frame)
		}
		if !bytes.HasSuffix(frame, []byte("\x1b[2;4H\x1b[?25h\x1b[?2026l")) {
			t.Errorf("frame does not end with the main pane's cursor: %q", frame)
		}
		return
	}
}

// waitFrame reads frames until one contains every want, failing if the stream
// ends first. It returns that frame.
func waitFrame(t *testing.T, a *client.Conn, want ...string) []byte {
	t.Helper()
	for {
		out, err := a.ReadOutput()
		if err != nil {
			t.Fatalf("stream ended before a frame contained %q: %v", want, err)
		}
		found := true
		for _, w := range want {
			found = found && bytes.Contains(out.Frame, []byte(w))
		}
		if found {
			return out.Frame
		}
	}
}

const setTitle = `printf "\033]0;%s\007" `

func TestTitleLabels(t *testing.T) {
	c := startServer(t)
	pane := spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", setTitle + "hello; exec sleep 60"}})
	a := attachAt(t, pane.Session, 100, 30)
	waitFrame(t, a,
		"\x1b[30;94H\x1b[7m hello \x1b[0m",
		"\x1b[30;1H\x1b[7m 0 hello \x1b[0m",
		"\x1b[1;1H─\x1b[7m hello \x1b[0m─")
}

func TestCommandNameLabels(t *testing.T) {
	c := startServer(t)
	one := spawnPane(t, c, client.Request{Argv: []string{"sleep", "60"}})
	spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", setTitle + "other; exec sleep 60"}})
	spawnPane(t, c, client.Request{Argv: []string{"sleep", "60"}, Window: one.Window})
	a := attachAt(t, one.Session, 100, 30)
	waitFrame(t, a,
		"\x1b[30;87H\x1b[7m sleep \x1b[0m other ",
		"\x1b[1;42H─ sleep ─")
}

func TestExitedTitle(t *testing.T) {
	c := startServer(t)
	pane := spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", "exit 3"}})
	a := attachAt(t, pane.Session, 100, 30)
	waitFrame(t, a, "\x1b[1;1H─\x1b[7m sh (exited 3) \x1b[0m─")
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
	if _, err := a.Call(client.Request{Type: "attach", Session: reply.Session, Cols: 20, Rows: 5}); err != nil {
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

func TestExitStatus(t *testing.T) {
	c := startServer(t)
	exits := spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", "echo bye; exit 3"}})
	killed := spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", "kill -TERM $$"}})
	running := spawnPane(t, c, client.Request{Argv: []string{"sleep", "60"}})

	if got := waitExited(t, c, exits.Pane); got != 3 {
		t.Errorf("exit status = %d, want 3", got)
	}
	waitCapture(t, c, exits.Pane, "bye")
	if got := waitExited(t, c, killed.Pane); got != 143 {
		t.Errorf("status after SIGTERM = %d, want 143", got)
	}
	list, err := c.Call(client.Request{Type: "list"})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.ID == running.Pane && !p.Alive {
					t.Error("sleep 60 is not alive")
				}
			}
		}
	}
}

func TestSpawnIntoSessionAndWindow(t *testing.T) {
	c := startServer(t)
	sleep := client.Request{Argv: []string{"sleep", "60"}}
	first := spawnPane(t, c, sleep)
	sleep.Window = first.Window
	second := spawnPane(t, c, sleep)
	sleep.Window = ""
	sleep.Session = first.Session
	third := spawnPane(t, c, sleep)

	if second.Session != first.Session || second.Window != first.Window || second.Pane == first.Pane {
		t.Errorf("pane into a window: %+v after %+v", second, first)
	}
	if third.Session != first.Session || third.Window == first.Window {
		t.Errorf("window into a session: %+v after %+v", third, first)
	}
	list, err := c.Call(client.Request{Type: "list"})
	if err != nil {
		t.Fatal(err)
	}
	want := []client.Session{{ID: first.Session, Windows: []client.Window{
		{ID: first.Window, Panes: []client.Pane{{ID: first.Pane, Alive: true}, {ID: second.Pane, Alive: true}}},
		{ID: third.Window, Panes: []client.Pane{{ID: third.Pane, Alive: true}}},
	}}}
	if !reflect.DeepEqual(list.Sessions, want) {
		t.Errorf("list = %+v, want %+v", list.Sessions, want)
	}
}

func TestLsCommand(t *testing.T) {
	newState(t)
	dir, err := filepath.EvalSymlinks(t.TempDir()) // /var is a symlink on macOS
	if err != nil {
		t.Fatal(err)
	}
	first := strings.Fields(runMux(t, "spawn", "sleep", "60"))
	second := strings.Fields(runMux(t, "spawn", "-w", first[1], "-c", dir, "sh", "-c", "pwd; exit 3"))
	if second[0] != first[0] || second[1] != first[1] {
		t.Fatalf("second spawn printed %v after %v", second, first)
	}
	waitCapture(t, dial(t), second[2], dir)

	want := strings.Join([]string{
		first[0] + " " + first[1] + " " + first[2] + " running",
		second[0] + " " + second[1] + " " + second[2] + " exited 3",
	}, "\n") + "\n"
	var out string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if out = runMux(t, "ls"); out == want {
			return
		}
	}
	t.Fatalf("mux ls printed %q, want %q", out, want)
}

func TestKillPane(t *testing.T) {
	c := startServer(t)
	first := spawnPane(t, c, client.Request{Argv: killable})
	second := spawnPane(t, c, client.Request{Argv: killable, Window: first.Window})
	done := spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", "exit 3"}, Window: first.Window})
	pid := panePID(t, c, first.Pane)
	waitExited(t, c, done.Pane)

	killTarget(t, c, client.Request{Pane: first.Pane})
	waitGone(t, pid)
	checkList(t, c, []client.Session{{ID: first.Session, Windows: []client.Window{
		{ID: first.Window, Panes: []client.Pane{alive(second.Pane), {ID: done.Pane, Status: 3}}},
	}}})
	if _, err := c.Call(client.Request{Type: "capture", Pane: first.Pane}); err == nil {
		t.Error("capture of a killed pane succeeded")
	}

	killTarget(t, c, client.Request{Pane: done.Pane})
	checkList(t, c, []client.Session{{ID: first.Session, Windows: []client.Window{
		{ID: first.Window, Panes: []client.Pane{alive(second.Pane)}},
	}}})
}

func TestKillWindow(t *testing.T) {
	c := startServer(t)
	w1 := spawnPane(t, c, client.Request{Argv: killable})
	w2 := spawnPane(t, c, client.Request{Argv: killable, Session: w1.Session})
	other := spawnPane(t, c, client.Request{Argv: killable})
	pid1 := panePID(t, c, w1.Pane)
	pid2 := panePID(t, c, w2.Pane)

	killTarget(t, c, client.Request{Window: w1.Window})
	waitGone(t, pid1)
	checkList(t, c, []client.Session{
		{ID: w1.Session, Windows: []client.Window{{ID: w2.Window, Panes: []client.Pane{alive(w2.Pane)}}}},
		{ID: other.Session, Windows: []client.Window{{ID: other.Window, Panes: []client.Pane{alive(other.Pane)}}}},
	})

	killTarget(t, c, client.Request{Window: w2.Window})
	waitGone(t, pid2)
	checkList(t, c, []client.Session{
		{ID: other.Session, Windows: []client.Window{{ID: other.Window, Panes: []client.Pane{alive(other.Pane)}}}},
	})
}

func TestKillSessionAndNewIDs(t *testing.T) {
	c := startServer(t)
	w1 := spawnPane(t, c, client.Request{Argv: killable})
	w2 := spawnPane(t, c, client.Request{Argv: killable, Session: w1.Session})
	other := spawnPane(t, c, client.Request{Argv: killable})
	pid1 := panePID(t, c, w1.Pane)
	pid2 := panePID(t, c, w2.Pane)

	killTarget(t, c, client.Request{Session: w1.Session})
	waitGone(t, pid1)
	waitGone(t, pid2)
	checkList(t, c, []client.Session{
		{ID: other.Session, Windows: []client.Window{{ID: other.Window, Panes: []client.Pane{alive(other.Pane)}}}},
	})

	fresh := spawnPane(t, c, client.Request{Argv: killable})
	for _, id := range []string{w1.Session, w1.Window, w1.Pane, w2.Window, w2.Pane} {
		if id == fresh.Session || id == fresh.Window || id == fresh.Pane {
			t.Errorf("new IDs %+v reuse %s", fresh, id)
		}
	}
}

func TestKillEndsAttach(t *testing.T) {
	c := startServer(t)
	a := spawnPane(t, c, client.Request{Argv: killable})
	spawnPane(t, c, client.Request{Argv: killable}) // keeps the server running

	at := dial(t)
	if _, err := at.Call(client.Request{Type: "attach", Session: a.Session, Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	if out, err := at.ReadOutput(); err != nil || len(out.Frame) == 0 {
		t.Fatalf("first output = %+v, %v; want a frame", out, err)
	}

	killTarget(t, c, client.Request{Session: a.Session})
	for {
		if _, err := at.ReadOutput(); err != nil {
			// The server closed the connection; dial's timer would not give EOF.
			if !errors.Is(err, io.EOF) {
				t.Fatalf("stream ended with %v, want EOF", err)
			}
			return
		}
	}
}

func TestKillCommandStopsServer(t *testing.T) {
	newState(t)
	ids := strings.Fields(runMux(t, "spawn", "sleep", "60"))
	session := ids[0]
	pid, err := serverPID(client.SocketPath())
	if err != nil {
		t.Fatal(err)
	}

	if out := runMux(t, "kill", session); out != "" {
		t.Fatalf("mux kill printed %q", out)
	}
	if _, err := os.Stat(client.SocketPath()); !os.IsNotExist(err) {
		t.Errorf("socket after the last kill: %v", err)
	}
	waitGone(t, pid)

	runMux(t, "spawn", "sleep", "60")
	if out := runMuxFails(t, "kill", ids[2]); !strings.Contains(out, "unknown") {
		t.Errorf("kill of the first server's pane printed %q, want unknown", out)
	}
}

func TestNoServer(t *testing.T) {
	newState(t)
	for _, args := range [][]string{
		{"ls"},
		{"kill", "p1-0000"},
		{"label", "p1-0000", "text"},
		{"attention", "p1-0000", "on"},
		{"collapse", "p1-0000", "on"},
		{"select", "p1-0000"},
		{"attach", "s1-0000"},
	} {
		if out := runMuxFails(t, args...); !strings.Contains(out, "no server is running") {
			t.Errorf("mux %v printed %q, want no server is running", args, out)
		}
	}
	if _, err := os.Stat(client.SocketPath()); !os.IsNotExist(err) {
		t.Errorf("socket after commands that only dial: %v", err)
	}
}

func TestRespawnDeadPane(t *testing.T) {
	c := startServer(t)
	a := spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", "echo old; exit 3"}})
	b := spawnPane(t, c, client.Request{Window: a.Window, Argv: []string{"sleep", "60"}})
	waitExited(t, c, a.Pane)

	respawnPane(t, c, a.Pane, []string{"sh", "-c", "echo new; exec sleep 60"})
	if text := waitCapture(t, c, a.Pane, "new"); strings.Contains(text, "old") {
		t.Errorf("screen after respawn still has the old output:\n%s", text)
	}
	checkList(t, c, []client.Session{{ID: a.Session, Windows: []client.Window{
		{ID: a.Window, Panes: []client.Pane{alive(a.Pane), alive(b.Pane)}},
	}}})
}

func TestRespawnLivePane(t *testing.T) {
	c := startServer(t)
	a := spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", "echo pid=$$; echo old; exec sleep 60"}})
	pid := panePID(t, c, a.Pane)

	respawnPane(t, c, a.Pane, []string{"sh", "-c", "echo new; exec sleep 60"})
	waitGone(t, pid)
	text := waitCapture(t, c, a.Pane, "new")
	if strings.Contains(text, "old") || strings.Contains(text, strconv.Itoa(pid)) {
		t.Errorf("screen after respawn still has the old output:\n%s", text)
	}
	checkList(t, c, []client.Session{{ID: a.Session, Windows: []client.Window{
		{ID: a.Window, Panes: []client.Pane{alive(a.Pane)}},
	}}})
}

func TestRespawnAttached(t *testing.T) {
	c := startServer(t)
	a := spawnPane(t, c, client.Request{Argv: killable})

	at := dial(t)
	if _, err := at.Call(client.Request{Type: "attach", Session: a.Session, Cols: 100, Rows: 30}); err != nil {
		t.Fatal(err)
	}
	if out, err := at.ReadOutput(); err != nil || len(out.Frame) == 0 {
		t.Fatalf("first output = %+v, %v; want a frame", out, err)
	}

	respawnPane(t, c, a.Pane, []string{"sh", "-c", "stty size; exec sleep 60"})
	for {
		out, err := at.ReadOutput()
		if err != nil {
			t.Fatalf("stream ended before the new pane's screen: %v", err)
		}
		if bytes.Contains(out.Frame, []byte("28 100")) {
			return
		}
	}
}

func TestErrors(t *testing.T) {
	c := startServer(t)
	a := spawnPane(t, c, client.Request{Argv: []string{"sleep", "60"}})
	b := spawnPane(t, c, client.Request{Argv: []string{"sleep", "60"}})
	sleep := []string{"sleep", "60"}
	env := []string{"PATH=/usr/bin:/bin"}
	// Each spawn has a valid command and PATH, so only its IDs can fail it.
	for name, req := range map[string]client.Request{
		"capture of an unknown pane":          {Type: "capture", Pane: "p99"},
		"spawn with no command":               {Type: "spawn", Env: os.Environ()},
		"unknown request type":                {Type: "bogus"},
		"spawn into an unknown session":       {Type: "spawn", Session: "s99", Argv: sleep, Env: env},
		"spawn into an unknown window":        {Type: "spawn", Window: "w99", Argv: sleep, Env: env},
		"spawn into another session's window": {Type: "spawn", Session: a.Session, Window: b.Window, Argv: sleep, Env: env},
		"kill with no target":                 {Type: "kill"},
		"kill with two targets":               {Type: "kill", Pane: a.Pane, Window: a.Window},
		"kill of an unknown pane":             {Type: "kill", Pane: "p99"},
		"kill of an unknown window":           {Type: "kill", Window: "w99"},
		"kill of an unknown session":          {Type: "kill", Session: "s99"},
		"attention with a window":             {Type: "attention", Window: a.Window, On: true},
		"attention with a session":            {Type: "attention", Session: a.Session, On: true},
		"select with a session":               {Type: "select", Session: a.Session},
		"select with no target":               {Type: "select"},
		"select with a window and a pane":     {Type: "select", Window: a.Window, Pane: a.Pane},
		"select of an unknown pane":           {Type: "select", Pane: "p99"},
		"respawn with no command":             {Type: "respawn", Pane: a.Pane, Env: env},
		"respawn of an unknown pane":          {Type: "respawn", Pane: "p99", Argv: sleep, Env: env},
		"respawn with a command not in PATH":  {Type: "respawn", Pane: a.Pane, Argv: []string{"no-such-command"}, Env: env},
	} {
		if _, err := c.Call(req); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	// The failed respawn left the pane's old process in place.
	if _, err := c.Call(client.Request{Type: "capture", Pane: a.Pane}); err != nil {
		t.Errorf("capture after the failed respawn: %v", err)
	}
	checkList(t, c, []client.Session{
		{ID: a.Session, Windows: []client.Window{{ID: a.Window, Panes: []client.Pane{alive(a.Pane)}}}},
		{ID: b.Session, Windows: []client.Window{{ID: b.Window, Panes: []client.Pane{alive(b.Pane)}}}},
	})
}

func sendInput(t *testing.T, a *client.Conn, data string) {
	t.Helper()
	if err := a.Send(client.Request{Type: "input", Data: []byte(data)}); err != nil {
		t.Fatal(err)
	}
}

// titled spawns a pane that sets its title and then waits.
func titled(title string) client.Request {
	return client.Request{Argv: []string{"sh", "-c", setTitle + title + "; echo shown-" + title + "; exec sleep 60"}}
}

func TestWindowKeys(t *testing.T) {
	c := startServer(t)
	one := spawnPane(t, c, titled("one"))
	two := titled("two")
	two.Session = one.Session
	spawnPane(t, c, two)
	a := attachAt(t, one.Session, 100, 30)

	const first = "\x1b[30;1H\x1b[7m 0 one \x1b[0m 1 two "
	const second = "\x1b[30;1H 0 one \x1b[7m 1 two \x1b[0m"
	waitFrame(t, a, first)
	sendInput(t, a, "\x02n")
	waitFrame(t, a, second)
	sendInput(t, a, "\x02n")
	waitFrame(t, a, first)
	sendInput(t, a, "\x02p")
	waitFrame(t, a, second)
	sendInput(t, a, "\x020")
	waitFrame(t, a, first)
	sendInput(t, a, "\x029") // no such window
	sendInput(t, a, "\x02n")
	waitFrame(t, a, second)
}

func TestNextPaneKey(t *testing.T) {
	c := startServer(t)
	main := spawnPane(t, c, client.Request{Argv: []string{"cat"}})
	other := spawnPane(t, c, client.Request{Argv: []string{"cat"}, Window: main.Window})
	a := attachAt(t, main.Session, 100, 30)

	sendInput(t, a, "\x02o")
	waitFrame(t, a, "\x1b[1;42H─\x1b[7m cat \x1b[0m─")
	sendInput(t, a, "routed\r")
	waitCapture(t, c, other.Pane, "routed")
	reply, err := c.Call(client.Request{Type: "capture", Pane: main.Pane})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(reply.Text, "routed") {
		t.Errorf("the main pane received the input:\n%s", reply.Text)
	}

	// Input after a command in the same read goes to the newly focused pane.
	sendInput(t, a, "\x02oback\r")
	waitCapture(t, c, main.Pane, "back")
}

func TestLiteralPrefix(t *testing.T) {
	c := startServer(t)
	pane := spawnPane(t, c, client.Request{Argv: []string{"cat", "-v"}})
	a := attachAt(t, pane.Session, 100, 30)

	sendInput(t, a, "\x02\x02\r")
	waitCapture(t, c, pane.Pane, "^B")
}

func TestPrefixInKeyboardModes(t *testing.T) {
	c := startServer(t)
	pane := spawnPane(t, c, client.Request{Argv: []string{"sleep", "60"}})

	// kitty, with the release of the prefix between it and the command
	a := attachAt(t, pane.Session, 100, 30)
	sendInput(t, a, "\x1b[98;5u\x1b[98;5:3ud")
	waitDetached(t, a)

	// modifyOtherKeys
	a = attachAt(t, pane.Session, 100, 30)
	sendInput(t, a, "\x1b[27;5;98~d")
	waitDetached(t, a)
}

func TestKeyboardModeFollowsFocus(t *testing.T) {
	c := startServer(t)
	main := spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", `printf "\033[?1h\033=\033[?2004h\033[>1u\033[>4;2m"; exec sleep 60`}})
	spawnPane(t, c, client.Request{Argv: []string{"sleep", "60"}, Window: main.Window})
	a := attachAt(t, main.Session, 100, 30)

	waitFrame(t, a, "\x1b[?1h\x1b=\x1b[?2004h\x1b[=1;1u\x1b[>4;2m")
	sendInput(t, a, "\x02o")
	waitFrame(t, a, "\x1b[?1l\x1b>\x1b[?2004l\x1b[=0;1u\x1b[>4;0m")
}

func TestSecondAttachDetachesFirst(t *testing.T) {
	c := startServer(t)
	pane := spawnPane(t, c, client.Request{Argv: []string{"cat"}})
	first := attachAt(t, pane.Session, 100, 30)
	second := attachAt(t, pane.Session, 100, 30)

	for {
		if _, err := first.ReadOutput(); err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatalf("first client's stream ended with %v, want EOF", err)
			}
			break
		}
	}
	sendInput(t, second, "hello\r")
	waitCapture(t, c, pane.Pane, "hello")
}

func TestReattachRedraws(t *testing.T) {
	c := startServer(t)
	one := spawnPane(t, c, titled("one"))
	two := titled("two")
	two.Session = one.Session
	spawnPane(t, c, two)
	a := attachAt(t, one.Session, 100, 30)

	sendInput(t, a, "\x02n")
	waitFrame(t, a, "\x1b[30;1H 0 one \x1b[7m 1 two \x1b[0m")
	sendInput(t, a, "\x02d")
	waitDetached(t, a)

	b := dial(t)
	if _, err := b.Call(client.Request{Type: "attach", Session: one.Session, Cols: 100, Rows: 30}); err != nil {
		t.Fatal(err)
	}
	waitFrame(t, b, "\x1b[30;1H 0 one \x1b[7m 1 two \x1b[0m", "shown-two")
}

func TestScrollbackKeepsLines(t *testing.T) {
	c := startServer(t)
	pane := spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", "seq 20000; exec sleep 60"}})

	text := waitCapture(t, c, pane.Pane, "20000")
	first, err := strconv.Atoi(strings.SplitN(text, "\n", 2)[0])
	if err != nil || first < 9001 || first > 11000 {
		t.Errorf("first line = %q, want a number from 9001 to 11000", strings.SplitN(text, "\n", 2)[0])
	}
}

func TestWheelScrolls(t *testing.T) {
	c := startServer(t)
	pane := spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", "seq 100; exec cat"}})
	a := attachAt(t, pane.Session, 100, 30)

	waitFrame(t, a, "\x1b[2;1H\x1b[0;39;49m74")
	sendInput(t, a, "\x1b[<64;10;10M")
	waitFrame(t, a, "\x1b[2;1H\x1b[0;39;49m71")
	sendInput(t, a, "x")
	waitFrame(t, a, "\x1b[2;1H\x1b[0;39;49m74")
}

func TestMouseGoesToPane(t *testing.T) {
	c := startServer(t)
	pane := spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", `printf "\033[?1000h\033[?1006h"; echo ready; exec cat`}})
	a := attachAt(t, pane.Session, 100, 30)
	waitCapture(t, c, pane.Pane, "ready")

	// Column 10 of terminal row 3 is row 2 of the pane's screen.
	sendInput(t, a, "\x1b[<0;10;3M")
	waitCapture(t, c, pane.Pane, "[<0;10;2M")
}

func TestDragCopies(t *testing.T) {
	c := startServer(t)
	pane := spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", "echo hello world; exec sleep 60"}})
	a := attachAt(t, pane.Session, 100, 30)

	waitFrame(t, a, "hello world")
	sendInput(t, a, "\x1b[<0;1;2M\x1b[<32;5;2M\x1b[<0;5;2m")
	waitFrame(t, a, "\x1b]52;c;aGVsbG8=\x07")
}

func TestClickSelects(t *testing.T) {
	c := startServer(t)
	one := spawnPane(t, c, titled("one"))
	two := titled("two")
	two.Session = one.Session
	second := spawnPane(t, c, two)
	stacked := spawnPane(t, c, client.Request{Argv: []string{"cat"}, Window: second.Window})
	a := attachAt(t, one.Session, 100, 30)

	sendInput(t, a, "\x1b[<0;10;30M\x1b[<0;10;30m")
	waitFrame(t, a, "\x1b[30;1H 0 one \x1b[7m 1 two \x1b[0m")
	sendInput(t, a, "\x1b[<0;60;10M\x1b[<0;60;10m")
	waitFrame(t, a, "\x1b[1;42H─\x1b[7m cat \x1b[0m─")
	sendInput(t, a, "typed\r")
	waitCapture(t, c, stacked.Pane, "typed")
}

func TestDragDivider(t *testing.T) {
	c := startServer(t)
	main := spawnPane(t, c, client.Request{Argv: []string{"sleep", "60"}})
	stacked := spawnPane(t, c, client.Request{Argv: []string{"sleep", "60"}, Window: main.Window})
	a := attachAt(t, main.Session, 100, 30)
	checkSize(t, c, main.Pane, "28 40")

	// The divider is column 41. Drag it to column 61.
	sendInput(t, a, "\x1b[<0;41;5M\x1b[<32;50;5M\x1b[<32;61;5M\x1b[<0;61;5m")
	checkSize(t, c, main.Pane, "28 60")
	checkSize(t, c, stacked.Pane, "28 39")
}

func TestClickCollapses(t *testing.T) {
	c := startServer(t)
	main := spawnPane(t, c, client.Request{Argv: []string{"sleep", "60"}})
	first := spawnPane(t, c, client.Request{Argv: []string{"cat"}, Window: main.Window})
	second := spawnPane(t, c, client.Request{Argv: []string{"cat"}, Window: main.Window})
	a := attachAt(t, main.Session, 100, 30)
	checkSize(t, c, second.Pane, "13 59")

	// The first stacked pane's title is on row 1; collapsing it gives its
	// screen to the second pane, whose title moves up to row 2.
	sendInput(t, a, "\x1b[<0;60;1M\x1b[<0;60;1m")
	waitFrame(t, a, "\x1b[2;42H─ sh ─") // checkSize respawned it as sh
	checkSize(t, c, second.Pane, "27 59")

	// Ctrl-b o skips the collapsed pane.
	sendInput(t, a, "\x02o")
	sendInput(t, a, "typed\r")
	waitCapture(t, c, second.Pane, "typed")

	// Opening it again restores both sizes and focuses it.
	sendInput(t, a, "\x1b[<0;60;1M\x1b[<0;60;1m")
	waitFrame(t, a, "\x1b[1;42H─\x1b[7m cat \x1b[0m─")
	checkSize(t, c, first.Pane, "14 59")
	checkSize(t, c, second.Pane, "13 59")
}

func TestAttention(t *testing.T) {
	c := startServer(t)
	one := spawnPane(t, c, titled("one"))
	two := titled("two")
	two.Window = one.Window
	second := spawnPane(t, c, two)
	a := attachAt(t, one.Session, 100, 30)
	waitFrame(t, a, "shown-one")

	mark := func(pane string, on bool) {
		t.Helper()
		if _, err := c.Call(client.Request{Type: "attention", Pane: pane, On: on}); err != nil {
			t.Fatal(err)
		}
	}
	const (
		windowBell  = "\x1b[30;1H\x1b[7m 0 \U000F009A one \x1b[0m"
		sessionBell = "\x1b[30;94H\x1b[7m \U000F009A one \x1b[0m"
	)

	// Only the marked pane's title shows the bell.
	mark(one.Pane, true)
	frame := waitFrame(t, a, windowBell, sessionBell, "\x1b[1;1H─\x1b[7m \U000F009A one \x1b[0m─")
	if bytes.Contains(frame, []byte("\U000F009A two")) {
		t.Error("the unmarked pane's title shows a bell")
	}

	// The window and session keep the bell while any pane is marked.
	mark(second.Pane, true)
	mark(one.Pane, false)
	frame = waitFrame(t, a, "\x1b[1;1H─\x1b[7m one \x1b[0m─", "\x1b[1;42H─ \U000F009A two ─")
	if bytes.Contains(frame, []byte("\x1b[30;1H\x1b[7m 0 one")) || bytes.Contains(frame, []byte("\x1b[30;96H")) {
		t.Error("the window or session lost its bell while a pane is marked")
	}

	mark(second.Pane, false)
	waitFrame(t, a, "\x1b[1;42H─ two ─", "\x1b[30;1H\x1b[7m 0 one \x1b[0m", "\x1b[30;96H\x1b[7m one \x1b[0m")

	if _, err := c.Call(client.Request{Type: "attention", Pane: "p99", On: true}); err == nil {
		t.Error("attention on an unknown pane succeeded")
	}
}

func TestSelect(t *testing.T) {
	c := startServer(t)
	one := spawnPane(t, c, titled("one"))
	two := client.Request{Argv: []string{"sh", "-c", setTitle + "two; exec cat"}, Window: one.Window}
	stacked := spawnPane(t, c, two)
	if _, err := c.Call(client.Request{Type: "collapse", Pane: stacked.Pane, On: true}); err != nil {
		t.Fatal(err)
	}
	three := titled("three")
	three.Session = one.Session
	window := spawnPane(t, c, three)
	a := attachAt(t, one.Session, 100, 30)

	runMux(t, "select", window.Window)
	waitFrame(t, a, "\x1b[30;1H 0 one \x1b[7m 1 three \x1b[0m")

	// The pane's window becomes current, and the collapsed pane opens and
	// takes the keys.
	runMux(t, "select", stacked.Pane)
	waitFrame(t, a, "\x1b[30;1H\x1b[7m 0 one \x1b[0m 1 three", "\x1b[1;42H─\x1b[7m two \x1b[0m─")
	sendInput(t, a, "typed\r")
	waitCapture(t, c, stacked.Pane, "typed")
}

func TestCollapse(t *testing.T) {
	c := startServer(t)
	main := spawnPane(t, c, client.Request{Argv: []string{"sleep", "60"}})
	first := spawnPane(t, c, client.Request{Argv: []string{"sleep", "60"}, Window: main.Window})
	second := spawnPane(t, c, client.Request{Argv: []string{"sleep", "60"}, Window: main.Window})
	attachAt(t, main.Session, 100, 30)

	if _, err := c.Call(client.Request{Type: "collapse", Pane: first.Pane, On: true}); err != nil {
		t.Fatal(err)
	}
	checkSize(t, c, second.Pane, "27 59")
	if _, err := c.Call(client.Request{Type: "collapse", Pane: first.Pane}); err != nil {
		t.Fatal(err)
	}
	checkSize(t, c, second.Pane, "13 59")

	for _, pane := range []string{main.Pane, "p99"} {
		if _, err := c.Call(client.Request{Type: "collapse", Pane: pane, On: true}); err == nil {
			t.Errorf("collapsing %s succeeded", pane)
		}
	}
}

func TestProgress(t *testing.T) {
	c := startServer(t)
	pane := spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", setTitle + `one; printf "\033]9;4;1;42\007"; read x; printf "\033]9;4;3\007"; read x; printf "\033]9;4;0\007"; exec cat`}})
	a := attachAt(t, pane.Session, 100, 30)

	waitFrame(t, a, "\x1b[1;1H─\x1b[7m \U000F0AA1 42% one \x1b[0m─")
	sendInput(t, a, "\r")
	waitFrame(t, a, "\x1b[1;1H─\x1b[7m \U000F051F one \x1b[0m─")
	sendInput(t, a, "\r")
	waitFrame(t, a, "\x1b[1;1H─\x1b[7m one \x1b[0m─")
}

func TestBellAndTitle(t *testing.T) {
	c := startServer(t)
	pane := spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", setTitle + `one; read x; printf "\007"; exec cat`}})
	a := attachAt(t, pane.Session, 100, 30)
	waitFrame(t, a, "\x1b]2;one | one | one\x07")

	if _, err := c.Call(client.Request{Type: "label", Session: pane.Session, Spans: []client.Span{{Text: "work"}}}); err != nil {
		t.Fatal(err)
	}
	waitFrame(t, a, "\x1b]2;work | one | one\x07")

	if _, err := c.Call(client.Request{Type: "attention", Pane: pane.Pane, On: true}); err != nil {
		t.Fatal(err)
	}
	waitFrame(t, a, "\x07\x1b[?2026h")

	sendInput(t, a, "\r")
	waitFrame(t, a, "\x07\x1b[?2026h")
}

func TestLabel(t *testing.T) {
	c := startServer(t)
	pane := spawnPane(t, c, titled("one"))
	a := attachAt(t, pane.Session, 100, 30)
	waitFrame(t, a, "shown-one")

	set := func(req client.Request, text string) {
		t.Helper()
		req.Type = "label"
		if text != "" {
			req.Spans = []client.Span{{Text: text}}
		}
		if _, err := c.Call(req); err != nil {
			t.Fatal(err)
		}
	}
	set(client.Request{Pane: pane.Pane}, "agent")
	waitFrame(t, a,
		"\x1b[30;94H\x1b[7m agent \x1b[0m",
		"\x1b[30;1H\x1b[7m 0 agent \x1b[0m",
		"\x1b[1;1H─\x1b[7m agent \x1b[0m─")

	set(client.Request{Session: pane.Session}, "work")
	set(client.Request{Window: pane.Window}, "team")
	waitFrame(t, a,
		"\x1b[30;95H\x1b[7m work \x1b[0m",
		"\x1b[30;1H\x1b[7m 0 team \x1b[0m",
		"\x1b[1;1H─\x1b[7m agent \x1b[0m─")

	set(client.Request{Pane: pane.Pane}, "")
	waitFrame(t, a, "\x1b[1;1H─\x1b[7m one \x1b[0m─")

	if _, err := c.Call(client.Request{Type: "label", Window: "w99"}); err == nil {
		t.Error("labeling an unknown window succeeded")
	}
}

func TestClickSession(t *testing.T) {
	c := startServer(t)
	one := spawnPane(t, c, titled("one"))
	spawnPane(t, c, titled("two"))
	a := attachAt(t, one.Session, 100, 30)

	sendInput(t, a, "\x1b[<0;97;30M\x1b[<0;97;30m")
	waitFrame(t, a, "\x1b[30;91H one \x1b[7m two \x1b[0m")
	checkSize(t, c, one.Pane, "24 80")
}
