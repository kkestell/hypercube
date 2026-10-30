# Mux keys, keyboard mode, and attach rules

## Goal

Task 7 of milestone 1 in `agents/mux-design.md`. An attached client gets the
rest of the built-in keys: Ctrl-b `n` and `p` for the next and previous
window, `0`-`9` for a window by number, `o` for the next pane, and Ctrl-b
Ctrl-b for a literal Ctrl-b. The prefix works whatever keyboard mode the
focused pane uses. The outer terminal's keyboard mode follows the focused
pane. Attaching to a session that has a client detaches that client.
`mux attach` with no session attaches to the first session, or spawns
`$SHELL` in a new session when none exists. Reattaching redraws the whole
screen and returns to the window and pane the session last showed.

## Related code

- `mux/internal/server/attach.go`
  - `attachment` holds the client's session, size, connection, wake channel,
    and `prefix` flag. `a.window()` returns `session.windows[0]` and
    `a.focus()` returns the window's first pane; both carry "Task 7 adds"
    comments.
  - `attach` adds the attachment and lays out the session; `layoutSession`
    picks the first attachment it finds and says task 7 detaches the earlier
    client.
  - `stream` passes input through `a.filter` to `a.focus().ptmx`.
  - `filter` handles only the legacy Ctrl-b byte and `d`.
  - `drawFrame` takes the cursor from pane 0 (`if i == 0`).
- `mux/internal/server/server.go` — `session` and `window` structs; `spawn`,
  `kill`, and `respawn` change the tree. `kill` closes the connections of
  attachments whose session is gone.
- `mux/cmd/mux/attach.go` — the terminal client. It enters the alternate
  screen and restores it on exit.
- `mux/cmd/mux/main.go` — `run` requires exactly one argument for `attach`;
  `spawn` builds a spawn request from `$SHELL`, the working directory, and the
  environment.
- libghostty, checked with a scratch program in `mux/build/_probe`:
  - `Terminal.KittyKeyboardFlags()` returns the pane's current kitty flags.
  - `Terminal.Mode(ModeDECCKM)`, `Mode(ModeKeypadKeys)` (set by `ESC =`), and
    `Mode(ModeBracketedPaste)` return those modes.
  - modifyOtherKeys has no getter. A `KeyEncoder` set with
    `SetOptFromTerminal`, then `SetOptKittyFlags(KittyKeyDisabled)`, encodes a
    Ctrl-b press as `\x1b[27;5;98~` when the pane set `CSI > 4;2 m` and as
    `\x02` otherwise. libghostty tracks only level 2; level 1 does not change
    how Ctrl-b or plain keys are encoded.
- `mux/cmd/mux/main_test.go` — `attachAt`, `waitFrame`, `waitDetached`,
  `waitCapture`, `spawnPane`, `setTitle`, and `TestKillEndsAttach`, which
  reads until EOF.

## Decisions

- **The session remembers what it shows.** `session` gets `active *window`
  and `window` gets `active *pane`. `a.window()` returns `a.session.active`
  and `a.focus()` returns `a.window().active`. Since a session has one
  client, keeping this on the session and window means a reattach returns to
  the same place, and no attachment state has to move on reattach.
  - `spawn`: a new session's `active` is its first window; a new window's
    `active` is its first pane. Spawning into an existing window or session
    leaves `active` alone.
  - `kill`: after removing targets, a remaining session whose `active` window
    is gone gets its first window, and a window whose `active` pane is gone
    gets its first pane.
  - `respawn`: a window whose `active` is the old pane gets the new one.
- **Window and pane keys.** `n` and `p` wrap around the session's windows.
  A digit selects that window by its position, matching the window bar's
  numbers; a digit past the last window does nothing. `o` moves to the next
  pane in the window and wraps. Switching windows needs no layout, since
  `layoutSession` already sizes every window in the session. `o` can focus a
  pane that does not fit the terminal and so is not drawn; that is unlikely
  and gets a comment.
- **Keys reach the pane as typed.** The client keeps the outer terminal in
  the focused pane's keyboard mode, so the server does not re-encode keys. It
  only has to find the prefix and the key after it, which it decodes from
  these forms in any mode:
  - one byte: Ctrl-b is `0x02`, other keys are their byte;
  - `CSI code[:alternates] ; mods[:event] [; text] u` (kitty), where `mods`
    is 1 plus a bitmask (shift 1, alt 2, ctrl 4) and `event` is 1 press, 2
    repeat, or 3 release;
  - `CSI 27 ; mods ; code ~` (modifyOtherKeys).
  Other escape sequences are one opaque key each: any other CSI sequence,
  `ESC O x`, and `ESC x`. Caps Lock and Num Lock bits (64 and 128) are
  ignored in `mods`.
- **Prefix state.** `filter` reads one key at a time:
  - With no prefix pending, a Ctrl-b press sets `prefix`; every other key's
    bytes pass to the pane.
  - With the prefix pending, kitty repeat and release events and kitty
    modifier keys (codes 57441 to 57454) are dropped, so panes that ask for
    event types or all keys as escape codes still get the prefix. The next
    press ends the prefix. Another Ctrl-b passes its own bytes, which are the
    literal Ctrl-b in the pane's mode. A key with no modifiers is a command.
    Anything else is dropped.
  - `filter(data) (pass []byte, cmd rune, rest []byte)` stops at a command
    so `stream` can write `pass` to the current pane, run the command, and
    filter `rest`, which may go to another pane after a switch.
  - Commands without a built-in action are dropped, at the one place `bind`
    will later turn them into `key` events.
  - Comments, no code: a key sequence split across two reads passes through
    undecoded; a paste containing `0x02` starts the prefix; with kitty event
    types, the release of a command key goes to the pane focused after the
    command, and with all keys as escape codes the Ctrl press before the
    prefix reaches the pane.
