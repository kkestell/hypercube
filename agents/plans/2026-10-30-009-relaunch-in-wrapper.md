# Relaunch in the wrapper

## Goal

Mux Milestone 2, pieces 1 and 3 in `agents/mux-design.md`. Today
`hypercube agent` replaces itself with the harness through `syscall.Exec`, and
a tmux `pane-died` hook runs `hypercube exited`, which counts quick exits and
respawns the pane with `tmux.Restart`. The mux will have no exit events, so the
wrapper must own relaunches.

When done, `hypercube agent` runs the harness as a child process and waits for
it. When the harness exits, the wrapper launches it again with the saved
conversation. After the third quick exit in a row, the wrapper marks the agent
exited and exits with the harness's status, and the dead pane shows that status
as it does today. Ctrl-C reaches the harness as before. `hypercube exited`,
`tmux.Restart`, `tmux.Dead`, the `pane-died` hook, and the extra lock in
`exited` are gone. Nothing else about tmux changes.

## Related code

- `cmd/hypercube/main.go` — `runAgent` (the wrapper) and `exited` (the
  relaunch policy). `openWorkspace` resets `QuickExits` when it relaunches a
  missing window. `main` dispatches `agent` and `exited`.
- `internal/harness/harness.go` — `Command` builds the harness command; it adds
  `--resume` (Claude Code) or `--session` (OpenCode) from the agent's saved
  `SessionID`, which is how a relaunch resumes the conversation. `Launch`
  starts the pane running `hypercube agent <prompt>`.
- `internal/tmux/tmux.go` — `NewPane` installs the `pane-died` hook and sets
  `remain-on-exit` (which keeps the dead pane and its status visible);
  `Restart` and `Dead` exist only for `exited`.
- `internal/store/store.go` — `Agent.Launched` and `Agent.QuickExits`.
- `cmd/hypercube/integration_test.go` — `TestTmuxNativeLifecycle` and the
  `TestNativeHarness` stand-in; "resume after exit" already checks one
  relaunch.

## Decisions

- **Quick exits live in the wrapper's memory.** `Launched` and `QuickExits`
  are read only by `exited`, written by `runAgent` and `exited`, and reset by
  `openWorkspace` when it starts a new wrapper. The wrapper now outlives every
  relaunch of its harness, so a local counter and a start time replace both
  fields, and a new wrapper starts at zero, which is what the reset did. Delete
  both fields from `store.Agent`.
- **Same rule as today.** After each exit: count it, reset the count to zero if
  the harness ran for 10 seconds or more, and stop at 3.
- **SIGINT is caught, not ignored.** The harness runs in the wrapper's process
  group, so Ctrl-C sends SIGINT to both. The wrapper calls `signal.Notify` with
  a channel it never reads, which keeps it alive. `signal.Ignore` would set
  SIG_IGN, which the child inherits across exec. The wrapper leaves SIGHUP at
  its default so that `kill-pane`, `kill-window`, and `kill-session` still end
  both processes.
- **Every launch repeats today's launch bookkeeping.** Before each launch the
  wrapper reloads the agent and sets `Target`, `Attention = false`, and
  `Activity` (busy with a prompt, idle without), then calls `tmux.Activity`.
  Reloading picks up the `SessionID` that the harness's hooks saved. The
  wrapper passes the same prompt to every launch, as `tmux.Restart` did by
  reusing the pane's command.
- **Ending.** After the last quick exit the wrapper sets `Activity = Exited`
  and `Attention = false`, calls `tmux.Activity` with `store.Exited`, and exits
  with the harness's exit code. A failure to start the harness (for example a
  missing binary) sets `Activity = Exited` and returns the error without
  relaunching, as today. A harness killed by a signal reports exit code -1,
  which the pane shows as 255; leave a comment, no code.
- The `tmux.Owns` check at the start of `runAgent` stays once, before the
  loop. Piece 4 replaces it.

## Implementation plan

