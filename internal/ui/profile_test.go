package ui

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kkestell/hypercube/internal/harness"
)

func TestSessionProfileAction(t *testing.T) {
	a := newApp(t)
	config := `{"profiles":[
 {"name":"next","description":"Next","harness":"test","model":"next","effort":"high"},
 {"name":"partial","description":"Partial","harness":"test","model":"partial","effort":"high"},
 {"name":"reject","description":"Reject","harness":"test","model":"reject","effort":"high"},
 {"name":"foreign","description":"Other","harness":"other","model":"m","effort":"low"}]}`
	if err := os.WriteFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "hypercube/config.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	p := a.page()
	p.send(`{"select":"` + a.session.ID + `"}`)
	shown := func() shownSession {
		if len(p.state.Workspaces) == 0 || len(p.state.Workspaces[0].Sessions) == 0 {
			return shownSession{}
		}
		return p.state.Workspaces[0].Sessions[0]
	}
	p.until("ready", func() bool { return shown().Ready })
	for _, profile := range []string{"unknown", "foreign"} {
		a.post("/session?action=profile&id="+a.session.ID+"&profile="+profile, "", http.StatusConflict)
	}
	a.post("/session?action=profile&id="+a.session.ID+"&profile=next", "", http.StatusOK)
	p.until("idle selection", func() bool { return shown().Model == "next" && shown().Effort == "high" })
	a.input("!work", http.StatusOK)
	p.until("working", func() bool { return shown().Activity == "busy" })
	a.post("/session?action=profile&id="+a.session.ID+"&profile=partial", "", http.StatusConflict)
	p.until("partial selection and error", func() bool {
		_, failed := p.find(a.session.ID, func(it harness.Item) bool {
			return it.Kind == harness.KindError && strings.Contains(it.Text, "effort rejected")
		})
		return failed && shown().Model == "partial" && shown().Effort == "low" && shown().Ready
	})
	a.post("/session?action=profile&id="+a.session.ID+"&profile=reject", "", http.StatusConflict)
	p.until("refusal", func() bool {
		_, failed := p.find(a.session.ID, func(it harness.Item) bool {
			return it.Kind == harness.KindError && strings.Contains(it.Text, "model rejected")
		})
		return failed
	})
	if shown().Model != "partial" || shown().Effort != "low" {
		t.Fatal(shown())
	}
	a.post("/session?action=stop&id="+a.session.ID, "", http.StatusOK)
	p.until("unready", func() bool { return !shown().Ready })
	a.post("/session?action=profile&id="+a.session.ID+"&profile=next", "", http.StatusConflict)
}
