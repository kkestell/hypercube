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

	"github.com/kkestell/hypercube/internal/buildinfo"
	"github.com/kkestell/hypercube/internal/desktop"
	"github.com/kkestell/hypercube/internal/hold"
	"github.com/kkestell/hypercube/internal/mcp"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/ui"
)

const usage = "usage: hypercube [version | ui [port] | mcp | hold [-terminal -cols N -rows N] <dir> <command> [args...]]"

func main() {
	var err error
	switch {
	case len(os.Args) > 1 && os.Args[1] == "hold":
		err = hold.Main(os.Args[2:])
	case len(os.Args) == 2 && os.Args[1] == "version":
		fmt.Println(buildinfo.String())
	case len(os.Args) > 1 && os.Args[1] == "ui":
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

func parsePort(args []string) (string, error) {
	if len(args) == 0 {
		return "4280", nil
	}
	if len(args) > 1 {
		return "", errors.New("ui accepts only one port")
	}
	n, err := strconv.Atoi(args[0])
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("invalid port %q", args[0])
	}
	return args[0], nil
}

// app is the app process: the database, the runtime that owns the agents, and
// the HTTP server.
type app struct {
	db      *store.DB
	rt      *session.Runtime
	server  *http.Server
	desktop *desktop.Desktop
	cancel  context.CancelFunc
	// done receives the server's result.
	done chan error
	once sync.Once
}

// startApp registers saved launches, then serves HTTP while adopting the
// sessions the user left running. Harness plugins are in plugins, or beside
// the executable when it is empty. The desktop app's connection, when it has one, drives the
// sessions' browsers and shows their alerts.
func startApp(listener net.Listener, plugins string, desktop *desktop.Desktop) (*app, error) {
	db, err := store.Open()
	if err != nil {
		return nil, err
	}
	rt := session.New(db, "http://"+listener.Addr().String())
	rt.PluginDir = plugins
	ctx, cancel := context.WithCancel(context.Background())
	a := &app{db: db, rt: rt, desktop: desktop, cancel: cancel, done: make(chan error, 1)}
	a.server = &http.Server{Handler: ui.Handler(db, rt, desktop)}
	if err := rt.Restore(func() {
		go func() { a.done <- a.server.Serve(listener) }()
	}); err != nil {
		cancel()
		_ = db.Close()
		return nil, err
	}
	go func() {
		err := rt.WatchAlerts(ctx, func(alert session.Alert) {
			// A failed alert is reported and the watch goes on.
			if err := desktop.Alert(alert); err != nil {
				fmt.Fprintln(os.Stderr, "hypercube: alerting the desktop app:", err)
			}
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "hypercube: watching alerts:", err)
		}
	}()
	return a, nil
}

// close ends the app. With quit it ends every agent first, and each session
// starts again when next used; without it the agents keep running for the
// next start to adopt.
func (a *app) close(quit bool) {
	a.once.Do(func() {
		if quit {
			a.rt.Quit()
		} else {
			a.rt.Detach()
		}
		a.desktop.Shutdown()
		_ = a.server.Close()
		a.cancel()
		_ = a.db.Close()
	})
}

// serveUI serves the browser view on the optional port, default 4280, until
// SIGINT or SIGTERM. SIGINT ends the agents; SIGTERM leaves them running. The
// desktop app connects to the same server.
func serveUI(args []string) error {
	port, err := parsePort(args)
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
	a, err := startApp(listener, "", &desktop.Desktop{})
	if err != nil {
		return err
	}
	fmt.Printf("%s; serving at http://localhost:%s/\n", buildinfo.String(), port)
	select {
	case sig := <-signals:
		a.close(sig == syscall.SIGINT)
		return nil
	case err := <-a.done:
		a.close(false)
		return err
	}
}
