# Mux `kill` and server exit

## Plan

`agents/plans/2026-10-30-003-mux-kill-and-server-exit.md`

## Summary

`kill` removes a pane, window, or session, hangs up its processes, and ends attachments that show a removed pane. Removing the last session removes the socket, replies, and exits the server with status 0; the next `mux spawn` starts a new server. `mux kill <id>` picks the field from the ID's first letter. The plan's goal is met.

## Decisions

- The test helper that sends `kill` is named `killTarget`, because `kill` is the command function in `package main`.
- `Server.kill` removes panes with one pass over the tree that matches a pane, window, or session ID. IDs are never empty, so the two request fields left empty match nothing. It validates the ID first with the existing `find*` functions.
- `TestKillEndsAttach` requires `io.EOF` from the attached stream. `dial`'s timeout closes the client's own side, which gives a different error, so EOF shows the server closed the connection.

## Checks run

- `make check` — passes.
- `go test -race -count=5` on the kill, attach, and error tests — passes.

## Manual verification

None beyond the tests. `TestKillCommandStopsServer` runs the built `mux` binary for `spawn`, `kill`, and a second `spawn`.
