// Command mux is a terminal multiplexer's command line. Every subcommand but
// the hidden "server" is a client of the server on the user's socket.
package main

import (
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
		return fmt.Errorf("usage: mux spawn [command...] | mux attach <session>")
	}
	switch args[0] {
	case "server":
		return server.New().ListenAndServe(client.SocketPath())
	case "spawn":
		return spawn(args[1:])
	case "attach":
		if len(args) != 2 {
			return fmt.Errorf("usage: mux attach <session>")
		}
		return attach(args[1])
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func spawn(argv []string) error {
	if len(argv) == 0 {
		argv = []string{os.Getenv("SHELL")}
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	c, err := connect()
	if err != nil {
		return err
	}
	defer c.Close()
	reply, err := c.Call(client.Request{Type: "spawn", Dir: dir, Argv: argv, Env: os.Environ()})
	if err != nil {
		return err
	}
	fmt.Println(strings.Join([]string{reply.Session, reply.Window, reply.Pane}, " "))
	return nil
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
