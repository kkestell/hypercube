# Mux bars and default labels

## Plan

`agents/plans/2026-10-30-006-mux-bars-and-labels.md`

## Summary

An attached client now draws the session bar, the window bar, and a title row above each shown pane. Each item shows a default label: the pane's OSC 0/2 title, or its command name. A window shows its first pane's label and a session its first window's. An exited pane's title row reads `label (exited N)`. A title change, spawn, kill, or respawn redraws every attachment. The plan's goal is met.

## Checks run

- `make check` — passes.
- `go test -race -count=5 ./cmd/mux` (in `mux/`) — passes.

## Manual verification

None beyond the tests; the three new tests read the drawn frames from a real attach.
