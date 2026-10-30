# Mux mouse, scrollback, and selection

## Goal

Task 8 of milestone 1 in `agents/mux-design.md`. While a client is attached,
the outer terminal reports the mouse. A press on the session bar, the window
bar, or a pane's title row or body selects that session, window, or pane. In
a pane that requested mouse tracking, the mouse goes to the pane, encoded in
the mode and format it asked for. In any other pane, the wheel scrolls the
pane's scrollback, and a left-button drag selects text, shows it highlighted,
and on release copies it to the outer terminal's clipboard through OSC 52.
Each pane keeps about 10,000 lines of scrollback. Detaching turns the outer
terminal's mouse reporting off.

## Related code

- `mux/internal/server/keyboard.go` — `parseKey` already treats
  `CSI < b ; x ; y M` as one opaque key; `filter(data) (pass, cmd, rest)`
  splits input at the prefix command.
- `mux/internal/server/attach.go`
  - `attachment` holds the session, size, connection, wake channel, and
    `prefix`. The server's mutex guards it.
  - `attach` detaches any other client of the session, runs `stream`, and
    afterwards lays out the captured `sess`, which is wrong once a click can
    move the attachment to another session.
  - `stream` writes `pass` to the focused pane and runs commands.
  - `draw` encodes each frame; it is the only writer to the connection.
  - `drawFrame` builds the bar labels inline and draws cells with
    `appendSGR`; `appendBar` writes items as `" text "`, cut at the width.
- `mux/internal/server/layout.go` — `layout` and `rect` (0-based; the title
  row is `y-1`).
- `mux/internal/server/pane.go` — `startPane` creates the terminal with
  `WithMaxScrollbackLines(scrollback)`.
- `mux/cmd/mux/attach.go` — enters the alternate screen and writes the
  keyboard reset on exit.
- `mux/cmd/mux/main_test.go` — `attachAt`, `waitFrame`, `waitCapture`,
  `sendInput`, `titled`, `TestWindowKeys`, `TestNextPaneKey`.
- libghostty, checked with scratch tests in `mux/build/scratch`:
  - A terminal's scrollback also has a byte limit, which defaults to 10,000
    bytes; with it, `seq 20000` at 80 by 24 keeps about 1,060 lines.
    `SetScrollbackMaxBytes(nil)` removes it and leaves about 9,930 lines
    under the 10,000-line limit, which prunes by page.
  - `MouseEncoder` with `SetOptFromTerminal` and `SetOptSize{ScreenWidth:
    cols, ScreenHeight: rows, CellWidth: 1, CellHeight: 1}` encodes a left
    press at position (4.5, 2.5) in a pane with modes 1002 and 1006 as
    `\x1b[<0;5;3M`, button-held motion (with `SetOptAnyButtonPressed(true)`)
    as `\x1b[<32;…M`, and `MouseButtonFour` as `\x1b[<64;…M`. A position
    past the right edge is clamped to the last column.
  - `Terminal.MouseTracking()` reports whether the pane asked for the mouse.
  - `ScrollViewportDelta`, `ScrollViewportBottom`, and `ViewportActive`
    scroll the view. A scrolled view stays put when output arrives.
  - `TrackGridRef` on a viewport point gives an anchor that follows its cell
    as output scrolls the screen; `Snapshot` plus a fresh `GridRef` make a
    `Selection` with either endpoint first, and `SetSelection` installs it
    inclusive of both cells. Closing the tracked ref after the terminal is
    closed is safe.
  - `SelectionFormatString()` returns the selected text;
    `SetSelection(nil)` clears it.
  - After `RenderState.Update`, `RenderStateRowCells.Selected()` reports each
    selected cell.
  - `SelectionGesture` decides whether the press and end cells are included
    from the pointer's position inside the cell. The outer terminal reports
    whole cells only, so every centered position tried dropped the end cell;
    the plan builds selections directly instead.

## Decisions

- **Outer mouse mode.** The client writes `\x1b[?1002h\x1b[?1006h` (button
  events with motion while a button is held, in SGR format) after entering
  the alternate screen, and its exit reset adds `\x1b[?1002l\x1b[?1006l`.
  The mode is fixed, so frames do not carry it. Comment, no code: a pane that
  asks for motion with no button held (mode 1003) gets motion only during
  drags.
