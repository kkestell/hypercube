# Terminal panel review

## Scope and coverage

The uncommitted changes on `main`: the terminal panel and layout toggle
buttons in `internal/ui/static/index.html`, `internal/ui/style.css` and the
generated `ui.css`; the four layout icons and the codicons license; the Go
changes in `cmd/hypercube/main.go`, `internal/report/report.go` and
`plugins/claude/main.go` with their tests; and `docs/usage.md`.

Three GPT Sol reviewers each covered one area: frontend practice, code
simplicity and correctness, and comments, documentation, tooling and meta. I
checked every finding against the code. The frontend has no React; the
frontend reviewer applied general frontend practice to the vanilla JS.

Gaps:

- The panel was not run in a browser. The reviewers probed tmux behavior with
  an isolated server; the layout findings follow from reading the code.
- `make check` passed (build, vet, gofmt, race tests including the tmux
  lifecycle test).

## Summary

| ID | Severity | Category    | Title                                                         |
| -- | -------- | ----------- | ------------------------------------------------------------- |
| F1 | Medium   | Correctness | A workspace row no longer shows a busy spinner                |
| F2 | Medium   | Correctness | Showing the panel during a refresh can start a second shell   |
| F3 | Low      | Layout      | The panel keeps its pixel height when the window gets shorter |

## F1. A workspace row no longer shows a busy spinner

Medium · Correctness · `internal/ui/static/index.html:315`, `docs/usage.md:46`

`renderSidebar` always draws `icon-folder` or `icon-folder-opened` for a
workspace row. The old `spinner(s.busy && 'Busy')` call was deleted, so `s.busy`
is fetched and never used. A collapsed workspace with busy agents shows no
activity. `docs/usage.md` says the folder icon becomes a spinner while any
agent is busy, which the code does not do. The session rows do show the spinner
in the folder column.

Fix: draw `spinner('Busy')` in place of the folder icon when `s.busy` is set.

Found by all three reviewers; rated low by two. Confirmed by reading the old
and new `renderSidebar`. I rate it medium because it drops existing behavior
and contradicts the guide.

## F2. Showing the panel during a refresh can start a second shell

Medium · Correctness · `internal/ui/static/index.html:676-684`

`createTerminal` sets `creating = false` as soon as `new-window` returns, then
queues the commands that set `@hypercube_terminal`. If a refresh is already
running when you click Show Panel, `setPanel` sets `again`. That refresh's
`render` queues `new-window`, and its `again` rerun queues `list-windows`
behind it. The creation callback runs before the rerun's results are rendered,
so `creating` is false, and the rerun's `list-windows` ran before the marker was
set. `renderPanel` sees no terminal and creates another. Only the first marked
window is shown, so the extra shell stays running and hidden.

Fix: keep `creating` true until `renderPanel` sees the marked terminal; clear
it immediately only when `new-window` fails.

Found by the simplicity reviewer, which confirmed the command order against an
isolated tmux server. I traced the `setPanel`, `refresh`, `renderPanel` and
`createTerminal` paths and agree. The window is a few milliseconds per click
(the one-second timer can also hit it), so it is rare, but the result is an
invisible persistent process.

## F3. The panel keeps its pixel height when the window gets shorter

Low · Layout · `internal/ui/static/index.html:36,736`

The panel is `h-[240px] shrink-0`. Dragging clamps its height to 80% of the
window, but nothing clamps it when the window later shrinks. After dragging the
panel to 600px in an 800px window and shrinking the window to 500px, the center
area collapses to zero and the bottom of the panel falls outside the
`overflow-hidden` body.

Fix: add a `max-h-[80vh]` class to the panel.

Found by the frontend reviewer from the layout rules; the browser probe did not
run. Rated low because it needs a large drag followed by a window resize.

## Checked and clean

- The Go changes (200-character titles, deleting a final team that has an extra
  tmux window, Claude pasted-tag removal) and their tests: no findings.
- The four SVGs are byte-identical to the upstream codicons, and
  `codicons-LICENSE` names the upstream project and license. The files are
  embedded by the existing static embed; no Makefile or script change is
  needed.
- `ui.css` matches a fresh Tailwind build.
- The browser-side panel logic has no automated test; the Go tests do not
  execute `index.html`.

## Medium and high issues

- F1 (Medium): a collapsed workspace with busy agents shows no spinner, and
  `docs/usage.md` says it does.
- F2 (Medium): clicking Show Panel while a refresh is running can leave a second,
  hidden shell running.

No high-severity issues were found.
