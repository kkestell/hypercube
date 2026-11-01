package terminal

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// read collects output until it contains want.
func read(t *testing.T, out <-chan []byte, want string) string {
	t.Helper()
	var got bytes.Buffer
	timeout := time.After(5 * time.Second)
	for !strings.Contains(got.String(), want) {
		select {
		case b, ok := <-out:
			if !ok {
				t.Fatalf("output ended before %q: %q", want, got.String())
			}
			got.Write(b)
		case <-timeout:
			t.Fatalf("no %q in output: %q", want, got.String())
		}
	}
	return got.String()
}

func ended(t *testing.T, sh *Shell) {
	t.Helper()
	select {
	case <-sh.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the shell did not end")
	}
}

func TestInputOutputAndReplay(t *testing.T) {
	set := &Set{Command: []string{"/bin/cat"}}
	defer set.Close()
	sh, err := set.Open("s", t.TempDir(), 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := set.Open("s", t.TempDir(), 80, 24); again != sh {
		t.Fatal("a second open started another shell")
	}
	_, out, cancel := sh.Subscribe()
	defer cancel()
	if err := sh.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	read(t, out, "hello")
	// A later subscriber gets the earlier output first.
	replay, _, cancel2 := sh.Subscribe()
	defer cancel2()
	if !strings.Contains(string(replay), "hello") {
		t.Fatalf("replay = %q", replay)
	}
}

func TestReplayIsBounded(t *testing.T) {
	set := &Set{Command: []string{"/bin/sh", "-c", "head -c 600000 /dev/zero | tr '\\0' x; sleep 300"}}
	defer set.Close()
	sh, err := set.Open("s", t.TempDir(), 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		replay, _, cancel := sh.Subscribe()
		cancel()
		if len(replay) == bufferSize {
			return
		}
		if len(replay) > bufferSize || time.Now().After(deadline) {
			t.Fatalf("replay is %d bytes, want %d", len(replay), bufferSize)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStartsInDirectoryAtSize(t *testing.T) {
	dir := t.TempDir()
	set := &Set{Command: []string{"/bin/sh", "-c", "pwd -P; stty size; read line; stty size"}}
	defer set.Close()
	sh, err := set.Open("s", dir, 100, 30)
	if err != nil {
		t.Fatal(err)
	}
	_, out, cancel := sh.Subscribe()
	defer cancel()
	// A resolved path may differ from dir by a symlink such as /var.
	got := read(t, out, "30 100")
	if !strings.Contains(got, strings.TrimPrefix(dir, "/private")) {
		t.Fatalf("output %q does not name %s", got, dir)
	}
	if err := sh.Resize(120, 40); err != nil {
		t.Fatal(err)
	}
	if err := sh.Write([]byte("\n")); err != nil {
		t.Fatal(err)
	}
	read(t, out, "40 120")
}

func TestShellExit(t *testing.T) {
	set := &Set{Command: []string{"/bin/sh", "-c", "echo bye; exit 3"}}
	defer set.Close()
	sh, err := set.Open("s", t.TempDir(), 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	_, out, cancel := sh.Subscribe()
	defer cancel()
	read(t, out, "bye")
	ended(t, sh)
	if code := sh.ExitCode(); code != 3 {
		t.Fatalf("exit code = %d", code)
	}
	// The channel closes after the exit is recorded, and a subscriber that
	// arrives late still gets the output.
	if _, ok := <-out; ok {
		t.Fatal("output continued after the shell ended")
	}
	replay, late, _ := sh.Subscribe()
	if !strings.Contains(string(replay), "bye") {
		t.Fatalf("replay after exit = %q", replay)
	}
	if _, ok := <-late; ok {
		t.Fatal("a subscription after exit stayed open")
	}
	// The session's next open starts a new shell.
	deadline := time.Now().Add(5 * time.Second)
	for {
		next, err := set.Open("s", t.TempDir(), 80, 24)
		if err != nil {
			t.Fatal(err)
		}
		if next != sh {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the ended shell stayed in the set")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStopEndsOnlyThatSession(t *testing.T) {
	set := &Set{Command: []string{"/bin/sleep", "300"}}
	defer set.Close()
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
		t.Fatal("stopping one session ended another's shell")
	default:
	}
	set.Stop("missing")
}

func TestCloseEndsEveryShellAndRefusesNew(t *testing.T) {
	set := &Set{Command: []string{"/bin/sleep", "300"}}
	a, _ := set.Open("a", t.TempDir(), 80, 24)
	b, _ := set.Open("b", t.TempDir(), 80, 24)
	set.Close()
	ended(t, a)
	ended(t, b)
	if _, err := set.Open("c", t.TempDir(), 80, 24); err == nil {
		t.Fatal("a closed set opened a shell")
	}
}

func TestStopKillsAShellThatIgnoresHangup(t *testing.T) {
	set := &Set{Command: []string{"/bin/sh", "-c", "trap '' HUP; echo ready; while :; do sleep 1; done"}}
	defer set.Close()
	sh, err := set.Open("s", t.TempDir(), 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	_, out, cancel := sh.Subscribe()
	defer cancel()
	read(t, out, "ready")
	set.Stop("s")
	ended(t, sh)
}
