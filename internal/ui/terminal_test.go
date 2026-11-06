package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/kkestell/hypercube/internal/desktop"
	"github.com/kkestell/hypercube/internal/session"
	"github.com/kkestell/hypercube/internal/store"
	"github.com/kkestell/hypercube/internal/terminal"
)

func TestTerminalStartsOnlyAfterWebSocketHandshake(t *testing.T) {
	db := open(t)
	change(t, db, func(tx *store.Tx) error {
		if err := tx.AddWorkspace(store.Workspace{ID: "w", Path: t.TempDir()}); err != nil {
			return err
		}
		return tx.AddSession(store.Session{ID: "s", Workspace: "w", Dir: t.TempDir()})
	})
	// Keep the holder's Unix socket path within macOS's length limit.
	dir, err := os.MkdirTemp("", "hc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	rt := session.New(db, "")
	rt.Terminals.Dir = dir
	rt.Terminals.Command = []string{"/bin/sh", "-c", "printf 'terminal-ready\\n'; exec /bin/cat"}
	t.Cleanup(rt.Quit)
	server := httptest.NewServer(Handler(db, rt, &desktop.Desktop{}))
	t.Cleanup(server.Close)
	response, err := server.Client().Get(server.URL + "/terminal?session=s")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("plain GET status = %d", response.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(dir, terminal.Name("s"))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("plain GET created a shell holder: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, _, err := dial(t, server, "/terminal?session=s")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	var output []byte
	for !bytes.Contains(output, []byte("terminal-ready")) {
		kind, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if kind != websocket.MessageBinary {
			t.Fatalf("expected shell output, got %s", data)
		}
		output = append(output, data...)
	}

	failed, _, err := dial(t, server, "/terminal?session="+strings.Repeat("x", 200))
	if err != nil {
		t.Fatal(err)
	}
	defer failed.CloseNow()
	kind, data, err := failed.Read(ctx)
	var exit struct{ Exit int }
	if err != nil || kind != websocket.MessageText || json.Unmarshal(data, &exit) != nil || exit.Exit != -1 {
		t.Fatalf("failed startup = %s, %v", data, err)
	}
	if _, _, err := failed.Read(ctx); websocket.CloseStatus(err) != websocket.StatusInternalError {
		t.Fatalf("failed startup close = %v", err)
	}
}
