# Backend composition in the command

## Goal

`ui.Handler` serves the browser view, and it also mounts the agents' MCP
route, the desktop app's connection, and the build identity, and wraps all of
them in the host and origin checks. The browser package therefore imports
`mcp` and `desktop` and decides access rules for callers that are not
browsers. Meanwhile `cmd/hypercube/main.go` holds command dispatch, backend
startup and shutdown, alert delivery, and the HTTP server in one file.

When this task is done:

- `cmd/hypercube/http.go` builds the backend's HTTP handler: it mounts
  `/mcp`, `/desktop`, and `/version`, hands every other path to
  `ui.Handler`, and applies `localOnly` once around everything.
- `ui.Handler(db, rt)` serves browser routes only and imports neither `mcp`
  nor `desktop`.
- `cmd/hypercube` is split into files by responsibility: `main.go`
  (dispatch), `programs.go`, `backend.go` (startup, shutdown, `serveUI`),
  `http.go`, and `alerts.go`.

Routes, methods, status codes, headers, access rules, startup order, and
shutdown order are unchanged.

Prerequisites: the [configuration](2026-11-07-006-application-config.md),
[protocol](2026-11-07-007-harness-protocol-boundary.md),
[MCP bridge](2026-11-07-008-mcp-bridge-executable.md), and
[holder](2026-11-07-009-holder-executable.md) plans. Addresses review
finding F5.

## Related code

- `cmd/hypercube/main.go` after the earlier plans:
  - `usage`, `main` (dispatch of `hold`, `version`, `ui`, `mcp`),
    `parsePort`.
  - `app` (`db`, `rt`, `server`, `desktop`, `cancel`, `done`, `once`).
  - `startApp(listener, programs, desktop)`: `config.Upgrade`,
    `store.Open`, `session.New`, `rt.ScanHarnesses`, the server with
    `ui.Handler(db, rt, desktop)`, `rt.Restore` with a callback that starts
    `server.Serve`, then a goroutine running `rt.WatchAlerts` with
    `sendAlert`.
  - `sendAlert(desktop, alert)` — reads notification preferences and calls
    `desktop.Alert`.
  - `(*app).close(quit)` — `rt.Quit` or `rt.Detach`, `desktop.Shutdown`,
    `server.Close`, `cancel`, `db.Close`, once.
  - `serveUI(args)` — port, listener, signal subscription before
    `startApp`, then waits for SIGINT (quit), SIGTERM (detach), or a server
    error.
