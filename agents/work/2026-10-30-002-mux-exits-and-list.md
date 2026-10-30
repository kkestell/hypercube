# Mux exits and `list`

## Plan

`agents/plans/2026-10-30-002-mux-exits-and-list.md`

## Summary

A pane whose process exits stays in its window with `alive` false and its exit status (128 plus the signal number for a signal death). `list` and `mux ls` show the whole tree. `spawn` adds a window to a session or a pane to a window, and `mux spawn` takes `-s`, `-w`, and `-c`. Unknown IDs return errors. The plan's goal is met.

## Decisions

- The test helper that spawns through the client is named `spawnPane`, because `spawn` is already the command function in `package main`.
- `mux ls` starts a server if none is running, like every other client command, and then prints nothing.

## Checks run

- `make check` — passes (top-level, including `mux`).
- `go test -race -count=5` on the new tests and `TestErrors` — passes.

## Manual verification

None beyond the tests. `TestLsCommand` runs the built `mux` binary for `spawn -w -c` and `ls`.
