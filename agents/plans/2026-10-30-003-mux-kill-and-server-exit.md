# Mux `kill` and server exit

## Goal

Task 3 of milestone 1 in `agents/mux-design.md`. When the task is done:

- `kill {pane | window | session}` hangs up the target's processes and
  removes it. Removing a window's last pane removes the window, and removing
  a session's last window removes the session.
- When `kill` removes the last session, the server removes its socket,
  replies, and exits. The next `mux spawn` starts a new server.
- IDs are never reused while a server runs.
- `mux kill <id>` sends `kill` for a pane, window, or session ID.

## Related code

- `mux/internal/server/server.go` — `Server`, `ListenAndServe` (accepts until
  the listener fails), `serve`, `handle`, `spawn`, `nextID` (one counter per
  kind, never reset), and `findPane`, `findWindow`, `findSession`.
- `mux/internal/server/pane.go` — `pane`, `startPane`, and `readPane`, which
  records the exit after `cmd.Wait` and leaves the PTY master, terminal, and
  render state open. `pty.StartWithSize` sets `Setsid` and `Setctty`, so the
  pane's process leads its own session and process group.
- `mux/internal/server/attach.go` — `attachment` (holds the pane it shows),
  `attach`, `stream` (input and resize), and `draw`.
- `mux/cmd/mux/main.go` — `run` and its usage text.
- `mux/cmd/mux/main_test.go` — `newState` (its cleanup SIGTERMs the server
  with a comment that it has no exit path until task 3), `serverPID`,
  `startServer`, `runMux`, `spawnPane`, `waitCapture`, and `TestErrors`.

## Decisions

- **Request.** `kill` takes exactly one of `pane`, `window`, or `session`.
  None or more than one fails with `kill needs exactly one of pane, window,
  or session`. An unknown ID fails with `unknown pane "p9"`, `unknown window
  "w9"`, or `unknown session "s9"`, the same text as `spawn` and `capture`.
  The reply is a bare `ok`.
- **Hanging up.** Closing the PTY master does not hang up the process: the
  master is a blocking file, so `Close` waits for `readPane`'s pending
  `Read`. Instead, `pane.hangUp` sends SIGHUP to the pane's process group,
  `-cmd.Process.Pid`. A shell passes SIGHUP on to its jobs. A process that
  ignores SIGHUP keeps running with no pane; that is unlikely and gets a
  comment only.
- **Freeing a pane.** `pane` gains `killed bool`, guarded by the server's
  mutex, and `free()`, which closes the PTY master, the terminal, and the
  render state. Exactly one place frees each pane:
  - `hangUp` sets `killed`. If the pane is already dead, it frees it now.
  - Otherwise it sends SIGHUP, and `readPane`, after recording the exit,
    frees the pane if `killed` is set.

  So `readPane` keeps writing to a live terminal until the process ends.
- **Removing from the tree.** `Server.kill` holds the mutex, finds the
  target, and removes it from its parent's slice, cascading empty windows and
  sessions upward. It calls `removePane` for every pane removed. Afterward
  every session has at least one window and every window at least one pane,
  so `attach`'s `sess.windows[0].panes[0]` stays valid.
- **Attachments to a removed pane.** Attach shows a session's first pane
  until task 5. `removePane` ends each attachment showing the pane: it
  deletes it from `s.attached` and closes its connection.
  - `attachment` gains `conn net.Conn`; `serve` passes the connection to
    `attach`.
  - Closing the connection makes `stream`'s `Decode` fail, so `attach` cleans
    up as it does when a client disconnects. `mux attach` returns on the read
    error and restores the terminal.
  - `draw` and `stream`'s resize work only while the attachment is still in
    `s.attached`, checked under the mutex. Without the check, a pending wake
    or resize could touch a terminal that `free` closed.
  - Input to a freed pane writes to a closed file and fails harmlessly.