- **Parsing.** New `mux/internal/server/mouse.go` holds the mouse code.
  `parseMouse(raw []byte) (mouseEvent, bool)` decodes `CSI < b ; x ; y M|m`
  into:

  ```go
  // mouseEvent is one SGR mouse report from the outer terminal.
  type mouseEvent struct {
  	button  libghostty.MouseButton // left, middle, right, four (wheel up), five (wheel down), and so on
  	held    bool                   // no button: motion with nothing pressed
  	mods    libghostty.Mods
  	action  libghostty.MouseAction // press, release, or motion
  	x, y    int                    // 0-based terminal cell
  }
  ```

  The low two bits of `b` with bits 64 and 128 pick the button (3 means
  none), 4 is Shift, 8 Alt, 16 Ctrl, and 32 motion; final `m` is a release.
  `filter` returns `(pass []byte, cmd rune, mouse *mouseEvent, rest []byte)`
  and stops at a mouse report the way it stops at a command, so nothing of
  the report reaches a pane as keys. A mouse report leaves a pending prefix
  pending.
- **Routing.** `stream` hands each mouse event to
  `(s *Server) mouse(a *attachment, ev mouseEvent)`, which takes the mutex.
  It hit-tests the shown layout:
  - Row 0 and row 1: a left press selects the session or window under `x`.
  - A pane's title row: a left press focuses the pane.
  - A pane's body: a non-wheel press focuses the pane, then the event goes
    to the pane as below. Motion and releases go to the pane the press
    started in, whatever is under the pointer, so a drag stays in one pane.
  - Anything else, such as the divider, is dropped.
- **The drag.** The attachment gets `drag *drag`, set by a body press of the
  left, middle, or right button and cleared on release:

  ```go
  // drag is a button held down in a pane. Motion and the release go to the
  // pane, with positions clamped to its screen.
  type drag struct {
  	pane   *pane
  	anchor *libghostty.TrackedGridRef // the selection's start; nil when the pane gets the mouse
  }
  ```

  A drag whose pane is no longer in the shown window (killed, respawned, or
  another window shown) is dropped and its anchor closed. `attach` closes a
  remaining anchor when the stream ends.
- **Panes that ask for the mouse.** When `p.term.MouseTracking()` is true,
  the event is encoded with a `MouseEncoder` and `MouseEvent` created and
  closed per event, like `modifyOtherKeys` does with the key encoder:
  `SetOptFromTerminal(p.term)`, the size option above from `p.cols` and
  `p.rows`, `SetOptAnyButtonPressed(a.drag != nil)` for motion, and the
  position at the cell's center relative to the pane, clamped to its screen.
  The bytes go to `p.ptmx`. Wheel events go to the pane too.
- **Scrollback.** In a pane that does not ask for the mouse, a wheel-up press
  calls `ScrollViewportDelta(-3)` and wheel-down `ScrollViewportDelta(3)`,
  then signals the attachment. Comment, no code: a pane on the alternate
  screen has no scrollback, so the wheel does nothing there. In `stream`,
  when `pass` is not empty and the focused pane's view is scrolled
  (`ViewportActive` false), the view returns to the bottom with
  `ScrollViewportBottom` and the attachment is signalled, so typing shows the
  prompt. `startPane` calls `p.term.SetScrollbackMaxBytes(nil)` so the
  10,000-line limit is the one that applies.
- **Selection.** In a pane that does not ask for the mouse:
  - Left press: clear the pane's selection, and set `drag.anchor` to
    `TrackGridRef` of the pressed cell (`PointTagViewport`). A plain click
    selects nothing.
  - Motion: `SetSelection(&Selection{Start: *anchor.Snapshot(), End:
    *GridRef(viewport cell)})` with the cell clamped to the pane, then
    signal. A selection never crosses panes, matching the accepted
    limitation.
  - Release: if the terminal has a selection, take
    `SelectionFormatString()`, store it in `a.clipboard`, clear the
    selection, and signal; close the anchor.
  - Comment, no code: dragging past the top or bottom does not scroll; the
    wheel scrolls during a drag, and the next motion extends the selection.
  - Middle and right presses start a drag with no anchor and do nothing.
- **Highlight.** `drawFrame` reads `cells.Selected()` and passes it to
  `appendSGR(b, style, selected)`, which flips inverse video for selected
  cells (`s.Inverse() != selected`).
