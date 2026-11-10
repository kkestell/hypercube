package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kkestell/hypercube/internal/config"
	"github.com/kkestell/hypercube/internal/harness"
)

func TestReviewSettings(t *testing.T) {
	t.Parallel()
	models := []config.ModelEntry{
		{Harness: "claude", Model: "opus", Efforts: []string{"low", "high"}, DefaultEffort: "high"},
		{Harness: "codex", Model: "sol"},
	}
	chosen := harness.Settings{Harness: "codex", Model: "sol", Effort: "low"}
	review := harness.Settings{Harness: "claude", Model: "opus", Effort: "low"}
	for _, test := range []struct {
		name string
		cfg  config.Config
		want harness.Settings
	}{
		{"review selection", config.Config{ReviewSelection: &review, DefaultSelection: &chosen, Models: models}, review},
		{"default selection", config.Config{DefaultSelection: &chosen, Models: models}, chosen},
		{"first model", config.Config{Models: models}, harness.Settings{Harness: "claude", Model: "opus", Effort: "high"}},
	} {
		if got, err := reviewSettings(test.cfg); err != nil || got != test.want {
			t.Errorf("%s: reviewSettings = %+v, %v; want %+v", test.name, got, err, test.want)
		}
	}
	if _, err := reviewSettings(config.Config{}); err == nil {
		t.Error("no models: reviewSettings succeeded")
	}
}

func TestReviewPrompt(t *testing.T) {
	t.Parallel()
	got := reviewPrompt("Review #{number} ({url}); #{number} is at {url}. {other}", 7, "https://github.com/o/r/pull/7")
	want := "Review #7 (https://github.com/o/r/pull/7); #7 is at https://github.com/o/r/pull/7. {other}"
	if got != want {
		t.Errorf("reviewPrompt = %q; want %q", got, want)
	}
}

func TestReviewNeedsANumber(t *testing.T) {
	t.Parallel()
	h := handler(t, open(t))
	for _, number := range []string{"", "seven", "0", "-1"} {
		req := httptest.NewRequest("POST", "http://localhost:4280/session?action=review&workspace=w&number="+number, nil)
		req.Header.Set("Origin", "http://localhost:4280")
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusBadRequest {
			t.Errorf("number %q: status %d %s", number, res.Code, res.Body)
		}
	}
}
