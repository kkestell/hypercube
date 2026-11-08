# Backend composition in the command

## Plan

`agents/plans/2026-11-07-010-backend-composition.md`

## Summary

The command now composes backend HTTP routes and owns the shared access check.
The UI handler serves browser routes only, without MCP or desktop imports.
Dispatch, backend lifecycle, HTTP composition, and alerts have separate files;
`programs.go` remains unchanged. The architecture reference already describes
the browser package accurately and needed no update.

## Departures from the plan

The proposed nested mux omitted POST from the `Allow` header on unsupported
`/mcp` methods. A small exact-path fallback in HTTP composition preserves the
original `GET, HEAD, POST` header and GET/HEAD browser fallthrough. The plan and
regression cases now record this requirement.

## Checks run

- Focused backend HTTP tests — passed, including access checks, version,
  framing, fallthrough, and unsupported MCP methods with matching Origin.
- `GOCACHE=/tmp/hypercube-shared-gocache go test ./cmd/hypercube ./internal/ui`
  — UI passed; backend process test was blocked by the sandbox's `/bin/ps`
  restriction. That test passed in the full check outside the sandbox.
- `GOCACHE=/tmp/hypercube-shared-gocache go list -f '{{join .Imports "\n"}}'
  ./internal/ui` — no MCP or desktop imports.
- `GOCACHE=/tmp/hypercube-shared-gocache make check` outside the sandbox —
  browser and website builds, TypeScript, 368 browser tests, 28 desktop tests,
  Go build, vet, formatting, and all Go race packages except Pi passed.
  `TestAppProcess` passed unchanged. Pi's unchanged
  `TestStopCancelsPendingCall` timed out on startup `get_state` after 60 seconds.
- `GOCACHE=/tmp/hypercube-shared-gocache go test -race ./plugins/pi -run
  '^TestStopCancelsPendingCall$' -count=1` outside the sandbox — passed.
- `GOCACHE=/tmp/hypercube-shared-gocache go test -race ./plugins/pi -count=1`
  outside the sandbox — passed. No plugin code changed; the initial timeout
  and passing reruns are recorded rather than repeating unaffected checks.
- `git diff --check` — passed.

The initial default-cache test attempt was blocked by sandbox access to
`~/Library/Caches/go-build`; the checks use the writable temporary cache above.

## Manual verification

Compared moved functions directly with `git show HEAD:cmd/hypercube/main.go`
and the access check with `git show HEAD:internal/ui/ui.go`. `close`, `serveUI`,
`sendAlert`, and `localOnly` are identical. `startApp` differs only in the HTTP
handler call and extraction of the alert goroutine; its order and error paths
are unchanged. `watchAlerts` preserves the goroutine body, with the desktop
parameter renamed and its failure comment lifted to the function comment.
