package terminal

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
func output(t *testing.T, sh *Shell) *Output {
	t.Helper()
	out, err := sh.Output()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { out.Close() })
	return out
}
func ended(t *testing.T, sh *Shell) {
	t.Helper()
	select {
	case <-sh.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("shell did not end")
	}
}
func TestInputOutputAndAdopt(t *testing.T) {
	set := newSet(t, "/bin/cat")
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
	set.Detach()
	next := &Set{Dir: set.Dir}
	t.Cleanup(next.Close)
	if err = next.Adopt("s", filepath.Join(set.Dir, "shell-s")); err != nil {
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
	ended(t, sh)
	if sh.ExitCode() != 0 {
		t.Fatal(sh.ExitCode())
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
	ended(t, sh)
	if sh.ExitCode() != 3 {
		t.Fatal(sh.ExitCode())
	}
	<-sh.removed
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
	ended(t, a)
	select {
	case <-b.Done():
		t.Fatal("stopped other shell")
	default:
	}
	set.Close()
	ended(t, b)
	if _, err = set.Open("c", t.TempDir(), 80, 24); err == nil {
		t.Fatal("closed set opened shell")
	}
}
