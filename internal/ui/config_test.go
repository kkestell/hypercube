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

	"github.com/kkestell/hypercube/internal/harness"
	"github.com/kkestell/hypercube/internal/store"
)

func TestSaveConfig(t *testing.T) {
	a := newApp(t)
	cfg := harness.Config{DefaultProfile: "next", Profiles: []harness.Profile{
		{Name: "next", Description: "Updated profile", Harness: "test", Model: "next", Effort: "high"},
	}}
	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a.post("/config", string(body), http.StatusNoContent)
	got, err := harness.LoadConfig()
	if err != nil || !reflect.DeepEqual(got, cfg) {
		t.Fatalf("saved config = %+v, %v", got, err)
	}
	// Removing the original profile does not reconfigure an existing session.
	if err := a.db.Read(func(tx *store.Tx) error {
		s, err := tx.Session(a.session.ID)
		if err == nil && (s.Model != "m" || s.Effort != "low") {
			t.Fatalf("existing session changed: %+v", s)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	next, err := a.rt.Create(a.session.Workspace, got.DefaultProfile, false, "next session")
	if err != nil || next.Model != "next" || next.Effort != "high" {
		t.Fatalf("new session = %+v, %v", next, err)
	}
	// Invalid request bodies leave the last saved configuration untouched.
	for name, body := range map[string]string{
		"broken JSON":     `{`,
		"empty profiles":  `{"profiles":[]}`,
		"missing default": strings.Replace(string(body), `"next"`, `"missing"`, 1),
		"unfinished":      `{"profiles":[{"name":"unfinished"}]}`,
		"unknown setting": strings.Replace(string(body), `{`, `{"theme":"light",`, 1),
		"second document": string(body) + `{}`,
		"duplicate names": `{"profiles":[` + string(mustProfileJSON(t, cfg.Profiles[0])) + `,` + string(mustProfileJSON(t, cfg.Profiles[0])) + `]}`,
	} {
		t.Run(name, func(t *testing.T) {
			a.post("/config", body, http.StatusBadRequest)
			got, err := harness.LoadConfig()
			if err != nil || !reflect.DeepEqual(got, cfg) {
				t.Fatalf("invalid save changed config: %+v, %v", got, err)
			}
		})
	}
}

func mustProfileJSON(t *testing.T, profile harness.Profile) []byte {
	t.Helper()
	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSaveConfigFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.WriteFile(filepath.Join(dir, "hypercube"), []byte("blocked"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := handler(t, open(t))
	req := httptest.NewRequest("POST", "http://localhost:4280/config", strings.NewReader(`{"profiles":[{"name":"one","description":"Jobs","harness":"codex","model":"sol","effort":"high"}]}`))
	req.Header.Set("Origin", "http://localhost:4280")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusInternalServerError || !strings.Contains(res.Body.String(), "saving config") {
		t.Fatalf("failed save = %d %s", res.Code, res.Body)
	}
}
