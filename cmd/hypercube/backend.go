package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/kkestell/hypercube/internal/buildinfo"
	"github.com/kkestell/hypercube/internal/desktop"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
)

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

// startApp finds the installed harnesses and registers saved launches, then
// serves HTTP while adopting the sessions the user left running. programs names
// the plugins and helpers.
// The desktop app's connection, when it has one, shows their alerts.
func startApp(listener net.Listener, programs session.Programs, desktop *desktop.Desktop) (*app, error) {
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
	a.server = &http.Server{Handler: handler(db, rt, desktop)}
	if err := rt.Restore(func() {
		go func() { a.done <- a.server.Serve(listener) }()
	}); err != nil {
		cancel()
		_ = db.Close()
		return nil, err
	}
	go watchAlerts(ctx, rt, desktop)
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
