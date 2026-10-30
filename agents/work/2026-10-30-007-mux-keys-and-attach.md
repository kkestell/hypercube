# Mux keys and attach rules

## Plan

`agents/plans/2026-10-30-007-mux-keys-and-attach.md`

## Summary

An attached client now has Ctrl-b `n`, `p`, `0`-`9`, `o`, and Ctrl-b Ctrl-b. The prefix is recognized in legacy, kitty, and modifyOtherKeys input. Each frame sets the outer terminal's keyboard mode to the focused pane's, and the client resets it on exit. A second attach detaches the first client, `mux attach` with no session attaches to the first session or spawns `$SHELL`, and a reattach returns to the window and pane last shown. The plan's goal is met.

## Decisions

- `parseKey` treats a kitty `CSI code u` with no modifier field as an unmodified press, which the plan did not spell out.
- `prefixKey` moved to `keyboard.go` with `filter`.

## Checks run

- `make check` — passes.
- `go test -race -count=5 ./cmd/mux` (in `mux/`) — passes.

## Manual verification

`mux attach` with no server, in a pseudo-terminal at 100 by 30 with `XDG_STATE_HOME=mux/build/state7` and `SHELL=/bin/sh`: it spawned a shell, ran `echo hi-$((1+1))`, and Ctrl-b Ctrl-b then Ctrl-b d detached and wrote the keyboard reset before leaving the alternate screen. Not checked by hand: Claude Code and vim keyboard modes, a second terminal's attach, and the outer shell's arrows and paste after detaching.

## Follow-up work

None.
