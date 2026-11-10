package plugin

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func start(t *testing.T, name string, args ...string) *Process {
	t.Helper()
	p, err := StartProcess("Shell", exec.Command(name, args...))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func waitDone(t *testing.T, p *Process, within time.Duration) {
	t.Helper()
	select {
	case <-p.Done():
	case <-time.After(within):
		t.Fatal("the process did not end")
	}
}

func TestMissingCLI(t *testing.T) {
	_, err := StartProcess("Nothing Here", exec.Command("hypercube-no-such-cli"))
	if err == nil || err.Error() != "Nothing Here is missing: install hypercube-no-such-cli" {
		t.Fatal(err)
	}
}

func TestReadReturnsTheExitReason(t *testing.T) {
	p := start(t, "sh", "-c", "echo out; echo oops >&2; exit 3")
	var lines []string
	err := p.Read(func(line []byte) { lines = append(lines, string(line)) })
	if len(lines) != 1 || lines[0] != "out\n" {
		t.Fatal(lines)
	}
	if err == nil || err.Error() != "sh exited: exit status 3: oops" {
		t.Fatal(err)
	}
	waitDone(t, p, time.Second)
	if p.Err() != err {
		t.Fatal(p.Err())
	}
}

// A write to a CLI that has ended returns why it ended.
func TestWriteAfterTheProcessEndsReturnsTheExitReason(t *testing.T) {
	p := start(t, "sh", "-c", "echo why >&2; exit 3")
	_ = p.Read(func([]byte) {})
	// Wait has closed the pipe by now.
	_, err := p.Stdin.Write([]byte("hello\n"))
	if err == nil || !strings.Contains(err.Error(), "exit status 3") || !strings.Contains(err.Error(), "why") {
		t.Fatal(err)
	}
}

// The last line reaches the handler even without a newline, and Done waits
// for the handler to return.
func TestDoneFollowsTheLastLine(t *testing.T) {
	p := start(t, "sh", "-c", `printf '{"last":1}'; exit 3`)
	handling := make(chan string)
	release := make(chan struct{})
	go func() {
		_ = p.Read(func(line []byte) {
			handling <- string(line)
			<-release
		})
	}()
	if line := <-handling; line != `{"last":1}` {
		t.Fatal(line)
	}
	<-p.exited
	select {
	case <-p.Done():
		t.Fatal("Done closed while the last line was being handled")
	default:
	}
	close(release)
	waitDone(t, p, 5*time.Second)
}

// Children that keep stdout open do not hold the reader.
func TestReadEndsSoonAfterTheExit(t *testing.T) {
	t.Parallel()
	p := start(t, "sh", "-c", "sleep 3 & exit 0")
	begun := time.Now()
	if err := p.Read(func([]byte) {}); err == nil || !strings.HasPrefix(err.Error(), "sh exited") {
		t.Fatal(err)
	}
	if elapsed := time.Since(begun); elapsed > 2500*time.Millisecond {
		t.Fatalf("Read took %v", elapsed)
	}
	waitDone(t, p, time.Second)
}

func TestStop(t *testing.T) {
	t.Parallel()
	t.Run("closing stdin", func(t *testing.T) {
		p := start(t, "cat")
		go p.Read(func([]byte) {})
		begun := time.Now()
		p.Stop()
		waitDone(t, p, time.Second)
		if elapsed := time.Since(begun); elapsed > 2*time.Second {
			t.Fatalf("Stop took %v", elapsed)
		}
	})
	t.Run("killing", func(t *testing.T) {
		p := start(t, "sleep", "30")
		go p.Read(func([]byte) {})
		p.Stop()
		waitDone(t, p, 5*time.Second)
		if err := p.Err(); err == nil || !strings.Contains(err.Error(), "killed") {
			t.Fatal(err)
		}
	})
}
