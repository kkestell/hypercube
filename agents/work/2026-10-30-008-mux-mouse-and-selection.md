# Mux mouse, scrollback, and selection

## Plan

`agents/plans/2026-10-30-008-mux-mouse-and-selection.md`

## Summary

The attached client turns on SGR mouse reporting (modes 1002 and 1006) and the server routes it: clicks on the session bar, window bar, title rows, and pane bodies select; panes that asked for the mouse get it encoded; other panes scroll with the wheel and select with a left drag, copying to the outer clipboard through OSC 52 on release. Each pane keeps about 10,000 lines. The plan's goal is met.

## Departures from the plan

- `mouseEvent.held` is named `noButton`. The plan's comment ("no button") contradicts the name `held`.
- `TestWheelScrolls` expects `\x1b[4;1H\x1b[0;39;49m75`, not `\x1b[4;1H75`: the first cell of a frame always writes its SGR after the cursor move.
- `TestMouseGoesToPane` waits for the pane to print `ready` after it enables mouse tracking. Without it the click can arrive before the mode is set and goes to selection.

## Decisions

- A release that selects only blank cells clears the selection but copies nothing, since an empty OSC 52 payload could clear the user's clipboard.
- Any new press ends a drag already in progress, closing its anchor.
- Motion after scrollback pruning discards the anchor's cell is ignored (comment, no code beyond the early return on the `Snapshot` error).
- `ViewportActive` and the other terminal getters panic on error, matching the surrounding code.

## Checks run

- `make check` — passes (vet, gofmt, `go test -race` for the root and `mux`).
- `go test -race -count=5 ./cmd/mux -run 'Scrollback|Wheel|MouseGoes|DragCopies|ClickSel|ClickSess'` — passes.

## Manual verification

Not run: the by-hand checks in the plan (vim with `mouse=a`, a real outer terminal's clipboard, clicks printing nothing after detach) need a real terminal. The tests drive the same reports through the client stream.

## Follow-up work

- A pane that leaves the shown window mid-drag keeps its highlight until its next press (commented in `endDrag`).
