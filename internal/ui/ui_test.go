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
)

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
	handler := Handler()
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
	Handler().ServeHTTP(res, httptest.NewRequest("GET", "http://localhost:4280/favicon.svg", nil))
	if res.Code != http.StatusOK || res.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("favicon response = %d, %v", res.Code, res.Header())
	}
	if len(Icon()) == 0 || !bytes.Equal(res.Body.Bytes(), Icon()) {
		t.Fatal("favicon differs from the menu bar icon")
	}
}

func TestPageDrivesControlClient(t *testing.T) {
	socket, err := os.MkdirTemp("/tmp", "hc-ui-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socket) })
	t.Setenv("TMUX_TMPDIR", socket)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-server").Run() })
	for _, args := range [][]string{
		{"-f", "/dev/null", "new-session", "-d", "-s", "plain"},
		{"new-session", "-d", "-s", "work"},
		{"set-option", "-t", "work", "@hypercube_state", "/state.json"},
	} {
		if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
			t.Fatalf("tmux %v: %v\n%s", args, err, out)
		}
	}
	server := httptest.NewServer(Handler())
	defer server.Close()

	foreign, _ := http.NewRequest("GET", server.URL, nil)
	foreign.Host = "example.com"
	if res, err := http.DefaultClient.Do(foreign); err != nil || res.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign host: %v %v", res.Status, err)
	}

	res, err := http.Get(server.URL + "/events")
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

	// The client attached to the marked session and answered the command.
	got := make(chan bool)
	go func() {
		for {
			line, err := events.ReadString('\n')
			if err != nil {
				return
			}
			text, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(strings.TrimSpace(line), "data: "))
			if string(text) == "work" {
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

func TestActionsRejectBadRequests(t *testing.T) {
	server := httptest.NewServer(Handler())
	defer server.Close()
	for path, want := range map[string]int{
		"/window?action=rm":                              http.StatusBadRequest,
		"/workspace?action=rm":                           http.StatusBadRequest,
		"/workspace?action=delete&state=/missing/x.json": http.StatusNotFound,
	} {
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
}
