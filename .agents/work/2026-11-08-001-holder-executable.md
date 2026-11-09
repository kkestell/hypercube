# Holder executable

## Plan

[Holder executable](../plans/2026-11-07-009-holder-executable.md)

## Summary

The backend supplies the separate holder executable to agent and panel shell
launches through `session.Programs`. Existing holders attach as before, and
`hypercube hold` remains available during replacement by older backends.
Tests build and run the real holder instead of dispatching holder commands
inside test binaries.

## Departures from the plan

The session, MCP, and UI test binaries also run harness stand-ins. Their
`TestMain` now dispatches `harnesstest.Run` before building the holder;
their redundant `TestStandIn` functions are removed. Otherwise each plugin
invocation would build another holder: the first passing `make check` took
131.8 seconds in the session package. The plan records this adjustment.

## Checks run

- `make check` — passed: browser and website builds, TypeScript, browser and
  desktop tests, Go build, vet, formatting, and race-enabled Go tests.
  Repeated successfully after the stand-in adjustment; the macOS session
  package completed in 77.9 seconds.
- Linux: `scripts/linux/run bash -lc 'make ui && go test -race
  ./internal/hold/... ./internal/terminal ./internal/session ./internal/mcp
  ./internal/ui ./cmd/hypercube'` — passed, including holder survival, shell
  adoption, missing-holder socket cleanup, and backend restart continuity.
  The process test checks that every holder runs `hypercube-hold`.
- `go list -deps ./cmd/hypercube-hold` — its only Hypercube packages are
  the command and `internal/hold`.
- `os.Executable` search under `internal` and `plugins` — only tests and
  the harness test helper remain.
- `git diff --check` — passed.

The initial focused macOS run passed holder and terminal tests, but other
packages encountered sandbox restrictions on the Go build cache. Required
checks then ran with approved access. The first Linux invocation passed
holder and terminal tests but could not compile the backend because the
container omits ignored UI assets; the passing retry built them with
`make ui` first.

## Manual verification

No installed app or live harness was used. Isolated macOS and Linux process
tests exercise the actual executable, retained transcript, agent and shell
PIDs across SIGTERM/SIGKILL restarts, and shutdown on SIGINT.

## Follow-up work

Packaging and development build integration remain in plan 011. This
intermediate change is not ready to install by itself. Nothing was installed
or committed; independent review is next.
