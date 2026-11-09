package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/config"
	"github.com/kkestell/hypercube/internal/harness"
)

// lead is the settings of the app's first session.
var lead = harness.Settings{Harness: "test", Model: "m", Effort: "low"}

func TestSaveConfig(t *testing.T) {
	a := newApp(t)
	cfg := config.Config{DefaultSelection: &harness.Settings{Harness: "test", Model: "next"}, ChildGuidance: "Use next for reviews.",
		Theme: "light", TerminalFontSize: 14, Notifications: config.Notifications{Requests: true},
		Models: []config.ModelEntry{{Harness: "test", Model: "next", Title: "Next", Efforts: []string{"low", "high"}, DefaultEffort: "high"}}}
	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a.post("/config", string(body), http.StatusNoContent)
	got, err := config.Load()
	if err != nil || !reflect.DeepEqual(got, cfg) {
		t.Fatalf("saved config = %+v, %v", got, err)
	}
	// Invalid request bodies leave the last saved configuration untouched.
	for name, body := range map[string]string{
		"broken JSON":        `{`,
		"incomplete default": strings.Replace(string(body), `"model":"next"`, `"model":""`, 1),
		"unknown setting":    strings.Replace(string(body), `{`, `{"font":"Menlo",`, 1),
		"second document":    string(body) + `{}`,
		"repeated model":     strings.Replace(string(body), `"models":[`, `"models":[{"harness":"test","model":"next","title":"Again","efforts":[],"default_effort":""},`, 1),
		"entry id":           strings.Replace(string(body), `"models":[{`, `"models":[{"id":"e",`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			a.post("/config", body, http.StatusBadRequest)
			got, err := config.Load()
			if err != nil || !reflect.DeepEqual(got, cfg) {
				t.Fatalf("invalid save changed config: %+v, %v", got, err)
			}
		})
	}
}

func TestSaveConfigFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.WriteFile(filepath.Join(dir, "hypercube"), []byte("blocked"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := handler(t, open(t))
	req := httptest.NewRequest("POST", "http://localhost:4280/config", strings.NewReader(`{"theme":"system","terminal_font_size":13}`))
	req.Header.Set("Origin", "http://localhost:4280")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusInternalServerError || !strings.Contains(res.Body.String(), "saving config") {
		t.Fatalf("failed save = %d %s", res.Code, res.Body)
	}
}
