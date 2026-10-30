package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/kkestell/hypercube/mux/client"
	"golang.org/x/term"
)

// attach shows the session on the terminal until the server detaches it. With
// no session it attaches to the first one, or starts a shell in a new one. The
// server draws every frame and puts the terminal in the focused pane's keyboard
// mode; this only passes frames, input, and size changes along.
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
	if session == "" {
		if session, err = firstSession(c); err != nil {
			return err
		}
	}
	if _, err := c.Call(client.Request{Type: "attach", Session: session, Cols: cols, Rows: rows}); err != nil {
		return err
	}

	saved, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	// The terminal reports button events with motion while a button is held, in
	// SGR format. A pane that asks for motion with no button held (mode 1003)
	// gets motion only during drags.
	// The window title is pushed so that detaching restores it.
	os.Stdout.WriteString("\x1b[22;0t\x1b[?1049h\x1b[?1002h\x1b[?1006h")
	defer func() {
		// Whatever keyboard mode the last frame set, the shell gets its keys back
		// in the default one, and no mouse reports.
		os.Stdout.WriteString("\x1b[?1002l\x1b[?1006l\x1b[=0;1u\x1b[>4;0m\x1b[?1l\x1b>\x1b[?2004l\x1b[?1049l\x1b[?25h\x1b[0m\x1b[23;0t")
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

// firstSession returns the first session's ID, spawning $SHELL in the working
// directory when there is none.
func firstSession(c *client.Conn) (string, error) {
	reply, err := c.Call(client.Request{Type: "list"})
	if err != nil {
		return "", err
	}
	if len(reply.Sessions) > 0 {
		return reply.Sessions[0].ID, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	reply, err = c.Call(client.Request{Type: "spawn", Dir: dir, Argv: []string{os.Getenv("SHELL")}, Env: os.Environ()})
	return reply.Session, err
}
