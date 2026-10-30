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
	waitCapture(t, c, reply.Pane, "27 100")

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
	checkSize(t, c, main.Pane, "27 40")
	checkSize(t, c, stack[0], "9 59")
	checkSize(t, c, stack[1], "8 59")
	checkSize(t, c, stack[2], "8 59")
	checkSize(t, c, second.Pane, "27 100")

	if err := a.Send(client.Request{Type: "resize", Cols: 120, Rows: 40}); err != nil {
		t.Fatal(err)
	}
	checkSize(t, c, main.Pane, "37 48")
	checkSize(t, c, stack[2], "11 71")

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

	attachAt(t, main.Session, 100, 7)
	checkSize(t, c, main.Pane, "4 40")
	checkSize(t, c, stack[0], "2 59")
	checkSize(t, c, stack[1], "1 59")
	checkSize(t, c, stack[2], "24 80")
}

func TestLayoutFollowsSpawnAndKill(t *testing.T) {
	c := startServer(t)
	main := spawnPane(t, c, client.Request{Argv: []string{"sleep", "60"}})
	attachAt(t, main.Session, 100, 30)
	checkSize(t, c, main.Pane, "27 100")

	added := spawnPane(t, c, client.Request{Argv: []string{"sleep", "60"}, Window: main.Window})
	checkSize(t, c, main.Pane, "27 40")
	checkSize(t, c, added.Pane, "27 59")

	killTarget(t, c, client.Request{Pane: added.Pane})
	checkSize(t, c, main.Pane, "27 100")
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
		if !bytes.Contains(frame, []byte("\x1b[4;42H")) {
			t.Errorf("frame does not start the stacked pane at row 4, column 42: %q", frame)
		}
		if !bytes.HasSuffix(frame, []byte("\x1b[4;4H\x1b[?25h\x1b[?2026l")) {
			t.Errorf("frame does not end with the main pane's cursor: %q", frame)
		}
		return
	}
}

// waitFrame reads frames until one contains every want, failing if the stream
// ends first.
func waitFrame(t *testing.T, a *client.Conn, want ...string) {
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
			return
		}
	}
}

const setTitle = `printf "\033]0;%s\007" `

func TestTitleLabels(t *testing.T) {
	c := startServer(t)
	pane := spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", setTitle + "hello; exec sleep 60"}})
	a := attachAt(t, pane.Session, 100, 30)
	waitFrame(t, a,
		"\x1b[1;1H\x1b[7m hello \x1b[0m",
		"\x1b[2;1H\x1b[7m 0 hello \x1b[0m",
		"\x1b[3;1H\x1b[7m hello \x1b[0m")
}

func TestCommandNameLabels(t *testing.T) {
	c := startServer(t)
	one := spawnPane(t, c, client.Request{Argv: []string{"sleep", "60"}})
	spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", setTitle + "other; exec sleep 60"}})
	spawnPane(t, c, client.Request{Argv: []string{"sleep", "60"}, Window: one.Window})
	a := attachAt(t, one.Session, 100, 30)
	waitFrame(t, a,
		"\x1b[1;1H\x1b[7m sleep \x1b[0m other ",
		"\x1b[3;42H sleep ")
}

func TestExitedTitle(t *testing.T) {
	c := startServer(t)
	pane := spawnPane(t, c, client.Request{Argv: []string{"sh", "-c", "exit 3"}})
	a := attachAt(t, pane.Session, 100, 30)
	waitFrame(t, a, "\x1b[3;1H\x1b[7m sh (exited 3) \x1b[0m")
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
	session := strings.Fields(runMux(t, "spawn", "sleep", "60"))[0]
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

	if out := runMux(t, "spawn", "sleep", "60"); out != "s1 w1 p1\n" {
		t.Errorf("spawn after the server exited printed %q, want a new server's s1 w1 p1", out)
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
		if bytes.Contains(out.Frame, []byte("27 100")) {
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
