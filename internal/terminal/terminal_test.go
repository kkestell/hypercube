package terminal

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/hold"
)

func newSet(t *testing.T, command ...string) *Set {
	t.Helper()
	dir, err := os.MkdirTemp("", "hc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	s := &Set{Dir: dir, Command: command}
	t.Cleanup(s.Close)
	return s
}
func read(t *testing.T, out io.Reader, want string) string {
	t.Helper()
	result := make(chan string, 1)
	go func() {
		var got strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := out.Read(buf)
			got.Write(buf[:n])
			if strings.Contains(got.String(), want) || err != nil {
				result <- got.String()
				return
			}
		}
	}()
	select {
	case got := <-result:
		if !strings.Contains(got, want) {
			t.Fatalf("no %q in %q", want, got)
		}
		return got
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out reading %q", want)
	}
	return ""
}
func output(t *testing.T, sh *Shell) *hold.Output {
	t.Helper()
	out, err := sh.Output()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { out.Close() })
	return out
}

// exitCode waits for the shell to end and returns its exit code.
func exitCode(t *testing.T, sh *Shell) int {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(25 * time.Millisecond) {
		if code := sh.ExitCode(); code != -1 {
			return code
		}
	}
	t.Fatal("shell did not end")
	return 0
}

// gone reports whether the session's shell holder has been removed.
func gone(s *Set, session string) bool {
	_, err := os.Stat(s.dir(session))
	return errors.Is(err, os.ErrNotExist)
}
func TestInputOutputAndAdopt(t *testing.T) {
	next := newSet(t, "/bin/cat")
	// The first set belongs to an app that exits without ending its shells.
	set := &Set{Dir: next.Dir, Command: next.Command}
	sh, err := set.Open("s", t.TempDir(), 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	out := output(t, sh)
	if err = sh.Write([]byte("before\n")); err != nil {
		t.Fatal(err)
	}
	read(t, out, "before")
	out.Close()
	if err = next.Adopt("s"); err != nil {
		t.Fatal(err)
	}
	again, err := next.Open("s", t.TempDir(), 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	replay := output(t, again)
	read(t, replay, "before")
	_ = again.Write([]byte("after\n"))
	read(t, replay, "after")
}
func TestStartsInDirectoryAtSize(t *testing.T) {
	set := newSet(t, "/bin/sh", "-c", "pwd -P; stty size; read line; stty size")
	dir := t.TempDir()
	sh, err := set.Open("s", dir, 100, 30)
	if err != nil {
		t.Fatal(err)
	}
	out := output(t, sh)
	got := read(t, out, "30 100")
	if !strings.Contains(got, strings.TrimPrefix(dir, "/private")) {
		t.Fatal(got)
	}
	if err = sh.Resize(120, 40); err != nil {
		t.Fatal(err)
	}
	_ = sh.Write([]byte("\n"))
	read(t, out, "40 120")
	if code := exitCode(t, sh); code != 0 {
		t.Fatal(code)
	}
}
func TestShellExitAndNewShell(t *testing.T) {
	set := newSet(t, "/bin/sh", "-c", "read line; echo bye; exit 3")
	sh, err := set.Open("s", t.TempDir(), 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	out := output(t, sh)
	_ = sh.Write([]byte("\n"))
	read(t, out, "bye")
	if _, err = io.Copy(io.Discard, out); err != nil {
		t.Fatal(err)
	}
	out.Close()
	if code := exitCode(t, sh); code != 3 {
		t.Fatal(code)
	}
	next, err := set.Open("s", t.TempDir(), 80, 24)
	if err != nil || next == sh {
		t.Fatalf("new shell: %v", err)
	}
}
func TestStopAndClose(t *testing.T) {
	set := newSet(t, "/bin/sleep", "300")
	a, err := set.Open("a", t.TempDir(), 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	b, err := set.Open("b", t.TempDir(), 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	set.Stop("a")
	if !gone(set, "a") || a.holder.Alive() {
		t.Fatal("Stop left the shell")
	}
	if gone(set, "b") || !b.holder.Alive() {
		t.Fatal("stopped other shell")
	}
	set.Close()
	if !gone(set, "b") {
		t.Fatal("Close left a shell")
	}
	if _, err = set.Open("c", t.TempDir(), 80, 24); err == nil {
		t.Fatal("closed set opened shell")
	}
}
