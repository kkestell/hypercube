// Command hypercube runs teams of coding agents. `hypercube ui` is the app
// process: it manages sessions and attaches to their holders. `hypercube mcp`
// serves harnesses configured by earlier versions, forwarding to the backend.
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
	"strings"
	"sync"
	"syscall"

	"github.com/kkestell/hypercube/internal/buildinfo"
	"github.com/kkestell/hypercube/internal/config"
	"github.com/kkestell/hypercube/internal/desktop"
	"github.com/kkestell/hypercube/internal/hold"
	"github.com/kkestell/hypercube/internal/mcpbridge"
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
		err = mcpbridge.Main(os.Stdin, os.Stdout)
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

// startApp upgrades the config, finds the installed harnesses, and registers
// saved launches, then serves HTTP while adopting the
// sessions the user left running. programs names the plugins and helpers.
// The desktop app's connection, when it has one, shows their alerts.
func startApp(listener net.Listener, programs session.Programs, desktop *desktop.Desktop) (*app, error) {
	// An earlier version's config is converted before anything reads it.
	if err := config.Upgrade(); err != nil {
		return nil, fmt.Errorf("upgrading the config: %w", err)
	}
	db, err := store.Open()
	if err != nil {
		return nil, err
	}
	rt := session.New(db, "http://"+listener.Addr().String(), programs)
	ctx, cancel := context.WithCancel(context.Background())
	// Sessions that start know the installed harnesses. A plugin that cannot
	// describe its harness is listed with its error.
	if _, err := rt.ScanHarnesses(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "hypercube: finding harness plugins:", err)
	}
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
			if err := sendAlert(desktop, alert); err != nil {
				fmt.Fprintln(os.Stderr, "hypercube: alerting the desktop app:", err)
			}
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "hypercube: watching alerts:", err)
		}
	}()
	return a, nil
}

// sendAlert passes an alert to the desktop app as the notification
// preferences allow. An unreadable config is reported and sends the alert as
// the defaults do.
func sendAlert(d *desktop.Desktop, alert session.Alert) error {
	n := config.DefaultNotifications
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "hypercube: reading notification preferences:", err)
	} else {
		n = cfg.Notifications
	}
	wanted := n.Replies
	if strings.HasPrefix(alert.Key, "request:") {
		wanted = n.Requests
	}
	if !wanted || !n.Desktop {
		return nil
	}
	return d.Alert(alert, n.Sound)
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
	p, err := programs()
	if err != nil {
		return err
	}
	a, err := startApp(listener, p, &desktop.Desktop{})
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
