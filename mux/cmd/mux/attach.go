package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/kkestell/hypercube/mux/client"
	"golang.org/x/term"
)

// attach shows the session's pane on the terminal until the server detaches it.
// The server draws every frame; this only passes frames, input, and size
// changes along.
func attach(session string) error {
	fd := int(os.Stdin.Fd())
	cols, rows, err := term.GetSize(fd)
	if err != nil {
		return err
	}
	c, err := connect()
	if err != nil {
		return err
	}
	defer c.Close()
	if _, err := c.Call(client.Request{Type: "attach", Session: session, Cols: cols, Rows: rows}); err != nil {
		return err
	}

	saved, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	os.Stdout.WriteString("\x1b[?1049h")
	defer func() {
		os.Stdout.WriteString("\x1b[?1049l\x1b[?25h\x1b[0m")
		term.Restore(fd, saved)
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || c.Send(client.Request{Type: "input", Data: buf[:n]}) != nil {
				return
			}
		}
	}()
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	go func() {
		for range winch {
			if cols, rows, err := term.GetSize(fd); err == nil {
				c.Send(client.Request{Type: "resize", Cols: cols, Rows: rows})
			}
		}
	}()

	for {
		out, err := c.ReadOutput()
		if err != nil || out.Detached {
			return nil
		}
		os.Stdout.Write(out.Frame)
	}
}
