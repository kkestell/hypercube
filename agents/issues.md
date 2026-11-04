# Issues

Known defects that need unusual timing or conditions, ordered by consequence.

## Ending a child can lose its last writes

`EndChild` (`internal/session/children.go`) checks that the worktree is
removable, then runs `git worktree remove --force` while the child's agent is
still running, so a write between the two is lost. A crash between removing
the worktree and saving the end leaves a child that restarts into a missing
directory. Stop the child's agent and shell before removing the worktree, and
save the end first.

## A late start reply leaves Create, Start or CreateChild waiting

Two one-minute timers in `internal/session/launch.go` own the same start
deadline. If the caller's timer fires and the reply arrives before `run`'s
timer starts, `launch` waits on `l.done` until the agent exits. On a wait
timeout, stop the client before waiting, and delete `run`'s timer.

## The holder can hang instead of exiting

In `internal/hold/holder.go`, a write error (such as a full disk) ends the
output copier while `Main` keeps waiting for the child, and a `setsid`
descendant that keeps stderr open blocks `<-child.stderrDone` forever. Wait
on the copier's result as well as the child, kill the process group if it
fails, and bound the stderr wait like stdout's.

## Codex turn end drops native child results

`endTurn` in `plugins/codex/events.go` finishes every open item and request on
every thread, so a native child's later results are dropped. Record each
item's thread and finish only the main thread's entries.

## Pi extension dialogs can block for 30 seconds

`input` blocks the plugin's only command loop
(`plugins/internal/plugin/serve.go`), so an extension dialog that Pi opens
before accepting a prompt cannot be answered until the input timeout. Run
`input` in its own goroutine.

## Restore races early requests

HTTP serving starts before `Restore` (`internal/session/restore.go`,
`cmd/hypercube/main.go`), so a surviving agent's `create_child`, or a user's
Stop, can race the restore. Register restored launches before serving
lifecycle routes; MCP must still be served first.

## The UI can be framed

HTML responses send no `frame-ancestors` policy (`internal/ui/ui.go`), so a
page could frame the UI over a pending approval. Send
`Content-Security-Policy: frame-ancestors 'none'`.

## A shell can open after its session stops

`Shell` in `internal/session/sessions.go` does not recheck `r.live` under
`mu`, so a terminal open that races Stop starts a shell. Recheck it.

## Detach can hang on a blocked plugin

`Detach` in `internal/session/sessions.go` takes `l.send`, which blocks when a
plugin stops reading stdin with full buffers. Close the connection without
taking `l.send`.

## Reply cards expire while hovered

Reply notification cards (`notifications/notification-card.jsx`) close after
15 seconds even while hovered or focused. Pause the timer on `pointerenter`
and `focusin`.

## Scrolling the transcript and file view by keyboard

The transcript and file view have no focusable scroll target. If WebKit does
not scroll a clicked, unfocusable element by keyboard, add `tabindex="0"`.

## Multi-select form fields return one value

Codex (`plugins/codex/requests.go`) and OpenCode form handling return a single
value for multi-select fields, so forms requiring two or more selections fail.
Refuse those fields when building the form.

## An interrupted Codex rollout import looks complete

`plugins/codex/storage.go` copies rollouts in place, so a copy cut off by a
crash is later accepted as complete. Copy to a temporary file and rename it.

## File view details

`files/highlighted-code.jsx` updates the DOM by hand; render from state
instead. The line-number gutter in `files/file-view.jsx` needs
`aria-hidden`.

## A killed holder leaves its session running

When an agent's holder process is killed from outside Hypercube, the session
keeps showing as running and cannot be ended while its branch is unmerged.
Hypercube should notice the holder's exit and mark the session exited.