- **Copy.** `attachment.clipboard []byte` is text waiting to be copied. In
  `draw`, while holding the mutex, a non-nil `clipboard` is prepended to the
  frame as `\x1b]52;c;<standard base64>\x07` and cleared. `draw` stays the
  only writer to the connection, and no protocol change is needed.
- **Selecting sessions, windows, and panes.** The bar labels move into
  `sessionLabels(sessions)` and `windowLabels(sess)`, used by `drawFrame`
  and the hit test. `barItem(items []string, width, x int) int` returns the
  item at column `x` with the same widths `appendBar` uses (each item is its
  rune count plus two, cut at `width`), or -1.
  - Window: `sess.active = sess.windows[i]`, signal.
  - Pane: `w.active = p`, signal.
  - Session: `(s *Server) show(a *attachment, sess *session)` closes and
    removes any other attachment of `sess` (moved out of `attach`, which
    calls it too), sets `a.session`, drops the drag, and lays out the old and
    new sessions, so the old one goes back to 80 by 24. After its stream,
    `attach` lays out `a.session` instead of the captured `sess`.

## Test plan

New tests in `mux/cmd/mux/main_test.go`, attached at 100 by 30, so a single
pane's screen starts at terminal row 4 and is 27 rows tall.

- `TestScrollbackKeepsLines`: a pane running `sh -c 'seq 20000; exec sleep
  60'`. Once `capture` shows `20000`, its first line is a number from 9001
  to 11000, so about 10,000 lines were kept.
- `TestWheelScrolls`: a pane running `sh -c 'seq 100; exec cat'`. A frame
  has `\x1b[4;1H75`; after `\x1b[<64;10;10M` a frame has `\x1b[4;1H72`;
  after typing `x` a frame has `\x1b[4;1H75` again.
- `TestMouseGoesToPane`: a pane running `sh -c 'printf
  "\033[?1000h\033[?1006h"; exec cat'`. After `\x1b[<0;10;5M`, the capture
  shows the echoed report `[<0;10;2M`.
- `TestDragCopies`: a pane running `sh -c 'echo hello world; exec sleep
  60'`. `\x1b[<0;1;4M\x1b[<32;5;4M\x1b[<0;5;4m` gives a frame containing
  `\x1b]52;c;aGVsbG8=\x07`.
- `TestClickSelects`: two windows titled `one` and `two`. A press and release
  at column 10 of row 2 shows `\x1b[2;1H 0 one \x1b[7m 1 two \x1b[0m`. In
  the second window, add a stacked `cat` pane; a press at (60, 10) shows its
  title highlighted, `\x1b[3;42H\x1b[7m cat \x1b[0m`, and typed input
  reaches it.
- `TestClickSession`: two sessions with panes titled `one` and `two`,
  attached to the first. A press and release at column 8 of row 1 shows
  `\x1b[1;1H one \x1b[7m two \x1b[0m`, and `checkSize` shows the first
  session's pane back at `24 80`.

By hand, with `make -C mux build` and `XDG_STATE_HOME` under `mux/build/`:
`seq 20000` and wheel up through the scrollback, then type to return to the
bottom; a drag highlights text and pastes elsewhere after release; vim with
`mouse=a` places the cursor, scrolls, and drags a visual selection; clicks on
the session bar, window bar, title rows, and pane bodies; after detaching,
clicks in the outer shell print nothing.

## Implementation plan

1. In `mux/internal/server/pane.go`, remove the scrollback byte limit in
   `startPane`.
2. Add `mux/internal/server/mouse.go` with `mouseEvent`, `parseMouse`,
   `drag`, `(s *Server) mouse`, the encoding for panes that ask for the
   mouse, the wheel scrolling, the selection steps, and `barItem`.
3. In `mux/internal/server/keyboard.go`, make `filter` return mouse events.
4. In `mux/internal/server/attach.go`:
   - add `drag` and `clipboard` to `attachment`;
   - `stream` passes mouse events to `s.mouse` and returns a scrolled focused
     pane to the bottom when typing;
   - add `show`, use it in `attach`, lay out `a.session` after the stream,
     and close a remaining anchor;
   - `draw` prepends the OSC 52 copy;
   - `drawFrame` uses `sessionLabels` and `windowLabels` and highlights
     selected cells through `appendSGR`.
5. In `mux/cmd/mux/attach.go`, turn on mouse reporting after entering the
   alternate screen and add its reset to the exit sequence.
6. In `mux/cmd/mux/main_test.go`, add the tests.
