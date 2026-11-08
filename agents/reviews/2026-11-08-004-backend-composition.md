# Backend composition review

## Scope and coverage

Reviewed the staged implementation of
[plan 010](../plans/2026-11-07-010-backend-composition.md), its
[work log](../work/2026-11-08-002-backend-composition.md), and the parent plan
assessment. Read the whole changed Go files: command dispatch, backend
lifecycle, alerts, HTTP composition and tests, and browser route registration
and affected tests. Traced their callers and the runtime's restore, quit,
detach, and alert paths, desktop connection lifecycle, MCP access check, and
unchanged backend process test.

Independently ran the focused backend HTTP tests and full UI package under
the race detector. Did not repeat `make check` or `TestAppProcess`: the work
log reports the process test and all full-check steps passing except Pi's
unchanged `TestStopCancelsPendingCall`, which timed out during startup
`get_state`. It reports that test and the full Pi race package passing on
reruns. Those broader results are implementation evidence, not checks run in
this review. No independent Linux or live-app validation was performed.

## Summary

No findings. The initial review completed before writing this report; no
implementation edits were needed. Whole touched files remain easy to follow.

The command owns backend HTTP composition and the shared access check, while
the UI package serves browser routes without MCP or desktop imports. The
exact `/mcp` fallback is proportionate: it preserves GET/HEAD browser
fallthrough and the original `Allow: GET, HEAD, POST` on unsupported methods
without adding a routing abstraction.

## Evidence

- Compared moved functions with `HEAD`: `close`, `serveUI`, `sendAlert`, and
  `localOnly` are identical. `startApp` differs only in the HTTP handler call
  and extraction of the alert goroutine. Inspection confirms `watchAlerts`
  preserves that body, including error reporting. Startup, signal
  subscription, restore's server-start callback, and shutdown order hold.
- Compared the old single mux and new nested mux using the actual route
  registrations and marker handlers in a temporary Go program. All 208
  method/path cases matched status, headers, body, and wildcard values,
  including canonical redirects, escaped paths, browser routes, backend
  routes, and unsupported methods. This checks routing, not domain handler
  behavior; the focused tests exercise the real composed handler.
- The moved access and framing tests cover the outer boundary, and the new
  fallthrough cases exercise MCP refusal, GET/HEAD fallback, and the combined
  method header. The unchanged process test covers MCP tool access, version,
  desktop and terminal connections, and signal-driven lifecycle behavior.

## Checks run

- `GOCACHE=/tmp/hypercube-shared-gocache go test -race ./cmd/hypercube -run
  '^(TestVersion|TestRoutesRefuseOtherSites|TestPageRefusesForeignFrames|TestRouteFallthrough)$'
  -count=1` — passed.
- `GOCACHE=/tmp/hypercube-shared-gocache go test -race ./internal/ui -count=1`
  — passed.
- `GOCACHE=/tmp/hypercube-shared-gocache go list -f '{{join .Imports "\n"}}'
  ./internal/ui` — no MCP or desktop imports.
- Scripted function comparison and temporary routing comparison described
  above — passed.
- `git diff --cached --check` — passed before and after adding the review.

## Verdict

Approve. No fixes or unresolved review findings. The separately reported Pi
timeout and passing reruns remain recorded without changing unrelated plugin
code or repeating the whole suite.
