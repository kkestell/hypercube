# Mux bars and default labels

## Goal

Task 6 of milestone 1 in `agents/mux-design.md`. An attached client now draws
the screen chrome that task 5 left blank: the top bar lists the sessions, the
second bar lists the shown session's windows, and each shown pane has a title
row. Every item shows a default label. A pane's label is the title its program
set with OSC 0 or OSC 2, or else its command name. A window shows its first
pane's label, and a session shows its first window's label. A pane whose
process has exited shows its exit status in its title row.

## Related code

- `mux/internal/server/attach.go` — `drawFrame` clears the screen, draws the
  divider, then each pane's rows and the cursor. `wake` signals the
  attachments whose shown window holds a pane; `signal` signals one.
  `a.window()` and `a.focus()` give the shown window and focused pane.
- `mux/internal/server/layout.go` — `barRows`, `titleRows`, `rect`, and
  `layout`. A pane's title row is `rect.y-1`.
- `mux/internal/server/pane.go` — `startPane` builds the terminal with
  `libghostty.NewTerminal`; `readPane` feeds output and records the exit.
- `mux/internal/server/server.go` — `spawn`, `kill`, and `respawn` change the
  tree; each currently signals only the affected session or window.
- libghostty: `Terminal.Title()` returns the title set by OSC 0 or 2, or an
  empty string. `WithTitleChanged` registers a function called inside
  `VTWrite` when the title changes.
- `mux/cmd/mux/main_test.go` — `startServer`, `spawnPane`, `attachAt`,
  `killTarget`, and `TestCursorAndOffsets`, which reads frames until they
  contain expected bytes.

## Decisions

- **Labels.** A new file `mux/internal/server/label.go` holds:
  - `(p *pane) label() string`: `p.term.Title()`, or `p.name` when the title
    is empty. A `Title` error means the terminal is broken, so it panics.
  - `(w *window) label() string`: `w.panes[0].label()`.
  - `(sess *session) label() string`: `sess.windows[0].label()`.
  Windows and sessions are never empty, since `kill` removes them with their
  last pane. The later `label` request will make these return a set label
  first; nothing here is built for it yet.
- **Command name.** `pane` gets `name string`, set in `startPane` to
  `filepath.Base(argv[0])`. `respawn` builds a new pane, so its name and title
  start fresh.
- **Exit status.** A pane's title row shows `label (exited N)` once `alive`
  is false. The bars show labels only.
- **One drawing helper for bars and title rows.** `appendBar(b []byte, row,
  col, width int, items []string, current int) []byte` moves to `row`, `col`
  (1-based) and writes each item as ` text `, the current one in inverse video
  (`\x1b[7m ... \x1b[0m`), cut off at `width` columns. Pass `current = -1` for
  none. Width is counted in runes; a title with wide or combining characters
  may draw a little off; that is unlikely and gets a comment.
  - Top bar: row 1, every session's label, the shown session current.
  - Window bar: row 2, each window of the shown session as `N label`, `N`
    counting from 0 to match the `0`-`9` keys of task 7, the shown window
    current.
  - Title rows: for each rect from `layout`, row `r.y` (1-based for `r.y-1`),
    column `r.x+1`, width `r.cols`, one item, current when the pane is
    `a.focus()`.
  Bars wider than the terminal are cut off; scrolling them is not needed.
- **Frame order.** `drawFrame` writes the clear, the two bars, the title rows,
  and the divider before any pane's cells, so the SGR state tracked in the
  cell loop starts from the reset that ends each chrome item. The frame still
  ends with the cursor, so `TestCursorAndOffsets` keeps passing. `drawFrame`
  needs the session list, so it becomes `drawFrame(a, s.sessions)`.
- **Redraws.** A label change in one session shows in every client's top bar,
  so rare tree and label changes signal every attachment through a new
  `(s *Server) signalAll()`:
  - `startPane` passes `libghostty.WithTitleChanged` with a function that
    calls `s.signalAll()`. It runs inside `VTWrite`, with the mutex held.
  - `spawn` and `kill` call `signalAll` after changing the tree.
  - `respawn` calls `signalAll` in place of `s.wake(p)`, since the command
    name may change.
  - `readPane` calls `s.wake(p)` after recording an exit, so the title row
    shows the status.
  Pane output keeps calling `s.wake(p)`. A full reset (`ESC c`) clears the
  title without calling the title function, so a hidden pane's label can stay
  stale until the next redraw; that is unlikely and gets a comment.
- `barRows` and `titleRows` lose "drawn by a later task" from their comments.

## Test plan

New tests in `mux/cmd/mux/main_test.go`. Each attaches at 100 by 30 and reads
frames until one contains the expected bytes, failing if the stream ends.

- `TestTitleLabels`: a pane running `sh -c 'printf "\033]0;hello\007"; exec
  sleep 60'`. A frame contains `\x1b[1;1H\x1b[7m hello \x1b[0m` (top bar),
  `\x1b[2;1H\x1b[7m 0 hello \x1b[0m` (window bar), and
  `\x1b[3;1H\x1b[7m hello \x1b[0m` (main pane's title row).
- `TestCommandNameLabels`: session one runs `sleep 60`; session two runs `sh
  -c 'printf "\033]0;other\007"; exec sleep 60'`; a second pane in session
  one's window runs `sleep 60`. Attached to session one, a frame contains
  `\x1b[1;1H\x1b[7m sleep \x1b[0m other ` and the stacked pane's title row
  `\x1b[3;42H sleep `.
- `TestExitedTitle`: a pane running `sh -c 'exit 3'`. A frame contains
  `\x1b[3;1H\x1b[7m sh (exited 3) \x1b[0m`.

By hand, as the design asks: in `mux attach`, `printf '\033]0;hello\007'`
changes the pane title and both bars, and `exit 3` in a stacked pane shows
`(exited 3)` in its title row.

## Implementation plan

1. Add `mux/internal/server/label.go` with the three `label` methods.
2. In `mux/internal/server/pane.go`, add `name` to `pane`, set it and pass
   `WithTitleChanged` in `startPane`, and call `s.wake(p)` after an exit in
   `readPane`.
3. In `mux/internal/server/attach.go`, add `signalAll` and `appendBar`, and
   draw the bars and title rows in `drawFrame(a, sessions)`.
4. In `mux/internal/server/server.go`, call `signalAll` from `spawn`, `kill`,
   and `respawn`.
5. In `mux/internal/server/layout.go`, update the constant comments.
6. In `mux/cmd/mux/main_test.go`, add the three tests.
