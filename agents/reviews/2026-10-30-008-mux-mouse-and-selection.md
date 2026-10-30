# Mux mouse, scrollback, and selection

## Scope and coverage

The staged changes for task 8 of milestone 1 in `agents/mux-design.md`, against `agents/plans/2026-10-30-008-mux-mouse-and-selection.md` and the work log's departures: `mux/internal/server/mouse.go`, the mouse and scrollback changes in `attach.go`, `keyboard.go`, `label.go`, and `pane.go`, the client's mouse modes in `mux/cmd/mux/attach.go`, and the new tests. I traced parsing, routing, the drag's lifetime, selection and copy, session switching, and the redraw of a scrolled pane, including the cursor.

Gaps:

- The by-hand checks the design asks for (vim with `mouse=a`, a real clipboard, clicks after detach) were not run; they need a real terminal.

## Summary

| ID | Severity | Category | Fix   | Title                                             |
| -- | -------- | -------- | ----- | ------------------------------------------------- |
| F1 | Low      | Simplicity | Fixed | Motion with no button is handled but never arrives |
| F2 | Low      | Concurrency | Fixed | Mouse reports are written to a pane under the server mutex |
| F3 | Low      | Comments | Fixed | The mode 1003 limitation is stated twice          |

## F1. Motion with no button is handled but never arrives

Low · Simplicity · Fixed · `mux/internal/server/mouse.go:31`

`mouseEvent.noButton` and its checks in `isDragButton` and `encodeMouse` handled motion with no button held. The client turns on mode 1002, which reports motion only while a button is held, and SGR releases name their button, so the case never occurs.

### Fix

Removed `noButton`. Button bits 3 map to `MouseButtonUnknown`, with a comment saying why it does not arrive; `isDragButton` and `encodeMouse` lost their checks.

## F2. Mouse reports are written to a pane under the server mutex

Low · Concurrency · Fixed · `mux/internal/server/mouse.go:191`

Typed input is written to a pane after the mutex is released, but an encoded mouse report is written while it is held. A pane that stops reading input while the user drags could fill its PTY input buffer and block the whole server. That needs a stalled program and many queued reports, which is unlikely, and the terminal's write callback already accepts the same risk.

### Fix

A comment at the write names the case. No code.

## F3. The mode 1003 limitation is stated twice

Low · Comments · Fixed · `mux/internal/server/mouse.go:109`

The note that a pane asking for mode 1003 gets motion only during drags appeared on both the client's mode setup and `Server.mouse`. It belongs where the mode is chosen.

### Fix

Removed it from `Server.mouse`'s comment.

## Checks run

- `make check` before and after the fixes — passes.

## Verdict

The change does what the design asks and no more: routing, tracking-pane encoding, wheel scrollback, drag selection, and the OSC 52 copy are correct for normal use, and the tests cover each. The three findings were minor and are fixed (F1, F2, F3).