1. `internal/store/store.go`: delete `Launched` and `QuickExits`.
2. `cmd/hypercube/main.go`:
   - Delete `exited` and its `case` in `main`.
   - In `openWorkspace`, drop the `QuickExits` reset
     (`a.Target, a.Attention = "", false`).
   - Rewrite `runAgent` to return `(int, error)`; `main` calls `os.Exit` with
     the code when the error is nil. Shape:

     ```go
     // runAgent runs the harness as a child and relaunches it with its saved
     // conversation until it exits quickly three times in a row.
     func runAgent(prompt string) (int, error) {
     	id, path, pane := ...
     	// (existing Owns check)
     	// Ctrl-C reaches the harness through the process group. Catch it here
     	// rather than ignore it: an ignored signal stays ignored in the child.
     	signal.Notify(make(chan os.Signal, 1), os.Interrupt)
     	quickExits := 0
     	for {
     		agent, err := markLaunched(path, id, pane, prompt) // today's store.Update
     		...
     		argv, env, err := harness.Command(agent, path, prompt)
     		tmux.Activity(...)
     		cmd := exec.Command(argv[0], argv[1:]...)
     		cmd.Env = (today's inherited environment merge)
     		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
     		started := time.Now()
     		err = cmd.Run()
     		var exit *exec.ExitError
     		if err != nil && !errors.As(err, &exit) {
     			markExited(path, id)
     			return 0, err
     		}
     		quickExits++
     		if time.Since(started) >= 10*time.Second {
     			quickExits = 0
     		}
     		if quickExits >= 3 {
     			markExited(path, id)
     			tmux.Activity(store.Workspace(path), pane, id, path, store.Exited, false)
     			return cmd.ProcessState.ExitCode(), nil
     		}
     	}
     }
     ```

     Inline `markLaunched` and `markExited` if each is used once; keep them as
     small functions if not. Remove imports that become unused (`strconv`,
     `syscall` if nothing else uses them).
3. `internal/tmux/tmux.go`: delete `Restart`, `Dead`, the `died` command, the
   `set-hook ... pane-died` entry in `NewPane`, and the `os.Executable` call
   that only fed `died`. Keep `remain-on-exit` and the `pane_dead_status`
   border text.
4. `internal/harness/harness.go`: update the `Launch` comment, which says the
   wrapper replaces itself with the harness.
5. `cmd/hypercube/integration_test.go`: add the checks below.

## Test plan

- **Integration (`TestTmuxNativeLifecycle`).** "resume after exit" stays as is
  and must still pass. Add, using a new agent created with `create_agent` so
  its launch count starts fresh:
  - `send-keys C-c` to its pane: the stand-in dies from SIGINT, the pane stays
    alive, and the launch count reaches 2. This shows the wrapper survived
    and the child received the signal.
  - Send `exit` and wait for launch 3, then send `exit` again: the pane
    becomes dead (`#{pane_dead}` is 1), `#{pane_dead_status}` is 0, the
    agent's `Activity` is `exited`, and the launch count stays 3.
  These three exits happen within seconds, so all count as quick.
- **Visible smoke test with `make dev`.** It opens a Ghostty window the user
  watches. Drive the panes with `tmux send-keys` against the dev server
  (`TMUX_TMPDIR=$HOME/projects/hypercube-dev`, as `scripts/dev` does). Do the
  following for the Claude Code lead, and for an OpenCode worker created
  through the lead with the dev config's `opencode` profile:
  - Ctrl-C reaches the harness normally: type a draft at the prompt and press
    Ctrl-C; the harness reacts as it does when run directly, and the pane
    does not die or relaunch.
  - Tell the harness a word to remember, quit it with its own quit command,
    and confirm it relaunches in the same pane; ask for the word and confirm
    the conversation resumed.
  - Quit it three times, each within 10 seconds of its launch; the pane stays
    dead and its border shows `exited` with the status.
  If any of these fails, stop and report to the lead what happened. Do not
  work around it; the design for how Hypercube learns of exits would need to
  change.
- Clean up afterwards: `scripts/dev clean`, and close the Ghostty window.
