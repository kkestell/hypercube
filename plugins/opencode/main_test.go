package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kkestell/hypercube/internal/harness"
)

type nativeCall struct {
	Method, Path string
	Body         map[string]any
}

type testHarness struct {
	*agent
	native  *httptest.Server
	mu      sync.Mutex
	events  []harness.Event
	calls   []nativeCall
	changed chan struct{}
	wire    chan map[string]any
	reply   func(http.ResponseWriter, *http.Request, map[string]any) bool
}

func connected(t *testing.T) *testHarness {
	t.Helper()
	h := &testHarness{changed: make(chan struct{}, 1), wire: make(chan map[string]any, 64)}
	h.agent = newAgent(func(e harness.Event) {
		h.mu.Lock()
		h.events = append(h.events, e)
		h.mu.Unlock()
		select {
		case h.changed <- struct{}{}:
		default:
		}
	})
	h.session = "s1"
	h.native = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "opencode" || password != "test-password" {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/api/event" {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"type\":\"server.connected\",\"data\":{}}\n\n")
			w.(http.Flusher).Flush()
			for {
				select {
				case e := <-h.wire:
					b, _ := json.Marshal(e)
					fmt.Fprintf(w, "data: %s\n\n", b)
					w.(http.Flusher).Flush()
				case <-r.Context().Done():
					return
				}
			}
		}
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		h.mu.Lock()
		h.calls = append(h.calls, nativeCall{r.Method, r.URL.RequestURI(), body})
		h.mu.Unlock()
		if h.reply != nil && h.reply(w, r, body) {
			return
		}
		var data any
		switch r.URL.Path {
		case "/api/command":
			data = []any{map[string]string{"name": "review"}}
		case "/api/plugin":
			data = []any{map[string]any{"state": map[string]string{"status": "active"}}}
		case "/api/model":
			data = []any{
				map[string]any{"providerID": "p", "id": "m", "name": "Model", "enabled": true, "variants": []any{map[string]string{"id": "low"}, map[string]string{"id": "high"}}, "limit": map[string]int{"context": 100000}},
				map[string]any{"providerID": "q", "id": "b/c", "name": "No effort", "enabled": true},
				map[string]any{"providerID": "p", "id": "disabled", "enabled": false},
			}
		case "/api/experimental/generate/text":
			data = map[string]any{"text": "Fix the parser"}
		case "/api/model/default":
			data = map[string]any{"providerID": "p", "id": "m"}
		case "/api/session", "/api/session/s1", "/api/session/source/fork":
			data = map[string]any{"id": "s1", "location": map[string]string{"directory": "/work"}, "model": map[string]string{"providerID": "p", "id": "m", "variant": "high"}}
		default:
			w.WriteHeader(204)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	h.server = &server{url: h.native.URL, password: "test-password", client: h.native.Client()}
	t.Cleanup(func() {
		h.cancel()
		if h.streamDone != nil {
			<-h.streamDone
		}
		h.native.Close()
	})
	return h
}

func (h *testHarness) feed(t *testing.T, kind string, data map[string]any) {
	t.Helper()
	if data == nil {
		data = map[string]any{}
	}
	if _, ok := data["sessionID"]; !ok && kind != "form.created" {
		data["sessionID"] = "s1"
	}
	b, _ := json.Marshal(map[string]any{"type": kind, "data": data})
	var e nativeEvent
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatal(err)
	}
	if err := h.handle(e); err != nil {
		t.Fatal(err)
	}
}

func (h *testHarness) sent(path string) []nativeCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []nativeCall
	for _, c := range h.calls {
		if c.Path == path {
			out = append(out, c)
		}
	}
	return out
}

func (h *testHarness) snapshot() []harness.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]harness.Event{}, h.events...)
}

func (h *testHarness) states() []string {
	var states []string
	for _, e := range h.snapshot() {
		if e.State != "" {
			states = append(states, e.State)
		}
	}
	return states
}

func (h *testHarness) item(id string) harness.Item {
	var found harness.Item
	for _, e := range h.snapshot() {
		for _, it := range e.Items {
			if it.ID == id {
				if it.Append {
					it.Text = found.Text + it.Text
				}
				found = it
			}
		}
	}
	return found
}

func waitFor(t *testing.T, h *testHarness, check func() bool) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for !check() {
		select {
		case <-h.changed:
		case <-timer.C:
			t.Fatal("timed out waiting for native event")
		}
	}
}