- **Server exit.**
  - `ListenAndServe` stores `path` and `listener` on the `Server` before
    accepting. `New` makes `exited chan struct{}`.
  - When `kill` leaves no sessions, it removes the socket and closes the
    listener while holding the mutex, before the reply. No new client can
    connect, and a test sees the socket gone as soon as the reply arrives.
  - `serve` writes the reply first. Then, if the request was a successful
    `kill` and no sessions remain, it closes `s.exited`. It does this even if
    the write failed.
  - When `Accept` fails with `net.ErrClosed`, `ListenAndServe` waits for
    `s.exited` and returns nil, and `mux server` exits 0. Pane processes have
    already received SIGHUP, and process exit closes every PTY master.
  - A client that connected before the listener closed could spawn after
    the last kill, and its pane would die with the server. That is unlikely
    and gets a comment only.
  - A server that never had a session, such as one `mux ls` started, keeps
    running. That needs no change in this task.
- **IDs.** `nextID` already never resets its counters, so this task adds
  only a test.
- **`mux kill <id>`.** The ID's first letter picks the request field: `p`
  pane, `w` window, `s` session. Any other ID fails with `unknown ID "x1"`
  without contacting the server. It prints nothing on success. The usage
  text gains `mux kill <id>`.

## Test plan

Tests go in `mux/cmd/mux/main_test.go`. Panes that a test kills run
`sh -c 'echo pid=$$; exec sleep 60'`. A helper `panePID(t, c, pane)` reads
the PID from `capture`, and `waitGone(t, pid)` polls `syscall.Kill(pid, 0)`
until it fails with `ESRCH`. `readPane` reaps the process, so no zombie
remains.

- `TestKillPane`: one window holds a live pane, a second live pane, and a
  pane that ran `sh -c 'exit 3'` and has exited.
  - Killing the first live pane ends its process, and `list` shows the
    window with the other two panes. `capture` of the killed pane fails.
  - Killing the exited pane succeeds, and `list` shows the window with only
    the second live pane.
- `TestKillWindow`: session A has windows w1 and w2; session B has one
  window.
  - Killing w1 ends its process; `list` shows A with only w2.
  - Killing w2 removes A; `list` shows only B.
- `TestKillSessionAndNewIDs`: session A has two windows, and session B has
  one.
  - Killing A ends both of A's processes, and `list` shows only B.
  - A new spawn returns a session, window, and pane ID that differ from
    every ID killed.
- `TestKillEndsAttach`: sessions A and B. A client attaches to A and reads a
  frame. Killing A ends the attached stream: `ReadOutput` returns an error.
  B keeps the server running, so the stream ends because of the kill.
- `TestKillCommandStopsServer` runs the command line:
  - `mux spawn sleep 60`, then `serverPID` for the server's PID.
  - `mux kill <session>` prints nothing. The socket is gone at once, and the
    server's PID goes away.
  - `mux spawn sleep 60` prints `s1 w1 p1`, which shows a new server
    started.
- `TestErrors` gains: `kill` with no target, with both `pane` and `window`,
  and with an unknown pane, window, and session.

## Implementation plan

1. In `mux/internal/server/pane.go`, add `killed`, `hangUp`, and `free`, and
   free a killed pane at the end of `readPane`.
2. In `mux/internal/server/attach.go`, add `conn` to `attachment` and an
   `attach` parameter for it, and skip `draw` and resize for an attachment
   no longer in `s.attached`.
3. In `mux/internal/server/server.go`, add `path`, `listener`, and `exited`
   to `Server`; add `kill` and `removePane`; add the `kill` case to
   `handle`; close `exited` in `serve` after the last session's kill; and
   return nil from `ListenAndServe` once `exited` closes.
4. In `mux/cmd/mux/main.go`, add the `kill` command and its usage text.
5. In `mux/cmd/mux/main_test.go`, add the tests and helpers above, and
   replace `newState`'s cleanup comment: tests that leave sessions running
   stop the server there.
