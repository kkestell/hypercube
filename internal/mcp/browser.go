package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Browser drives each session's native browser window.
type Browser interface {
	// Navigate loads url in the session's browser, creating the browser if it
	// has none, and returns the page's URL and title once it has loaded.
	Navigate(ctx context.Context, session, url string) (finalURL, title string, err error)
	// Eval runs script as the body of an async function in the main frame of
	// the session's page, and returns JSON.stringify of the value it returns.
	Eval(ctx context.Context, session, script string) (string, error)
	// Close closes the session's browser, if it has one.
	Close(ctx context.Context, session string) error
}

// browserTimeout bounds a page load and a script, so a page that never
// finishes does not hold the agent's tool call forever.
const browserTimeout = 30 * time.Second

type navigateArgs struct {
	URL string `json:"url"`
}

type evalArgs struct {
	Script string `json:"script"`
}

// addBrowserTools gives the session its browser's tools.
func addBrowserTools(server *mcp.Server, browser Browser, self string) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "browser_navigate",
		Description: "Load a URL in your browser and wait until the page finishes loading. Your browser is a web page in its own native window, which the user sees; it keeps the user's logins. Returns the final URL and the page's title.",
	}, result(func(ctx context.Context, a navigateArgs) (string, error) {
		ctx, cancel := context.WithTimeout(ctx, browserTimeout)
		defer cancel()
		finalURL, title, err := browser.Navigate(ctx, self, a.URL)
		if errors.Is(err, context.DeadlineExceeded) {
			return "", fmt.Errorf("the page did not finish loading within %v", browserTimeout)
		}
		if err != nil {
			return "", err
		}
		b, err := json.Marshal(struct {
			URL   string `json:"url"`
			Title string `json:"title"`
		}{finalURL, title})
		return string(b), err
	}))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "browser_eval",
		Description: "Run JavaScript in your browser's page, as the body of an async function in its main frame: it can await and must return a value. Returns JSON.stringify of the returned value, and a thrown exception as an error. Use it to read the page, change it, and click its elements.",
	}, result(func(ctx context.Context, a evalArgs) (string, error) {
		ctx, cancel := context.WithTimeout(ctx, browserTimeout)
		defer cancel()
		text, err := browser.Eval(ctx, self, a.Script)
		if errors.Is(err, context.DeadlineExceeded) {
			return "", fmt.Errorf("the script did not finish within %v", browserTimeout)
		}
		return text, err
	}))
	mcp.AddTool(server, &mcp.Tool{
		Name:        "browser_close",
		Description: "Close your browser window when you are done with it. The next browser_navigate opens a new one.",
	}, result(func(ctx context.Context, _ struct{}) (string, error) {
		ctx, cancel := context.WithTimeout(ctx, browserTimeout)
		defer cancel()
		if err := browser.Close(ctx, self); err != nil {
			return "", err
		}
		return "closed", nil
	}))
}
