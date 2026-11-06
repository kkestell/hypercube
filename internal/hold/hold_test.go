package hold

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func temp(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "hc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}
func start(t *testing.T, size *Size, argv ...string) *Holder {
	t.Helper()
	dir := temp(t)
	h, err := Start(filepath.Join(dir, "run"), argv, os.Environ(), dir, size)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Stop(); _ = h.Remove() })
	return h
}
func output(t *testing.T, h *Holder, from int64) *Output {
	t.Helper()
	out, err := h.Output(from)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { out.Close() })
	return out
}
func readN(t *testing.T, in io.Reader, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	done := make(chan error, 1)
	go func() { _, err := io.ReadFull(in, b); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("read timed out")
	}
	return b
}
func until(t *testing.T, fn func() bool) {
	t.Helper()
	end := time.Now().Add(5 * time.Second)
	for time.Now().Before(end) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition timed out")
}
func TestLineOutputReattachesAtPosition(t *testing.T) {
	h := start(t, nil, "/bin/cat")
	out := output(t, h, 0)
	if _, err := h.Write([]byte("first\n")); err != nil {
		t.Fatal(err)
	}
	if got := string(readN(t, out, 6)); got != "first\n" {
		t.Fatal(got)
	}
	out.Close()
	h.Close()
	again, err := Open(h.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if _, err = again.Write([]byte("next\n")); err != nil {
		t.Fatal(err)
	}
	if got := string(readN(t, output(t, again, 6), 5)); got != "next\n" {
		t.Fatal(got)
	}
}
func TestSegmentsAndDiscard(t *testing.T) {
	h := start(t, nil, "/bin/cat")
	data := append(bytes.Repeat([]byte("x"), 3*segmentSize+10), '\n')
	go h.Write(data)
	if got := readN(t, output(t, h, 0), len(data)); !bytes.Equal(got, data) {
		t.Fatal("output differs")
	}
	if err := h.Discard(segmentSize + 100); err != nil {
		t.Fatal(err)
	}
	positions, err := h.segments()
	if err != nil || len(positions) != 3 || positions[0] != segmentSize {
		t.Fatalf("segments %v: %v", positions, err)
	}
}
func TestExitIncludesStatusAndStderr(t *testing.T) {
	h := start(t, nil, "/bin/sh", "-c", "printf end; printf old >&2; head -c 3000 /dev/zero | tr '\\0' x >&2; echo final >&2; exit 3")
	b, err := io.ReadAll(output(t, h, 0))
	if err != nil || string(b) != "end" {
		t.Fatalf("%q: %v", b, err)
	}
	e, err := h.Exit()
	if err != nil || e.Code != 3 || e.Status != "exit status 3" || len(e.Stderr) != 2048 || !strings.HasSuffix(e.Stderr, "final\n") {
		t.Fatalf("%+v: %v", e, err)
	}
}

func TestOutputWriteFailureStopsChild(t *testing.T) {
	dir := filepath.Join(temp(t), "run")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	segment := (&Holder{dir: dir}).segment(0)
	if err := syscall.Mkfifo(segment, 0600); err != nil {
		t.Fatal(err)
	}
	h, err := Start(dir, []string{"/usr/bin/yes"}, os.Environ(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Stop(); _ = h.Remove() })
	// Closing the reader makes the holder's next output write fail with EPIPE.
	reader, err := os.Open(segment)
	if err != nil {
		t.Fatal(err)
	}
	readN(t, reader, 1)
	reader.Close()
	until(t, func() bool { return !h.Alive() })
	exit, err := h.Exit()
	if err != nil || exit.Code == 0 || !strings.Contains(exit.Status, "broken pipe") {
		t.Fatalf("exit %+v: %v", exit, err)
	}
}
func TestClosedAndLost(t *testing.T) {
	h := start(t, nil, "/bin/cat")
	out := output(t, h, 0)
	done := make(chan error, 1)
	go func() { _, err := out.Read(make([]byte, 1)); done <- err }()
	out.Close()
	if err := <-done; !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	until(t, func() bool { _, err := os.Stat(filepath.Join(h.dir, "pid")); return err == nil })
	b, _ := os.ReadFile(filepath.Join(h.dir, "pid"))
	pid, _ := strconv.Atoi(string(b))
	_ = syscall.Kill(pid, syscall.SIGKILL)
	until(t, func() bool { return !h.Alive() })
	_, err := output(t, h, 0).Read(make([]byte, 1))
	if !errors.Is(err, ErrLost) {
		t.Fatal(err)
	}
	if _, err = h.Write([]byte("late\n")); err == nil {
		t.Fatal("write reached dead holder")
	}
}
func TestInputDropsPartialLinesAndSerializesWriters(t *testing.T) {
	h := start(t, nil, "/bin/cat")
	conn, err := net.Dial("unix", filepath.Join(h.dir, "in"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(conn, "partial")
	conn.Close()
	a := append(bytes.Repeat([]byte("a"), 8<<20), '\n')
	b := append(bytes.Repeat([]byte("b"), 8<<20), '\n')
	var wg sync.WaitGroup
	for _, p := range [][]byte{a, b} {
		wg.Go(func() {
			c, err := net.Dial("unix", filepath.Join(h.dir, "in"))
			if err != nil {
				t.Error(err)
				return
			}
			defer c.Close()
			if _, err = c.Write(p); err != nil {
				t.Error(err)
			}
		})
	}
	got := readN(t, output(t, h, 0), len(a)+len(b))
	wg.Wait()
	if !bytes.Equal(got, append(bytes.Clone(a), b...)) && !bytes.Equal(got, append(bytes.Clone(b), a...)) {
		t.Fatal("lines merged or partial input delivered")
	}
}
func TestLongSocketPathRefused(t *testing.T) {
	dir := filepath.Join(temp(t), strings.Repeat("x", 104))
	_, err := Start(dir, []string{"cat"}, os.Environ(), "", nil)
	if err == nil || !strings.Contains(err.Error(), dir) {
		t.Fatal(err)
	}
}
func TestStopClosesInput(t *testing.T) {
	h := start(t, nil, "/bin/sh", "-c", "cat; echo closed")
	out := output(t, h, 0)
	_, _ = h.Write([]byte("line\n"))
	readN(t, out, 5)
	_ = h.Stop()
	b, err := io.ReadAll(out)
	if err != nil || string(b) != "closed\n" {
		t.Fatalf("%q: %v", b, err)
	}
}
func TestStopKillsChildIgnoringEOF(t *testing.T) {
	h := start(t, nil, "/bin/sh", "-c", "echo ready; exec sleep 300")
	readN(t, output(t, h, 0), 6)
	began := time.Now()
	_ = h.Stop()
	_, err := io.ReadAll(output(t, h, 6))
	if err != nil {
		t.Fatal(err)
	}
	e, _ := h.Exit()
	if e.Code == 0 || time.Since(began) < 3*time.Second {
		t.Fatalf("exit %+v after %v", e, time.Since(began))
	}
}
func TestTerminalResizeHangupAndRetention(t *testing.T) {
	h := start(t, &Size{Cols: 80, Rows: 24}, "/bin/sh", "-c", "trap 'echo hungup; exit' HUP; stty size; read line; stty size; read line; head -c 700000 /dev/zero | tr '\\0' x; echo done; while :; do sleep 1; done")
	out := output(t, h, 0)
	got := readN(t, out, 7)
	if string(got) != "24 80\r\n" {
		t.Fatal(string(got))
	}
	if err := h.Resize(120, 40); err != nil {
		t.Fatal(err)
	}
	_, _ = h.Write([]byte("\n"))
	got = readN(t, out, 10)
	if !bytes.Contains(got, []byte("40 120")) {
		t.Fatal(string(got))
	}
	// The output that follows rotates the earlier segments away.
	_, _ = h.Write([]byte("\n"))
	until(t, func() bool { p, _ := h.segments(); return len(p) == 2 && p[0] >= segmentSize })
	_ = h.Stop()
	_, err := io.ReadAll(out)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := h.Exit()
	if e.Code != 0 {
		t.Fatalf("hangup exit: %+v", e)
	}
}
func TestSurvivesKilledParent(t *testing.T) {
	dir := filepath.Join(temp(t), "run")
	exe, _ := os.Executable()
	cmd := exec.Command(exe, "parent", dir)
	cmd.Stderr = os.Stderr
	ready, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	readN(t, ready, 6)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	h, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Stop(); _ = h.Remove() })
	if !h.Alive() {
		t.Fatal("holder died with parent")
	}
	_, err = h.Write([]byte("alive\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(readN(t, output(t, h, 0), 6)); got != "alive\n" {
		t.Fatal(got)
	}
}
