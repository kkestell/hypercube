# Put backend assembly and lifetime in focused command files

## Goal and prerequisites

Keep the backend in `cmd/hypercube`, with focused files for startup/shutdown,
HTTP composition, and alerts. `internal/ui` owns browser routes and assets;
it no longer assembles backend MCP tools, the desktop connection, build
identity, and common request protection.

Prerequisites: the [configuration](2026-11-07-006-application-config.md),
[protocol](2026-11-07-007-harness-protocol-boundary.md),
[MCP](2026-11-07-008-mcp-bridge-executable.md), and
[holder](2026-11-07-009-holder-executable.md) source changes. This plan
addresses [review finding F5](../reviews/2026-11-07-007-runtime-boundaries.md#f5-browser-handlers-compose-unrelated-backend-endpoints).
It does not own build or distribution integration.

## Related code

- `cmd/hypercube/main.go` currently contains CLI dispatch, `app`, `startApp`,
  `sendAlert`, resource close, and `serveUI`. Earlier source plans add helper
  entry points and inject explicit programs but leave backend assembly here.
- `internal/ui/ui.go:Handler` registers browser routes, `/version`, `/mcp`,
  and `/desktop`, and applies `localOnly` around the complete mux.
- `internal/ui/ui_test.go` tests version and cross-client request protection
  alongside browser behavior. Other UI fixtures construct this broad handler.
- `internal/desktop/desktop.go` owns the connection and shutdown. Backend
  close already shuts that connection before closing the server and database.
- `cmd/hypercube/integration_test.go` exercises the real backend, agents,
  panel shells, desktop disconnection, and signal/restart behavior.

## Decisions

Keep command dispatch, argument parsing, program location, and compatibility
commands in focused command files. Move existing backend construction and
resource ownership to `backend.go`, HTTP assembly/protection to `http.go`, and
notification preference/delivery code to `alerts.go`. Keep the private
`startApp` test seam introduced by existing tests, now receiving explicit
programs. Do not add `internal/backend`, an exported serve API, service
wrappers, or generic interfaces for a single caller.

The command's outer mux mounts `POST /mcp`, `GET /desktop`, and `GET /version`,
then delegates the remaining routes to `ui.Handler(db, rt)`. Move `localOnly`
to that composition and apply it once around the entire handler. Preserve
methods, not-found and method-not-allowed behavior, localhost host checks,
origin checks, the specific `/mcp` exception and known-session check, and
`SAMEORIGIN` framing protection. The browser and desktop keep the same routes.
The UI package must not import MCP or desktop to compose them.

Preserve startup order: upgrade config; open store; construct runtime; scan
plugins; register saved launches; start HTTP service; then adopt readers and
deliver queued work. Restoring plugins may call MCP before restoration ends.
Keep session transaction/lifecycle ownership together.

Move shutdown without changing its behavior or order: the runtime quits or
detaches, the desktop connection shuts down, the HTTP server closes, the
alert context is cancelled, and SQLite closes. Do not add a watcher completion
channel, a wait for it, or a new shutdown guarantee. The current connection
shutdown already releases blocked desktop writes. Keep signal subscription
visibly owned by `serveUI`.

## Implementation plan

1. Split the existing command by the responsibilities above, preserving
   private names where they remain useful. Keep explicit executable programs
   and compatibility dispatch from the prior plans intact. Inspect whole
   files, not just moved blocks, and remove unused imports/comments.
2. Move the outer mux, `/version` implementation, and `localOnly` into the
   command. Narrow `ui.Handler` and its package comment to browser routes.
   Update backend composition and all UI handler callers/fixtures without
   leaving a second wrapper or compatibility handler in UI.
3. Move version and cross-client protection assertions to command tests using
   the real composition. Keep browser route/socket/content tests in UI and
   backend tool semantics in `internal/mcp`. Avoid replacing production
   composition with a copied mux in a UI test merely to preserve old setup.
4. Compare the moved startup, error paths, notification watch, and close
   sequence with their current implementation. Preserve their behavior;
   do not redesign restoration, runtime shutdown, desktop transport, or alert
   delivery during this responsibility move.

## Acceptance and verification

- Command composition tests preserve version identity and cache headers,
  foreign host/origin rejection, browser actions and data/terminal/desktop
  socket protection, frame header, `/mcp` origin exception and known-session
  validation, and route methods/not-found behavior. Relocate existing
  assertions and add only missing regressions consequential to the mux move.
- Existing backend process tests preserve MCP callbacks during operation,
  desktop-disconnect independence, agent/shell survival over SIGTERM, and
  deliberate termination over SIGINT. UI tests keep browser behavior without
  importing unrelated backend endpoints.
- Review the moved shutdown sequence directly against the existing order and
  retain current desktop shutdown coverage. No watcher wait or new
  connected-desktop/backpressure test belongs to this code movement.
- Inspect final imports and command files: UI does not compose MCP/desktop;
  one outside protection covers every route; backend resources have visible
  owners. No new backend package or generic abstraction is introduced.
- Run focused command, UI, MCP, and desktop tests affected by the move, then
  `make check` before a code commit. Record results or omissions in the work
  log and stage the source task for fresh review. No repeated packaging,
  signing, installation, or screenshot checks are needed for this source
  move; the next plan verifies the combined operational result.

Next plan: [Runtime integration](2026-11-07-011-runtime-integration.md).
