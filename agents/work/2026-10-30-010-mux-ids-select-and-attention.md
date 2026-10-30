# Mux IDs, select, server start, and pane attention

## Plan

`agents/plans/2026-10-30-010-mux-ids-select-and-attention.md`

## Summary

IDs now carry a per-server random tag (`p7-4f2a`), `select` and `mux select <id>` exist, only `mux spawn` and a bare `mux attach` start a server (`client.DialOrStart`), and `attention` takes only a pane, with windows and sessions showing the bell while any of their panes is marked. The plan's goal is met.

## Departures from the plan

- `mux/internal/server/pane.go` changed too: the `pane` struct lives there, so the `attention` field moved there.
- `attach` now dials before it reads the terminal size, so `mux attach <session>` with no server fails with "no server is running" even without a terminal.
- `waitFrame` in the tests now returns the frame it matched, so `TestAttention` can check a bell is absent.

## Decisions

- `selectTarget` looks the pane up with its own loop over the tree, because it needs the pane's window and session together.

## Checks run

- `make check` — passes.
- `go test -race -count=5 -run 'TestSelect|TestAttention|TestNoServer|TestKillCommand|TestBellAndTitle' ./cmd/mux` (in `mux/`) — passes.

## Manual verification

None beyond the tests; they run the built `mux` against their own state directory and server.

## Follow-up work

None.
