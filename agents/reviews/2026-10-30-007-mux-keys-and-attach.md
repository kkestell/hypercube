# Mux keys, keyboard mode, and attach rules

## Scope and coverage

The staged changes for task 7 of milestone 1 in `agents/mux-design.md`, against
`agents/plans/2026-10-30-007-mux-keys-and-attach.md`: `keyboard.go` (key
decoding, `filter`, keyboard modes), the `active` window and pane in
`server.go` and their repair in `spawn`, `kill`, and `respawn`, the command
loop, one-client attach, and cursor placement in `attach.go`, the client's
reset and session choice in `cmd/mux/attach.go`, the `attach [session]`
argument, and the new tests.

The key decoder handles what the plan requires and nothing more: the
legacy byte, kitty `CSI … u` with alternates, event types, empty modifier
fields, and associated text, modifyOtherKeys `CSI 27 ; m ; c ~`, and other
escape sequences as one opaque key. The unlikely cases (split reads, a paste
containing Ctrl-b, a multibyte character after the prefix, a command key's
release, a pane that does not fit) carry comments and no code.

Gaps:

- Not exercised in a real terminal: Claude Code (kitty) and vim
  (modifyOtherKeys) panes, a second terminal's attach, and the outer shell's
  keys after detaching. The work log lists these as not checked by hand.

## Summary

No findings.

## Checks run

- `make check` — passes.
- A scratch test in `mux/internal/server` (removed afterward) fed `parseKey`
  and `filter` the legacy, kitty (with alternates, lock bits, event types,
  associated text, modifier keys), modifyOtherKeys, SS3, and CSI forms. Each
  decoded as intended; the prefix survived interleaved Ctrl presses and
  releases, Ctrl-b Ctrl-b passed one Ctrl-b, and an arrow after the prefix was
  dropped.

## Verdict

The change meets the plan with no findings. The hand checks in a real terminal
remain to be done.
