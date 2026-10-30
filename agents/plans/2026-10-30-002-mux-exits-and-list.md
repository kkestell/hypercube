# Mux exits and `list`

## Goal

Task 2 of milestone 1 in `agents/mux-design.md`. When the task is done:

- A pane whose process exits stays in its window with `alive` false and its
  exit status. A signal death's status is 128 plus the signal number.
- `list` returns every session, window, and pane, with each pane's `alive`
  and `status`. `mux ls` prints them.
- `spawn` with a `session` adds a window to that session, and `spawn` with a
  `window` adds a pane to that window. `mux spawn` takes `-s`, `-w`, and `-c`.
- Unknown IDs return errors.

## Related code

- `mux/internal/server/pane.go` — `pane`, `startPane`, and `readPane`, which
  reaps the process with `cmd.Wait` when the PTY read fails and records
  nothing yet.
- `mux/internal/server/server.go` — `Server`, `handle`, `spawn` (which
  rejects a non-empty `session` or `window` until this task), and `findPane`.
- `mux/client/client.go` — `Request` and `Reply`.
- `mux/cmd/mux/main.go` — `run` and `spawn`.
- `mux/cmd/mux/main_test.go` — `startServer`, `runMux`, `dial`,
  `waitCapture`, and `TestErrors`.

## Decisions

- **Recording an exit.** `pane` gains `alive bool` and `status int`, guarded
  by the server's mutex. `startPane` sets `alive` true. After `cmd.Wait`,
  `readPane` locks and sets `alive` false and `status` from
  `exitStatus(cmd.ProcessState)`:
  - `ProcessState.Sys().(syscall.WaitStatus)`; if `Signaled()`, 128 plus the
    signal number, else `ExitStatus()`.
  - The PTY read fails only once every holder of the PTY's terminal side has
    closed it. A background child that keeps it open delays the exit until
    that child ends; that is unlikely and gets a comment only.
  - The PTY master, terminal, and render state stay open so `capture` and
    attach still show the last screen. `kill` (task 3) and `respawn` (task 4)
    own closing or replacing them.
- **`list` reply.** `client.Reply` gains `Sessions []Session`
  (`sessions,omitempty`). The new wire types are:
  - `Session {id, windows []Window}`
  - `Window {id, panes []Pane}`
  - `Pane {id, alive, status}`. `alive` and `status` are always present, so
    exit status 0 is not dropped. A live pane's `status` is 0.

  Sessions, windows, and panes appear in creation order.
- **Spawning into an existing session or window.**
  - Neither set: a new session with a new window, as now.
  - `session` set, `window` empty: a new window appended to that session.
  - `window` set: a new pane appended to that window. If `session` is also
    set, it must be the window's session, or the request fails with
    `window w2 is not in session s1`.
  - An unknown ID fails with `unknown session "s9"` or `unknown window "w9"`.
  - The reply always carries all three IDs.
  - A new pane starts at 80 by 24. Attach still shows the session's first
    pane; layout is task 5.
  - `findSession` and `findWindow` join `findPane`; `findWindow` returns the
    window and its session.
- **`mux spawn [-s session] [-w window] [-c dir] [command...]`.** A
  `flag.FlagSet` parses the options and stops at the first non-option, so
  `mux spawn sh -c 'exit 3'` keeps its own `-c`. `-c` defaults to the current
  directory.
- **`mux ls`.** One line per pane, in `list` order: the session, window, and
  pane IDs, then `running` or `exited <status>`:

  ```
  s1 w1 p1 running
  s1 w1 p2 exited 3
  ```

  It matches the `s1 w1 p1` line `mux spawn` prints. With nothing running it
  prints nothing.

## Test plan

Tests go in `mux/cmd/mux/main_test.go`. A helper `waitExited(t, c, pane)`
polls `list` until that pane has `alive` false and returns its status.

- `TestExitStatus` spawns three panes in one server:
  - `sh -c 'echo bye; exit 3'`: status 3, and `capture` still shows `bye`.
  - `sh -c 'kill -TERM $$'`: status 143.
  - `sleep 60`: `list` shows it `alive` true.
- `TestSpawnIntoSessionAndWindow` spawns `sleep 60` three times: once with
  no IDs, once with the first reply's `window`, and once with its `session`.
  - The second reply has the same session and window and a new pane.
  - The third has the same session and a new window.
  - `list` shows one session whose first window holds the first two panes
    and whose second window holds the third pane.
- `TestLsCommand` runs the command line:
  - `mux spawn sleep 60`, then `mux spawn -w <window> -c <dir> sh -c 'pwd;
    exit 3'` with `dir` resolved through `filepath.EvalSymlinks`.
  - The second spawn prints the first's session and window.
  - `capture` of the second pane shows `dir`.
  - Polling `mux ls` shows exactly the two lines `<s> <w> <p1> running` and
    `<s> <w> <p2> exited 3`.
- `TestErrors` gains, after spawning two sessions:
  - `spawn` with an unknown `session`.
  - `spawn` with an unknown `window`.
  - `spawn` with the first session and the second session's window.

## Implementation plan

1. In `mux/client/client.go`, add `Session`, `Window`, and `Pane`, and
   `Reply.Sessions`.
2. In `mux/internal/server/pane.go`, add `alive` and `status` to `pane`, set
   `alive` in `startPane`, record the exit in `readPane`, and add
   `exitStatus`.
3. In `mux/internal/server/server.go`, add `findSession` and `findWindow`,
   rewrite `spawn` for the three cases, and add the `list` handler to
   `handle`.
4. In `mux/cmd/mux/main.go`, parse `-s`, `-w`, and `-c` in `spawn`, add the
   `ls` command, and add both to the usage text.
5. Add the tests above.
