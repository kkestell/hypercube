// Command hypercube runs teams of coding agents in tmux. `hypercube ui` is
// the app process; the other commands run inside agents' panes and harnesses.
package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/mcp"
	"github.com/kkestell/hypercube/internal/menu"
	"github.com/kkestell/hypercube/internal/report"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/tmux"
	"github.com/kkestell/hypercube/internal/ui"
	"github.com/kkestell/hypercube/internal/wait"
)

const usage = "usage: hypercube ui [port] [--menu]"

func main() {
	var err error
	switch {
	case len(os.Args) == 1:
		err = errors.New(usage)
	case len(os.Args) <= 4 && os.Args[1] == "ui":
		err = serveUI(os.Args[2:])
	case len(os.Args) == 2 && os.Args[1] == "wait":
		var delivered bool
		delivered, err = wait.Run(os.Stdout)
		if err == nil && delivered {
			os.Exit(2)
		}
	case len(os.Args) == 3 && os.Args[1] == "agent":
		err = runAgent(os.Args[2])
	case len(os.Args) == 4 && os.Args[1] == "exited":
		err = exited(os.Args[2], os.Args[3])
	case len(os.Args) == 2 && os.Args[1] == "mcp":
		err = mcp.Serve(os.Stdin, os.Stdout)
	case len(os.Args) == 3 && os.Args[1] == "report":
		err = report.Run(os.Args[2], os.Stdin, os.Stdout)
	default:
		err = errors.New(usage)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hypercube:", err)
		os.Exit(1)
	}
}

func parseUIArgs(args []string) (port string, withMenu bool, err error) {
	port = "4280"
	havePort := false
	for _, arg := range args {
		if arg == "--menu" {
			withMenu = true
			continue
		}
		if havePort {
			return "", false, errors.New("ui accepts only one port")
		}
		n, err := strconv.Atoi(arg)
		if err != nil || n < 1 || n > 65535 {
			return "", false, fmt.Errorf("invalid port %q", arg)
		}
		port, havePort = arg, true
	}
	return port, withMenu, nil
}

// serveUI serves the browser view on the optional port, default 4280.
func serveUI(args []string) error {
	port, withMenu, err := parseUIArgs(args)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		return err
	}
	defer listener.Close()
	db, err := store.Open()
	if err != nil {
		return err
	}
	defer db.Close()
	session.Restore(db)
	handler := ui.Handler(db)
	url := "http://localhost:" + port + "/"
	fmt.Printf("Serving the Hypercube UI at %s\n", url)
	if withMenu {
		go func() {
			if err := http.Serve(listener, handler); err != nil {
				fmt.Fprintln(os.Stderr, "hypercube:", err)
				os.Exit(1)
			}
		}()
		menu.Run(url, ui.Icon())
		return nil
	}
	return http.Serve(listener, handler)
}

// runAgent replaces the wrapper with the harness. tmux owns its PTY and process
// lifetime; the pane-died hook handles relaunches using the saved conversation.
func runAgent(prompt string) error {
	id, pane := os.Getenv("HYPERCUBE_AGENT_ID"), os.Getenv("TMUX_PANE")
	if id == "" || !tmux.Owns(pane, id) {
		return errors.New("agent must run in its own Hypercube pane")
	}
	db, err := store.Open()
	if err != nil {
		return err
	}
	defer db.Close()
	var agent store.Agent
	err = db.Tx(func(tx *store.Tx) error {
		a, err := tx.Agent(id)
		if err != nil {
			return err
		}
		a.Pane, a.Launched, a.Attention = pane, time.Now().UnixMilli(), false
		a.Activity = store.Idle
		if prompt != "" {
			a.Activity = store.Busy
		}
		agent = a
		return tx.SaveAgent(a)
	})
	if err != nil {
		return err
	}
	argv, env, err := harness.Command(agent, prompt)
	if err == nil {
		var binary string
		binary, err = exec.LookPath(argv[0])
		if err == nil {
			var inherited []string
			for _, item := range os.Environ() {
				key, _, _ := strings.Cut(item, "=")
				if _, replaced := env[key]; !replaced {
					inherited = append(inherited, item)
				}
			}
			for key, value := range env {
				inherited = append(inherited, key+"="+value)
			}
			err = syscall.Exec(binary, argv, inherited)
		}
	}
	_ = db.Tx(func(tx *store.Tx) error {
		a, e := tx.Agent(id)
		if e != nil {
			return e
		}
		a.Activity = store.Exited
		return tx.SaveAgent(a)
	})
	return err
}

// exited runs from a pane-died hook.
func exited(agent, pane string) error {
	db, err := store.Open()
	if err != nil {
		return err
	}
	defer db.Close()
	return session.Exited(db, agent, pane)
}
