// Command mux is a terminal multiplexer's command line. Every subcommand but
// the hidden "server" is a client of the server on the user's socket.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/kkestell/hypercube/mux/client"
	"github.com/kkestell/hypercube/mux/internal/server"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mux:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: mux spawn [-s session] [-w window] [-c dir] [command...] | mux ls | mux kill <id> | mux attach <session>")
	}
	switch args[0] {
	case "server":
		return server.New().ListenAndServe(client.SocketPath())
	case "spawn":
		return spawn(args[1:])
	case "ls":
		return ls()
	case "kill":
		if len(args) != 2 {
			return fmt.Errorf("usage: mux kill <id>")
		}
		return kill(args[1])
	case "attach":
		if len(args) != 2 {
			return fmt.Errorf("usage: mux attach <session>")
		}
		return attach(args[1])
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func spawn(args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	// Parsing stops at the first non-option, so the command keeps its own flags.
	flags := flag.NewFlagSet("spawn", flag.ContinueOnError)
	session := flags.String("s", "", "add a window to this session")
	window := flags.String("w", "", "add a pane to this window")
	flags.StringVar(&dir, "c", dir, "run the command in this directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	argv := flags.Args()
	if len(argv) == 0 {
		argv = []string{os.Getenv("SHELL")}
	}
	c, err := connect()
	if err != nil {
		return err
	}
	defer c.Close()
	reply, err := c.Call(client.Request{Type: "spawn", Session: *session, Window: *window, Dir: dir, Argv: argv, Env: os.Environ()})
	if err != nil {
		return err
	}
	fmt.Println(strings.Join([]string{reply.Session, reply.Window, reply.Pane}, " "))
	return nil
}

// ls prints one line per pane.
func ls() error {
	c, err := connect()
	if err != nil {
		return err
	}
	defer c.Close()
	reply, err := c.Call(client.Request{Type: "list"})
	if err != nil {
		return err
	}
	for _, s := range reply.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				state := "running"
				if !p.Alive {
					state = fmt.Sprintf("exited %d", p.Status)
				}
				fmt.Println(s.ID, w.ID, p.ID, state)
			}
		}
	}
	return nil
}

// kill removes the pane, window, or session with the given ID.
func kill(id string) error {
	req := client.Request{Type: "kill"}
	switch {
	case strings.HasPrefix(id, "p"):
		req.Pane = id
	case strings.HasPrefix(id, "w"):
		req.Window = id
	case strings.HasPrefix(id, "s"):
		req.Session = id
	default:
		return fmt.Errorf("unknown ID %q", id)
	}
	c, err := connect()
	if err != nil {
		return err
	}
	defer c.Close()
	_, err = c.Call(req)
	return err
}

// connect dials the server, starting it first if nothing is listening. Two
// commands starting a server at the same moment is unlikely; the loser's server
// exits with an error and its dial reaches the winner.
func connect() (*client.Conn, error) {
	path := client.SocketPath()
	if c, err := client.Dial(path); err == nil {
		return c, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, "server")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	cmd.Process.Release()

	deadline := time.Now().Add(2 * time.Second)
	for {
		c, err := client.Dial(path)
		if err == nil {
			return c, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("server did not start: %w", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
