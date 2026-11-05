package mcp

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeBrowser stands in for the window's browsers. It records the session of
// each call; a script of "throw" fails.
type fakeBrowser struct{ sessions []string }

func (b *fakeBrowser) Navigate(_ context.Context, session, url string) (string, string, error) {
	b.sessions = append(b.sessions, session)
	return url + "/final", "Title of " + url, nil
}

func (b *fakeBrowser) Eval(_ context.Context, session, script string) (string, error) {
	b.sessions = append(b.sessions, session)
	if script == "throw" {
		return "", errors.New("ReferenceError: x is not defined")
	}
	return `"ran ` + script + `"`, nil
}

func (b *fakeBrowser) Close(_ context.Context, session string) error {
	b.sessions = append(b.sessions, session)
	return nil
}

func TestBrowserTools(t *testing.T) {
	db, rt := family(t, false, t.TempDir(), t.TempDir())
	browser := &fakeBrowser{}
	server := httptest.NewServer(Handler(db, rt, browser))
	defer server.Close()

	// Top-level and child sessions both drive their own browsers.
	for _, id := range []string{"session-1", "b"} {
		s := connect(t, server.URL, id)
		if text, failed := callTool(t, s, "browser_navigate", map[string]any{"url": "https://example.com"}); failed || text != `{"url":"https://example.com/final","title":"Title of https://example.com"}` {
			t.Errorf("%s: browser_navigate = %q, failed %v", id, text, failed)
		}
		if text, failed := callTool(t, s, "browser_eval", map[string]any{"script": "return 1"}); failed || text != `"ran return 1"` {
			t.Errorf("%s: browser_eval = %q, failed %v", id, text, failed)
		}
		if text, failed := callTool(t, s, "browser_eval", map[string]any{"script": "throw"}); !failed || !strings.Contains(text, "ReferenceError") {
			t.Errorf("%s: a throwing script = %q, failed %v", id, text, failed)
		}
		if text, failed := callTool(t, s, "browser_close", map[string]any{}); failed || text != "closed" {
			t.Errorf("%s: browser_close = %q, failed %v", id, text, failed)
		}
	}
	want := "session-1 session-1 session-1 session-1 b b b b"
	if got := strings.Join(browser.sessions, " "); got != want {
		t.Errorf("calls came from %q, want %q", got, want)
	}
}
