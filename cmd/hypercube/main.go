// Command hypercube runs teams of coding agents. `hypercube ui` is the app
// process: it manages sessions and attaches to their holders. `hypercube mcp` is the stdio server each
// agent's harness runs, which forwards to the app.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"github.com/kkestell/hypercube/internal/hold"
	"github.com/kkestell/hypercube/internal/mcp"
	"github.com/kkestell/hypercube/internal/menu"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/ui"
)

const usage = "usage: hypercube ui [port] [--menu] | mcp | hold [-terminal -cols N -rows N] <dir> <command> [args...]"

func main() {
	var err error
	switch {
	case len(os.Args) > 1 && os.Args[1] == "hold":
		err = hold.Main(os.Args[2:])
	case len(os.Args) == 1:
		err = errors.New(usage)
	case len(os.Args) <= 4 && os.Args[1] == "ui":
		err = serveUI(os.Args[2:])
	case len(os.Args) == 2 && os.Args[1] == "mcp":
		err = mcp.Bridge(os.Stdin, os.Stdout)
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

// app is the app process: the database, the runtime that owns the agents, and
// the HTTP server.
type app struct {
	db     *store.DB
	rt     *session.Runtime
	server *http.Server
	cancel context.CancelFunc
	// done receives the server's result.
	done chan error
	once sync.Once
}

// startApp serves HTTP on the listener, then restores the sessions the user
// left running. Harness plugins are in plugins, or beside the executable when
// it is empty.
func startApp(listener net.Listener, plugins string) (*app, error) {
	db, err := store.Open()
	if err != nil {
		return nil, err
	}
	rt := session.New(db, "http://"+listener.Addr().String())
	rt.PluginDir = plugins
	ctx, cancel := context.WithCancel(context.Background())
	a := &app{db: db, rt: rt, cancel: cancel, done: make(chan error, 1)}
	a.server = &http.Server{Handler: ui.Handler(db, rt)}
	go func() { a.done <- a.server.Serve(listener) }()
	rt.Restore()
	go rt.StopIdleSessions(ctx)
	return a, nil
}

// shutdown ends every agent, then the server. Sessions keep their running
// mark, so the next start restores them.
func (a *app) shutdown() { a.close(true) }
func (a *app) detach()   { a.close(false) }
func (a *app) close(quit bool) {
	a.once.Do(func() {
		if quit {
			a.rt.Quit()
		} else {
			a.rt.Detach()
		}
		_ = a.server.Close()
		a.cancel()
		_ = a.db.Close()
	})
}

// serveUI serves the browser view on the optional port, default 4280, until
// the menu's Quit, SIGINT, or SIGTERM.
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
	// Signals received during restore are handled after its attachments exist.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	a, err := startApp(listener, "")
	if err != nil {
		return err
	}
	url := "http://localhost:" + port + "/"
	fmt.Printf("Serving the Hypercube UI at %s\n", url)
	if withMenu {
		go func() {
			select {
			case sig := <-signals:
				a.close(sig == syscall.SIGINT)
				os.Exit(0)
			case err := <-a.done:
				a.detach()
				fmt.Fprintln(os.Stderr, "hypercube:", err)
				os.Exit(1)
			}
		}()
		menu.Run(url, ui.Icon())
		a.shutdown()
		return nil
	}
	select {
	case sig := <-signals:
		a.close(sig == syscall.SIGINT)
		return nil
	case err := <-a.done:
		a.detach()
		return err
	}
}