- `internal/ui/ui.go` — package comment ("serves the browser view and the
  app's routes ... Agents reach the tools through /mcp"), `server`,
  `Handler(db, rt, desktop)`, the inline `GET /version` handler, `POST /mcp`,
  `GET /desktop`, `localOnly`, `refused`. Imports `buildinfo`, `desktop`,
  `mcp`, `net`.
- `internal/ui/ui_test.go` — `handler` (line 54) builds
  `Handler(db, session.New(...), &desktop.Desktop{})`; `TestVersion` (113),
  `TestRoutesRefuseOtherSites` (121), `TestPageRefusesForeignFrames` (169)
  test what moves; `TestFavicon` and the rest test browser routes.
- `internal/ui/terminal_test.go:43`, `ui_test.go:304`,
  `submissions_test.go:18` — other `Handler` calls.
- `internal/desktop/desktop.go` — `Desktop` is an `http.Handler` for the
  desktop app's WebSocket; `Shutdown` closes it.
- `cmd/hypercube/integration_test.go` `TestAppProcess` — exercises the real
  backend over HTTP, including `/version`, `/desktop`, and `/terminal`.

## Decisions

### Files in `cmd/hypercube`

| File | Contents |
| --- | --- |
| `main.go` | Package comment, `usage`, `main`, `parsePort` |
| `programs.go` | `programs()` (from the earlier plans) |
| `backend.go` | `app`, `startApp`, `(*app).close`, `serveUI` |
| `http.go` | `handler`, `version`, `localOnly` |
| `alerts.go` | `watchAlerts`, `sendAlert` |

The backend stays in the `main` package. It has one caller, and
`TestAppProcess` already runs it as a process; a separate package would add
an exported API for no second user.

### HTTP composition

`cmd/hypercube/http.go`:

```go
// handler serves the backend: the browser view, the agents' tools at /mcp,
// the desktop app's connection at /desktop, and the build identity at
// /version. localOnly guards every route.
func handler(db *store.DB, rt *session.Runtime, d *desktop.Desktop) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /mcp", mcp.Handler(db, rt))
	mux.Handle("GET /desktop", d)
	mux.HandleFunc("GET /version", version)
	mux.Handle("/", ui.Handler(db, rt))
	return localOnly(mux)
}

// version answers with the build identity, which the desktop app and
// installers compare with the installed executable's.
func version(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintln(w, buildinfo.String())
}
```

`localOnly` moves from `internal/ui/ui.go` unchanged, with its comment: it
refuses other host names, refuses requests from other origins except to
`/mcp` (which requires a known session in `X-Hypercube-Session`), requires
actions to name their origin, and sets `X-Frame-Options: SAMEORIGIN`.

Requests that matched no route in the single mux still get the same answer:
`GET /mcp` and unknown paths fall through `/` to the browser mux, whose
`GET /` file server answers 404, and a wrong method on a browser route gets
405 from the browser mux.

`startApp` builds the server with `handler(db, rt, desktop)`.

`internal/ui/ui.go`:

```go
// Package ui serves the browser view: the page, its data WebSocket, its
// actions, files, previews, and panel terminals. The data WebSocket carries
// Hypercube's data from the database, and the transcripts of the session the
// page selects. Browsers cap a host's HTTP connections at six; WebSockets are
// outside that cap, so open pages never stall each other's requests.
package ui

// Handler serves the browser view's routes.
func Handler(db *store.DB, rt *session.Runtime) http.Handler
```

`Handler` registers the same browser routes as today, minus `/version`,
`/mcp`, and `/desktop`, and returns the mux. The `buildinfo`, `desktop`,
`mcp`, and `net` imports go.

### Alerts

`cmd/hypercube/alerts.go`:

```go
// watchAlerts passes the runtime's alerts to the desktop app until ctx ends.
// A failed alert is reported and the watch goes on.
func watchAlerts(ctx context.Context, rt *session.Runtime, d *desktop.Desktop)
```

Its body is the goroutine body from `startApp`; `startApp` runs
`go watchAlerts(ctx, rt, desktop)` at the same point. `sendAlert` moves
unchanged.

### Startup and shutdown

`startApp` keeps its order: upgrade the configuration, open the store, build
the runtime, scan harnesses, build the server, restore (starting the server
from the callback, so restored plugins can call `/mcp` before restore
returns), then watch alerts. `close` keeps its order: quit or detach the
runtime, shut down the desktop connection, close the server, cancel the
alert context, close the database. `serveUI` keeps subscribing to signals
before `startApp`.

## Test plan

- New `cmd/hypercube/http_test.go`, using `handler` over a temporary store
  (`XDG_STATE_HOME` in `t.TempDir()`), `session.New(db, "",
  session.Programs{})`, and `&desktop.Desktop{}`:
  - `TestVersion` — moved from `internal/ui`: 200, body
    `buildinfo.String()+"\n"`, `Cache-Control: no-store`.
  - `TestRoutesRefuseOtherSites` — moved: foreign host, rebound host on
    `/mcp`, foreign-origin input, input without origin, and terminal from a
    foreign host answer 403; an origin-less `POST /mcp` from `127.0.0.1` is
    not refused by `localOnly`; WebSockets to `/state`, `/terminal`, and
    `/desktop` from a foreign origin answer 403.
  - `TestPageRefusesForeignFrames` — moved: `GET /` has
    `X-Frame-Options: SAMEORIGIN`.
  - `TestRouteFallthrough` — new: `GET /mcp` answers 404, `POST /version`
    answers 405, and `POST /mcp` with an unknown session answers 403 from
    the MCP handler.
- `internal/ui` tests pass with `Handler(db, rt)`; `TestFavicon`,
  `TestConfig`, `TestActions`, the terminal and transcript tests are
  unchanged apart from the call.
- `TestAppProcess` passes unchanged: `/version` matches `hypercube version`,
  tools work through `/mcp`, the desktop connection opens and closes without
  disturbing agents, and the SIGTERM, SIGKILL, and SIGINT behavior holds.
- `go list -f '{{join .Imports "\n"}}' ./internal/ui` lists no `internal/mcp`
  or `internal/desktop`.

## Implementation plan

1. Create `cmd/hypercube/http.go` with `handler`, `version`, and
   `localOnly`. Narrow `ui.Handler` and its package comment as above.
2. Move `TestVersion`, `TestRoutesRefuseOtherSites`, and
   `TestPageRefusesForeignFrames` to `cmd/hypercube/http_test.go` and add
   `TestRouteFallthrough`. Update the remaining `Handler` calls in
   `internal/ui` tests.
3. Create `cmd/hypercube/alerts.go` with `watchAlerts` and `sendAlert`.
4. Move `app`, `startApp`, `close`, and `serveUI` to
   `cmd/hypercube/backend.go`, with `startApp` calling `handler` and
   `watchAlerts`. Leave `main.go` with the package comment, `usage`,
   `main`, and `parsePort`, and trim each file's imports.
5. Compare the moved `startApp`, `close`, and `serveUI` line by line with
   the originals for order and error handling.
6. Run `make check`.

## Documentation updates

- `agents/architecture.md` line 112 links the browser interface to
  `internal/ui/ui.go`, which still defines it. If the surrounding text says
  the UI package serves `/mcp` or the desktop connection, point it at
  `cmd/hypercube/http.go`.
