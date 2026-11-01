package ui

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/store"
)

// open returns a database in a temporary state directory, with an isolated
// tmux server.
func open(t *testing.T) *store.DB {
	t.Helper()
	socket, err := os.MkdirTemp("/tmp", "hc-ui-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-server").Run() })
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	db, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func change(t *testing.T, db *store.DB, fn func(*store.Tx) error) {
	t.Helper()
	if err := db.Tx(fn); err != nil {
		t.Fatal(err)
	}
}

func TestLeads(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "hypercube/config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `{"profiles":[
		{"name":"first","role":"lead","description":"First lead","harness":"claude","model":"opus","effort":"high"},
		{"name":"worker","role":"work","description":"Worker","harness":"codex","model":"sol","effort":"high"},
		{"name":"second","role":"lead","description":"Second lead","harness":"codex","model":"sol","effort":"high"}
	]}`
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := Handler(open(t))
	get := func() *httptest.ResponseRecorder {
		t.Helper()
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, httptest.NewRequest("GET", "http://localhost:4280/leads", nil))
		return res
	}
	res := get()
	if res.Code != http.StatusOK || res.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("leads response = %d %s", res.Code, res.Body)
	}
	type lead struct{ Name, Description string }
	var got []lead
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []lead{{"first", "First lead"}, {"second", "Second lead"}}) {
		t.Fatalf("leads = %+v", got)
	}
	// The same handler reads config edits instead of keeping a cached list.
	if err := os.WriteFile(path, []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	res = get()
	if res.Code != http.StatusInternalServerError || !strings.Contains(res.Body.String(), path+": unexpected end of JSON input") {
		t.Fatalf("broken config response = %d %s", res.Code, res.Body)
	}
}

func TestIcon(t *testing.T) {
	res := httptest.NewRecorder()
	Handler(open(t)).ServeHTTP(res, httptest.NewRequest("GET", "http://localhost:4280/favicon.svg", nil))
	if res.Code != http.StatusOK || res.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("favicon response = %d, %v", res.Code, res.Header())
	}
	if len(Icon()) == 0 || !bytes.Equal(res.Body.Bytes(), Icon()) {
		t.Fatal("favicon differs from the menu bar icon")
	}
}

func TestStateStreamsSnapshots(t *testing.T) {
	db := open(t)
	change(t, db, func(tx *store.Tx) error { return tx.AddWorkspace(store.Workspace{ID: "w", Path: "/src/repo"}) })
	server := httptest.NewServer(Handler(db))
	defer server.Close()
	res, err := http.Get(server.URL + "/state")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	events := bufio.NewReader(res.Body)
	next := func() []store.ShownWorkspace {
		t.Helper()
		for {
			line, err := events.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if data, ok := strings.CutPrefix(strings.TrimSpace(line), "data: "); ok {
				var list []store.ShownWorkspace
				if err := json.Unmarshal([]byte(data), &list); err != nil {
					t.Fatal(err)
				}
				return list
			}
		}
	}
	if list := next(); len(list) != 1 || list[0].Name != "repo" || len(list[0].Sessions) != 0 {
		t.Fatalf("first snapshot = %+v", list)
	}
	change(t, db, func(tx *store.Tx) error {
		return tx.AddSession(store.Session{ID: "s", Workspace: "w", Title: "Fix it"})
	})
	if list := next(); len(list[0].Sessions) != 1 || list[0].Sessions[0].Title != "Fix it" || list[0].Sessions[0].Running {
		t.Fatalf("snapshot after a change = %+v", list)
	}
}

func TestPageDrivesControlClient(t *testing.T) {
	db := open(t)
	for _, args := range [][]string{
		{"-f", "/dev/null", "new-session", "-d", "-s", "plain"},
		{"new-session", "-d", "-s", "hypercube-s1"},
		{"set-option", "-t", "hypercube-s1", "@hypercube_session", "s1"},
		{"new-session", "-d", "-s", "hypercube-s2"},
	} {
		if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
			t.Fatalf("tmux %v: %v\n%s", args, err, out)
		}
	}
	server := httptest.NewServer(Handler(db))
	defer server.Close()

	foreign, _ := http.NewRequest("GET", server.URL, nil)
	foreign.Host = "example.com"
	if res, err := http.DefaultClient.Do(foreign); err != nil || res.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign host: %v %v", res.Status, err)
	}
	// A stopped session, and a tmux session Hypercube did not mark, have no
	// terminals to attach to.
	for _, id := range []string{"s2", "missing"} {
		if res, err := http.Get(server.URL + "/events?session=" + id); err != nil || res.StatusCode != http.StatusConflict {
			t.Fatalf("events for %s: %v %v", id, res.Status, err)
		}
	}

	res, err := http.Get(server.URL + "/events?session=s1")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	events := bufio.NewReader(res.Body)
	if line, _ := events.ReadString('\n'); line != "event: id\n" {
		t.Fatalf("first event: %q", line)
	}
	data, _ := events.ReadString('\n')
	id := strings.TrimSpace(strings.TrimPrefix(data, "data: "))

	post := func(origin string) int {
		req, _ := http.NewRequest("POST", server.URL+"/send?id="+id, strings.NewReader("display-message -p '#{session_name}'"))
		req.Header.Set("Origin", origin)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if code := post("http://example.com"); code != http.StatusForbidden {
		t.Fatalf("foreign origin: %d", code)
	}
	if code := post(server.URL); code != http.StatusOK {
		t.Fatalf("send: %d", code)
	}

	// The client attached to the session's tmux session and answered.
	got := make(chan bool)
	go func() {
		for {
			line, err := events.ReadString('\n')
			if err != nil {
				return
			}
			text, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(strings.TrimSpace(line), "data: "))
			if string(text) == "hypercube-s1" {
				got <- true
				return
			}
		}
	}()
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("no reply from the control client")
	}
}

func TestActions(t *testing.T) {
	db := open(t)
	change(t, db, func(tx *store.Tx) error {
		tx.AddWorkspace(store.Workspace{ID: "w", Path: t.TempDir()})
		tx.AddSession(store.Session{ID: "s", Workspace: "w", Unseen: true})
		tx.AddSession(store.Session{ID: "t", Workspace: "w"})
		return tx.AddAgent(store.Agent{ID: "a", Session: "t", Name: "lead", Role: store.Lead, Activity: store.Exited})
	})
	server := httptest.NewServer(Handler(db))
	defer server.Close()
	post := func(path string, want int) {
		t.Helper()
		req, _ := http.NewRequest("POST", server.URL+path, nil)
		req.Header.Set("Origin", server.URL)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != want {
			t.Errorf("%s: status %d, want %d", path, res.StatusCode, want)
		}
	}
	post("/session?action=rm&id=s", http.StatusBadRequest)
	post("/workspace?action=rm", http.StatusBadRequest)
	post("/session?action=rename&id=s&title=%20", http.StatusConflict)
	post("/session?action=rename&id=missing&title=x", http.StatusConflict)
	post("/session?action=rename&id=s&title=Parser", http.StatusOK)
	post("/session?action=seen&id=s", http.StatusOK)
	change(t, db, func(tx *store.Tx) error {
		if s, err := tx.Session("s"); err != nil || s.Title != "Parser" || s.Unseen {
			t.Errorf("session after rename and seen = %+v, %v", s, err)
		}
		return nil
	})
	post("/session?action=delete&id=s", http.StatusOK)
	change(t, db, func(tx *store.Tx) error {
		if list, _ := tx.Sessions("w"); len(list) != 1 || list[0].ID != "t" {
			t.Errorf("sessions after delete = %+v", list)
		}
		return nil
	})
	post("/workspace?action=delete&id=w", http.StatusOK)
	change(t, db, func(tx *store.Tx) error {
		if list, _ := tx.Workspaces(); len(list) != 0 {
			t.Errorf("workspaces after delete = %+v", list)
		}
		if _, err := tx.Agent("a"); err == nil {
			t.Error("the deleted workspace's agent remains")
		}
		return nil
	})
}
