# Review of mux bars and default labels

## Scope and coverage

I reviewed the staged changes for task 6 of milestone 1 in `agents/mux-design.md`, against `agents/plans/2026-10-30-006-mux-bars-and-labels.md` and the work log `agents/work/2026-10-30-006-mux-bars-and-labels.md`. That covers the `label` methods in `mux/internal/server/label.go`, `signalAll`, `appendBar`, and the chrome in `drawFrame` in `attach.go`, the pane name, title callback, and exit wake in `pane.go`, the redraw calls in `spawn`, `kill`, and `respawn` in `server.go`, the constant comments in `layout.go`, and the three new tests in `mux/cmd/mux/main_test.go`.

I traced when each label change reaches each client: title changes inside `VTWrite` with the mutex held, spawns, kills (including attachments whose session the kill ends, which are removed before `signalAll`), respawns of live and exited panes, and exits of shown and hidden panes. I checked that no freed terminal is read for a label, that the chrome's resets leave the cell loop's SGR tracking starting from a known state, that the divider and the title rows do not overlap, and that the frame still ends with the cursor.

Gaps:

- Nothing was checked by hand in a live terminal, which the design's task 6 check asks for; the frames were checked through the tests.

## Summary

No findings.

## Checks run

- `make check` from the repository root: passes.
- `go test -race -count=3 -run 'TestTitleLabels|TestCommandNameLabels|TestExitedTitle|TestCursorAndOffsets' ./cmd/mux` in `mux/`: passes.

## Verdict

The change meets the plan and the design's default-label rules. Both bars and every shown pane's title row draw the OSC title or the command name, windows and sessions take their first child's label, an exited pane's title row shows its status, and every client redraws when a label or the tree changes. The two unlikely cases, wide characters in a title and a full reset clearing a hidden pane's title, carry comments. Nothing needs fixing; the by-hand check remains.
