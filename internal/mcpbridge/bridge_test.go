package mcpbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// backend serves a tool that reports the session header, refusing other sessions.
func backend() http.Handler {
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		session := r.Header.Get("X-Hypercube-Session")
		server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
		mcp.AddTool(server, &mcp.Tool{Name: "whoami"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: session}}}, nil, nil
		})
		return server
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Hypercube-Session") != "s" {
			http.Error(w, "unknown session", http.StatusForbidden)
			return
		}
		handler.ServeHTTP(w, r)
	})
}

const script = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"c","version":"1"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"whoami","arguments":{}}}
{"jsonrpc":"2.0","id":3,"method":"no/such"}
`

type answer struct {
	ID     int `json:"id"`
	Result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	} `json:"result"`
	Error *json.RawMessage `json:"error"`
}

func answers(t *testing.T, output string) []answer {
	t.Helper()
	var list []answer
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		var a answer
		if err := json.Unmarshal([]byte(line), &a); err != nil {
			t.Fatalf("bridge wrote %q: %v", line, err)
		}
		list = append(list, a)
	}
	return list
}

func TestRun(t *testing.T) {
	server := httptest.NewServer(backend())
	defer server.Close()
	var out bytes.Buffer
	if err := Run(t.Context(), server.URL, "s", strings.NewReader(script), &out); err != nil {
		t.Fatal(err)
	}
	got := answers(t, out.String())
	if len(got) != 3 || got[0].ID != 1 || got[0].Error != nil || got[1].ID != 2 ||
		got[1].Error != nil || got[1].Result.IsError || len(got[1].Result.Content) != 1 ||
		got[1].Result.Content[0].Text != "s" || got[2].ID != 3 || got[2].Error == nil {
		t.Fatalf("answers = %s", out.String())
	}
}

func TestRunAnswersEachRequestWhenRefused(t *testing.T) {
	server := httptest.NewServer(backend())
	defer server.Close()
	for _, tc := range []struct{ name, url, session string }{
		{"unknown session", server.URL, "nobody"},
		{"bad backend URL", ":invalid", "s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := Run(t.Context(), tc.url, tc.session, strings.NewReader(script), &out); err != nil {
				t.Fatal(err)
			}
			got := answers(t, out.String())
			if len(got) != 3 {
				t.Fatalf("answers = %s", out.String())
			}
			for i, a := range got {
				if a.ID != i+1 || a.Error == nil {
					t.Errorf("answer %d = %+v", i, a)
				}
			}
		})
	}
}

func TestRunWaitsForRestartingBackend(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, "http://"+addr, "s", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"whoami","arguments":{}}}`+"\n"), &out)
	}()
	// The first refused attempt waits for the replacement backend.
	select {
	case err := <-done:
		t.Fatalf("bridge gave up before backend started: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	listener, err = net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: backend()}
	go server.Serve(listener)
	defer server.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("bridge did not resume")
	}
	got := answers(t, out.String())
	if len(got) != 1 || got[0].ID != 1 || got[0].Error != nil ||
		len(got[0].Result.Content) != 1 || got[0].Result.Content[0].Text != "s" {
		t.Fatal(out.String())
	}
}

func TestMainNeedsEnvironment(t *testing.T) {
	for _, tc := range []struct{ name, url, session string }{
		{"no URL", "", "s"},
		{"no session", "http://localhost", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HYPERCUBE_URL", tc.url)
			t.Setenv("HYPERCUBE_SESSION_ID", tc.session)
			if err := Main(strings.NewReader(""), &bytes.Buffer{}); err == nil || err.Error() != "HYPERCUBE_SESSION_ID and HYPERCUBE_URL must be set" {
				t.Fatalf("missing environment: %v", err)
			}
		})
	}
}