- **Keyboard mode in each frame.** `drawFrame` writes the focused pane's
  modes right after the clear at the start of every frame, so the cursor
  stays last and the mode follows focus changes and the pane's own changes
  without tracking what was sent. In this order:
  - `\x1b[?1h` or `\x1b[?1l` (cursor keys);
  - `\x1b=` or `\x1b>` (keypad);
  - `\x1b[?2004h` or `\x1b[?2004l` (bracketed paste, which decides how a
    paste reaches the pane);
  - `\x1b[=F;1u` with the pane's kitty flags `F`;
  - `\x1b[>4;2m` or `\x1b[>4;0m` (modifyOtherKeys).
  Focus reporting and mouse modes are not mirrored; the mouse is task 8.
- **Client reset.** Before leaving the alternate screen, the client writes
  `\x1b[=0;1u\x1b[>4;0m\x1b[?1l\x1b>\x1b[?2004l`, so the outer shell gets
  its keys back in the default mode however the stream ended.
- **One client per session.** `attach` closes the connection of any
  attachment to the same session and removes it from `s.attached` before
  adding the new one, the same way `kill` ends an attachment. The old
  client's stream ends, its cleanup finds the new client in `layoutSession`,
  and the old client restores the terminal and exits.
- **`mux attach` with no session.** The client decides, with no new request:
  it calls `list`, attaches to the first session, or spawns `$SHELL` in the
  working directory with its environment when the list is empty and attaches
  to the new session. A session killed between the list and the attach
  returns the usual unknown-session error.
- **Reattach.** Every frame is already a full redraw, and `attach` signals a
  frame at once, so reattaching redraws with no change beyond `active`.

## Test plan

New tests in `mux/cmd/mux/main_test.go`, attached at 100 by 30.

- `TestWindowKeys`: session with two windows whose panes set titles `one`
  and `two`. `\x02n` gives a window bar `\x1b[2;1H 0 one \x1b[7m 1 two
  \x1b[0m`; `\x02n` again wraps to `\x1b[2;1H\x1b[7m 0 one \x1b[0m 1 two `;
  `\x02p` wraps back to window 1; `\x020` selects window 0.
- `TestNextPaneKey`: two `cat` panes in one window. `\x02o` then `routed\r`
  reaches the stacked pane and not the main one, and a frame shows the
  stacked pane's title row `\x1b[3;42H\x1b[7m cat \x1b[0m`. `\x02o` again
  sends `back\r` to the main pane.
- `TestLiteralPrefix`: a `cat -v` pane. `\x02\x02\r` shows `^B` in its
  capture.
- `TestPrefixInKeyboardModes`: `\x1b[98;5u\x1b[98;5:3ud` detaches
  (`waitDetached`); after reattaching, `\x1b[27;5;98~d` detaches.
- `TestKeyboardModeFollowsFocus`: a main pane running `sh -c 'printf
  "\033[?1h\033=\033[?2004h\033[>1u\033[>4;2m"; exec sleep 60'` and a stacked
  `sleep 60` pane. A frame contains
  `\x1b[?1h\x1b=\x1b[?2004h\x1b[=1;1u\x1b[>4;2m`; after `\x02o`, a frame
  contains `\x1b[?1l\x1b>\x1b[?2004l\x1b[=0;1u\x1b[>4;0m`.
- `TestSecondAttachDetachesFirst`: two clients attach to one `cat` session.
  The first reads to EOF; input from the second reaches the pane.
- `TestReattachRedraws`: two windows titled `one` and `two`; `\x02n`, then
  `\x02d` and `waitDetached`. A new attach's frames show window 1 current in
  the window bar and the pane's output.

`TestInputGoesToMainPane` and `TestCursorAndOffsets` keep passing unchanged:
after a kill the first remaining pane becomes active, and the frame still ends
with the cursor.

By hand, with the mux built by `make -C mux build` and `XDG_STATE_HOME` under
`mux/build/`: the window and pane keys; Ctrl-b Ctrl-b in `cat -v`; Ctrl-b d
and modified keys such as Shift-Enter in a pane running Claude Code (kitty
protocol) and in vim (modifyOtherKeys); arrows and paste in the outer shell
after detaching; a second terminal's `mux attach` detaching the first;
`mux attach` with no server spawning a shell; detaching and reattaching
redrawing the same window.

## Implementation plan

1. In `mux/internal/server/server.go`, add `active` to `session` and
   `window`, and set and repair it in `spawn`, `kill`, and `respawn`.
2. Add `mux/internal/server/keyboard.go` with the key decoding, `filter`
   (moved from `attach.go`), and `appendKeyboardModes(b []byte, p *pane)
   ([]byte, error)`, which reads the modes and probes modifyOtherKeys with a
   `KeyEncoder` and `KeyEvent` it creates and closes.
3. In `mux/internal/server/attach.go`:
   - `a.window()` and `a.focus()` return the `active` fields.
   - `stream` loops over `filter` results and runs commands through a new
     `(s *Server) command(a *attachment, cmd rune) (detach bool)`, which
     takes the mutex, changes `active`, and signals the attachment.
   - `attach` detaches an earlier client of the session.
   - `drawFrame` writes the keyboard modes after the clear and takes the
     cursor from the focused pane's rect, hiding it when that pane is not
     drawn.
   - Drop the "task 7" notes from the comments, including `layoutSession`'s.
4. In `mux/cmd/mux/attach.go`, write the keyboard reset before leaving the
   alternate screen, and choose or spawn a session when none is given.
5. In `mux/cmd/mux/main.go`, accept `mux attach [session]` and update the
   usage text.
6. In `mux/cmd/mux/main_test.go`, add the tests.