func TestCorrectionsAreAdmittedWhileAToolRuns(t *testing.T) {
	h := connected(t)
	if err := h.subscribe(h.ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.input("original", nil); err != nil {
		t.Fatal(err)
	}
	h.wire <- map[string]any{"type": "session.execution.started", "data": map[string]any{"sessionID": "s1"}}
	h.wire <- map[string]any{"type": "session.tool.input.started", "data": map[string]any{"sessionID": "s1", "assistantMessageID": "m1", "id": "tool", "name": "shell"}}
	waitFor(t, h, func() bool { return h.item("m1:tool").Kind == harness.KindTool })
	for _, text := range []string{"correction one", "correction two"} {
		if err := h.input(text, nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := h.sent("/api/session/s1/prompt"); len(got) != 3 || got[1].Body["text"] != "correction one" || got[2].Body["text"] != "correction two" {
		t.Fatalf("native admissions: %+v", got)
	}
	for _, c := range h.sent("/api/session/s1/prompt") {
		if c.Body["delivery"] != "steer" {
			t.Fatalf("not steering: %+v", c)
		}
	}
	if h.item("m1:tool").Done || !reflect.DeepEqual(h.states(), []string{harness.Working}) {
		t.Fatal("correction ended the tool or invented a turn boundary")
	}
	if len(h.sent("/api/session/s1/interrupt?resume=false")) != 0 {
		t.Fatal("correction cancelled execution")
	}
	h.wire <- map[string]any{"type": "session.tool.success", "data": map[string]any{"sessionID": "s1", "assistantMessageID": "m1", "id": "tool", "content": []any{map[string]string{"type": "text", "text": "finished normally"}}}}
	h.wire <- map[string]any{"type": "session.step.started", "data": map[string]any{"sessionID": "s1", "assistantMessageID": "m2"}}
	h.wire <- map[string]any{"type": "session.text.ended", "data": map[string]any{"sessionID": "s1", "assistantMessageID": "m2", "ordinal": 0, "text": "corrected response"}}
	h.wire <- map[string]any{"type": "session.execution.succeeded", "data": map[string]any{"sessionID": "s1"}}
	waitFor(t, h, func() bool { return len(h.states()) == 2 })
	if it := h.item("m1:tool"); !it.Done || it.Failed || it.Output != "finished normally" {
		t.Fatalf("tool: %+v", it)
	}
	ev := h.snapshot()
	if !reflect.DeepEqual(ev[len(ev)-1].Final, []string{"m2:text:0"}) {
		t.Fatalf("final: %+v", ev[len(ev)-1])
	}
}

func TestInputImagesAndAdmissionFailure(t *testing.T) {
	h := connected(t)
	if err := h.input("look", []harness.Image{{MIMEType: "image/png", Data: "AAAA"}}); err != nil {
		t.Fatal(err)
	}
	got := h.sent("/api/session/s1/prompt")[0].Body
	if got["files"].([]any)[0].(map[string]any)["uri"] != "data:image/png;base64,AAAA" {
		t.Fatal(got)
	}
	h.reply = func(w http.ResponseWriter, r *http.Request, _ map[string]any) bool {
		if strings.HasSuffix(r.URL.Path, "/prompt") {
			w.WriteHeader(409)
			fmt.Fprint(w, `{"message":"not admitted"}`)
			return true
		}
		return false
	}
	if err := h.input("rejected", nil); err == nil || !strings.Contains(err.Error(), "not admitted") {
		t.Fatal(err)
	}
	if len(h.states()) != 0 {
		t.Fatal("admission fabricated execution state")
	}
}

func TestStopIsExplicitCancellation(t *testing.T) {
	h := connected(t)
	h.feed(t, "session.execution.started", nil)
	if err := h.interrupt(); err != nil {
		t.Fatal(err)
	}
	if len(h.sent("/api/session/s1/interrupt?resume=false")) != 1 {
		t.Fatal("Stop did not interrupt")
	}
	h.feed(t, "session.execution.interrupted", nil)
	for _, e := range h.snapshot() {
		if len(e.Final) > 0 {
			t.Fatal("interrupted turn has a final")
		}
		for _, it := range e.Items {
			if it.Kind == harness.KindError {
				t.Fatal("Stop shown as an error")
			}
		}
	}
}

func TestBeginResumeForkAndSelection(t *testing.T) {
	for _, tc := range []struct {
		name         string
		args         harness.StartArgs
		path, method string
	}{
		{"new", harness.StartArgs{}, "/api/session", "POST"},
		{"resume", harness.StartArgs{Conversation: "s1"}, "/api/session/s1", "GET"},
		{"fork", harness.StartArgs{Fork: "source", ForkDir: "/work"}, "/api/session/source/fork", "POST"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := connected(t)
			a := tc.args
			a.Dir = "/work"
			a.Model, a.Effort = "p/m", "high"
			if err := h.begin(h.ctx, a); err != nil {
				t.Fatal(err)
			}
			if c := h.sent(tc.path); len(c) != 1 || c[0].Method != tc.method {
				t.Fatalf("open: %+v", c)
			}
			if h.selected != (harness.Selection{Model: "p/m", Effort: "high"}) {
				t.Fatal(h.selected)
			}
			if h.window != 100000 {
				t.Fatal(h.window)
			}
		})
	}
	h := connected(t)
	h.reply = func(w http.ResponseWriter, r *http.Request, _ map[string]any) bool {
		if r.URL.Path == "/api/session/missing" {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"message":"not found"}`)
			return true
		}
		return false
	}
	if err := h.begin(h.ctx, harness.StartArgs{Conversation: "missing", Dir: "/work", Model: "p/m"}); err == nil || !strings.Contains(err.Error(), "could not resume") {
		t.Fatal(err)
	}
	if len(h.sent("/api/session")) != 0 {
		t.Fatal("resume silently created a replacement")
	}
}

func TestConfigurationAndCatalog(t *testing.T) {
	h := connected(t)
	for _, tc := range []struct {
		model, effort, want string
		fail                bool
	}{
		{"p/m", "high", "high", false}, {"q/b/c", "", "", false},
		{"p/m", "", "", true}, {"p/m", "default", "", true}, {"p/m", "unknown", "", true}, {"q/b/c", "high", "", true}, {"p/disabled", "", "", true},
	} {
		got, err := h.configure(harness.Selection{Model: tc.model, Effort: tc.effort})
		if tc.fail {
			if err == nil {
				t.Fatalf("accepted %s/%s", tc.model, tc.effort)
			}
			continue
		}
		if err != nil || got.Effort != tc.want || got.Model != tc.model {
			t.Fatalf("selection %+v, %v", got, err)
		}
	}
}

// fakeExecutable leaves the HTTP server in the test process while exercising
// the real launch, readiness, per-launch environment, and stdin-close lifecycle.
func fakeExecutable(t *testing.T, url string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "launch")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" \"$OPENCODE_PASSWORD\" \"$OPENCODE_CONFIG_CONTENT\" \"$(pwd)\" > \"$FAKE_LAUNCH_LOG\"\nprintf '%s\\n' '{\"url\":\"" + url + "\"}'\ncat >/dev/null\n"
	if err := os.WriteFile(filepath.Join(dir, "opencode"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_LAUNCH_LOG", log)
	return log
}

// owned makes the opencode executable start a server that is h's native server
// behind a proxy checking the launch password, and returns the launch log.
func owned(t *testing.T, h *testHarness) string {
	t.Helper()
	var log string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile(log)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		password := strings.Split(string(data), "\n")[1]
		if _, got, _ := r.BasicAuth(); got != password {
			t.Error("server password was not sent")
		}
		r.SetBasicAuth("opencode", "test-password")
		h.native.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(proxy.Close)
	log = fakeExecutable(t, proxy.URL)
	return log
}

func TestOwnedServerDiscoveryAndLaunch(t *testing.T) {
	h := connected(t)
	log := owned(t, h)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := models(ctx, harness.ModelsRequest{Dir: t.TempDir(), Model: "p/m"})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Models) != 2 || c.Default != "p/m" || !reflect.DeepEqual(c.Efforts, []string{"low", "high"}) {
		t.Fatalf("catalog: %+v", c)
	}
	if len(h.sent("/api/session")) != 0 {
		t.Fatal("discovery created a session")
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	if lines[0] != "serve --stdio --port 0" || len(lines[1]) != 64 {
		t.Fatalf("launch: %q", lines[:2])
	}
}
