# Hypercube on the mux

## Goal

Mux Milestone 2, piece 4 in `agents/mux-design.md` ("Hypercube on the mux").
Hypercube runs its teams on the mux instead of tmux. A new `internal/mux`
package replaces `internal/tmux` and talks to the mux server only through
`mux/client`. Only the pane ID is saved, as the agent's `Target`; a
workspace's mux session and a team's window are found through `list`. Hooks,
`wait`, and the status line set plain text labels and attention through the
socket. The `c`, `W`, and `X` bindings, unseen-reply marks, and every tmux
call go away. The build, `scripts/dev`, `make install`, and
`scripts/reinstall` build and install the `mux` executable, and the
integration test drives the mux through the socket.

Done when `make check` passes, nothing outside `mux/` mentions tmux except the
recorded smoke test in `docs/harness-support.md`, and the smoke test below
passes.

One implementer pass. The pieces cannot land separately with `make check`
passing, since every caller of `internal/tmux` changes at once.

## Related code

- `internal/tmux/tmux.go` — everything being replaced: `NewPane`, `Respawn`,
  `Owns`, `KillPane`, `ConfigureSession` (status formats, `c`/`W`/`X`, the two
  session hooks), `Activity`, `markReply`, `attention`, `KillSession`,
  `Attach`, `WorkspaceSession`, `FindWindow`, `SelectWindow`, `KillWindow`,
  `FindPane`, `RefreshActivity`.
- `mux/client/client.go` — `Request`, `Reply`, `Session`/`Window`/`Pane` (the
  `list` tree), `Span`, `Dial`, `ErrNoServer`, `DialOrStart(mux)`,
  `SocketPath`, `Conn.Call`, `Conn.Send`, `Conn.ReadOutput`.
- `mux/internal/server/server.go` — error texts: `unknown pane %q`,
  `unknown window %q`, `unknown session %q`. `kill` of the last session makes
  the server exit and remove its socket before replying. `spawn` does not make
  a new window current. IDs look like `p7-4f2a`; the first letter is the kind.
- `mux/cmd/mux/main.go` — `call` routes an ID by its first letter; the new
  package does the same.
- `mux/cmd/mux/main_test.go` — `newState`, `attachAt`: a socket path must
  stay under 104 bytes on macOS; an attached client must have its output read.
- `cmd/hypercube/main.go` — `stop`, `status`, `stopOrDelete`, `deleteTeam`,
  `openWorkspace`, `windowAction`, `runAgent`, `running`, and the `window`
  case with its `display-message` error path.
- `internal/harness/harness.go` — `Launch` builds the environment (dropping
  `TMUX`/`TMUX_PANE`) and the pane label, then calls `tmux.NewPane`.
- `internal/mcp/mcp.go` — `createAgent` (`FindWindow`, `Owns`, `KillPane`
  cleanup) and `removeAgent` (`FindPane`, `KillPane`).
- `internal/hook/hook.go`, `internal/wait/wait.go` — call
  `tmux.RefreshActivity`.
- `internal/statusline/statusline.go` — clears attention and calls
  `tmux.Activity`.
- `cmd/hypercube/integration_test.go` — `TestTmuxNativeLifecycle`.
- `cmd/hypercube/main_test.go`, `internal/mcp/mcp_test.go` — set
  `TMUX_TMPDIR` and `TMUX`.
- `Makefile`, `mux/Makefile`, `scripts/dev`, `scripts/reinstall`, `go.mod`.

## Decisions

- **Go module.** Hypercube's `go.mod` requires
  `github.com/kkestell/hypercube/mux v0.0.0` with
  `replace github.com/kkestell/hypercube/mux => ./mux`, then `go mod tidy`.
  Hypercube imports only `mux/client`, which uses only the standard library, so
  building Hypercube needs no cgo. The root module's `./...` excludes the
  nested `mux/` module, so `go vet` and `go test` at the root are unchanged.
- **Finding `mux`.** `mux` is installed beside `hypercube`, and Hypercube runs
  `filepath.Join(filepath.Dir(os.Executable()), "mux")`. The two binaries
  always come from the same build, and dev, tests, and installs stay apart
  without a PATH lookup or a setting. `make build` produces `bin/mux` beside
  `bin/hypercube`; `make install` installs both to `$(BINDIR)`.
- **Which calls start a server.** Only `Spawn` uses `client.DialOrStart`.
  Every other call uses `client.Dial` and treats `ErrNoServer` as "nothing
  running": lookups return empty, `Kill` succeeds, labels and attention do
  nothing.
- **Stale IDs.** An error whose text starts with `unknown ` means the saved ID
  names nothing in this server. `Kill` treats it as success; the best-effort
  label and attention calls ignore every error, as `tmux.Activity` did. A
  saved ID proves ownership, so there are no ownership checks.
- **Lookups.** `Workspace(s)` is the session holding any agent's `Target`;
  `Team(s, team)` is the window holding that team's lead's `Target`. Each
  dials and calls `list` once. If a user kills a lead's pane by hand while its
  workers live, the next `hypercube` starts the team again in a new window;
  leave a comment naming that case and add no code.
- **Who saves `Target`.** The wrapper cannot learn its own pane ID, so
  `harness.Launch` saves `Target` from the `spawn` reply and then calls
  `mux.RefreshActivity`. `runAgent` no longer sets `Target` or checks
  `TMUX_PANE`; it only requires `HYPERCUBE_AGENT_ID` and `HYPERCUBE_STATE`. The
  wrapper's first refresh can run before `Target` is saved and do nothing; the
  refresh in `Launch` reads the newer state, so the label still appears.
- **Labels.** Plain text, one span. A pane's label is
  `name · harness model effort · ~/dir · busy` (or `idle`); an exited agent
  drops the last part, since the mux shows the exit status in the title. A
  team's window is labeled with the team name and a workspace's session with
  `filepath.Base(root)`, both set by `openWorkspace` right after the team's
  lead is spawned. Attention is the mux's `attention` mark on the pane, which
  the mux rolls up to the window and session.
- **Selection.** `hypercube new` without `-d` selects the new team's window,
  as before. A started team's window is not selected, so reattaching keeps the
  current window. Workers spawn into the team window without taking focus; the
  lead is the window's first pane and so its focused pane.
- **Attach.** `mux.Attach(session)` runs `mux attach <session>` with the
  terminal's stdin, stdout, and stderr. Running `hypercube` inside a mux pane
  is not supported (switching is in the design's "Later" section); the usage
  guide says to run it from a shell outside the mux.
- **Reinstall.** Installing a new `mux` does not restart the running server;
  restarting it would end every pane, including shells Hypercube did not
  start. `scripts/reinstall` stops the running workspaces as it does now. If
  Hypercube's sessions were the server's only ones, the server exits on the
  last kill, and the restarted workspaces start the new `mux`. If the server
  is still running after the stops, reinstall prints that it keeps the old mux
  until its other sessions end. The first reinstall after this change stops
  the workspaces with the installed tmux-based binary and starts them on the
  mux with no extra step.
- **Tests and `bin/mux`.** Building the mux needs libghostty-vt from
  `mux/Makefile`, so the integration test does not build it. It symlinks the
  repository's `bin/mux` beside its own build of `hypercube` and fails with
  "bin/mux is missing; run make build" when absent. `make test` and
  `make check` depend on `build`. Focused `go test -run` of the integration
  test needs `make build` first.
- **Short state directories in tests.** Any test that reaches the mux sets
  `XDG_STATE_HOME` under `os.MkdirTemp("/tmp", "hc-")`, so the socket path
  stays under 104 bytes and the test never touches the user's mux server.
  Dialing a too-long path fails with `EINVAL`, not `ErrNoServer`.
- **Assertions with no socket equivalent.** `list` reports neither labels nor
  the current window, so the integration test drops its window-name check and
  its "reattach keeps the selected window" check. The smoke test covers both.

## Test plan

`cmd/hypercube/integration_test.go`, `TestTmuxNativeLifecycle` renamed
`TestNativeLifecycle`, keeping its stand-in harness and its order:

- Setup: `XDG_STATE_HOME` is `<os.MkdirTemp("/tmp", "hc-")>/state with
  spaces`; drop `TMUX_TMPDIR`, `TMUX`, and the tmux lookup. Symlink `bin/mux`
  beside the built `hypercube`. A cleanup dials the socket and, if a server
  answers, kills every listed session so it exits.
- Replace the `tmux` helper with a `mux` helper that dials, calls one
  `client.Request`, and fails the test on error, plus a `list` helper and
  `windowOf(pane)` / `sessionOf(pane)` read from it. `until` logs the `list`
  tree instead of tmux output.
- Two teams share one mux session (`len(Sessions) == 1`) in two windows, each
  holding its team's panes.
- The `c` shortcut becomes `cli("new", "-d")`, which creates `session-1` in
  the same session; the cancelled-`X` check goes away; the dirty-worktree
  refusal through `delete-session` stays; the confirmed `X` becomes
  `cli("delete-session", shortcutLead.Team)`.
- "Selection and renaming must not affect routing": `select {window:
  otherWindow}` and `label {window: firstWindow, "renamed"}` before creating
  `extra`, which must land in `firstWindow`.
- Typing into a pane: attach one test client to the workspace session at
  200×50 with a goroutine discarding its output, then `select {pane}` and
  `Send` an `input` request. `send-keys … Enter` becomes text plus `"\r"`;
  `C-c` becomes `"\x03"`. The draft, busy, steering, failure, exit-relaunch,
  Ctrl-C, and three-quick-exits checks stay. "Dead pane" reads `Alive == false`
  and `Status == 0` from `list`.
- The stale-target test (a `Target` pointing at another team's pane) goes
  away. After `remove_agent`, the worker's pane is absent from `list` and its
  worktree is gone.
- Closing one window: `kill {window: otherWindow}`, then `cli("start")`
  resumes only that team.
- Stop and restart with queued messages, unchanged.
- The final `X` becomes `cli("delete-session", lead.Team)` on the last team:
  the state file and worktree are gone, `client.Dial` returns `ErrNoServer`
  (the server exited with its last session), and `status` does not list the
  workspace.

Other tests:

- `cmd/hypercube/main_test.go`: `TestDelete` and `TestDeleteMissingCheckout`
  replace the `TMUX_TMPDIR`/`TMUX` lines with a short `XDG_STATE_HOME`.
- `internal/mcp/mcp_test.go`: `TestRemoveDirWorker` and
  `TestSessionBoundaries` the same; `team` saves `Target: "p1-0000"`.

Smoke test, visible in Ghostty with `make dev` (the user watches):

- Start a team; the status bar shows the workspace's base name and `default`,
  and the lead's title shows its label with `idle`.
- Have the lead create a Claude Code worker and an OpenCode worker; both open
  stacked on the right with their labels.
- Exchange messages both ways; titles change between `busy` and `idle`.
- Trigger an approval; the bell shows on the pane, team, and workspace and
  clears once answered.
- Remove a worker; its pane closes.
- `scripts/dev stop`, then `scripts/dev up`; the teams return with their
  conversations resumed.
- Ctrl-b d detaches and closes the window; rerun `~/projects/hypercube-dev/run`
  in Ghostty (`open -na Ghostty --args -e ~/projects/hypercube-dev/run`) and
  the same agents reappear without relaunching.
- `scripts/dev clean`; `ps` shows no `mux server` under the dev state.

## Implementation plan

1. `go.mod`: add the `require` and `replace` for the mux module; run
   `go mod tidy`.
2. `Makefile`: `build` also runs `$(MAKE) -C mux build` and
   `install -m 755 mux/bin/mux bin/mux`; `test` depends on `build`; `install`
   also installs `bin/mux` to `$(BINDIR)/mux`.
3. Create `internal/mux/mux.go` (package comment: Hypercube's panes, windows,
   labels, and attention in the mux, through its socket):
   - `exe()` — the `mux` beside `os.Executable()`.
   - `dial()` — `client.Dial(client.SocketPath())`.
   - `target(req *client.Request, id string)` — sets `Pane`, `Window`, or
     `Session` by the ID's first letter.
   - `Spawn(session, window, dir string, argv []string, env map[string]string)
     (client.Reply, error)` — `DialOrStart(exe)`, one `spawn` with `env` as
     `KEY=value` items.
   - `Kill(id string) error` — nil for an empty ID, no server, or an
     `unknown ` error.
   - `Label(id, text string) error`, `Select(id string) error`.
   - `Attach(session string) error` — runs `mux attach <session>`.
   - `Workspace(s store.State) (string, error)` and
     `Team(s store.State, team string) (string, error)` over one `list` each,
     with a private `find(sessions, pane) (session, window string)`.
   - `label(a store.Agent) string` — the pane label above.
   - `Activity(a store.Agent)` — best effort: if `a.Target` is set and a
     server answers, set the label and `attention {pane, on: a.Attention}`.
   - `RefreshActivity(path, id string)` — loads the agent and calls
     `Activity`; keep its comment about reading the newest state after the
     commit.
4. `internal/harness/harness.go`: `Launch(a, statePath, prompt, session,
   window string) (client.Reply, error)` passes the full `os.Environ()` plus
   `HYPERCUBE_AGENT_ID` and `HYPERCUBE_STATE` to `mux.Spawn` with argv
   `exe agent prompt`, saves `reply.Pane` as the agent's `Target`, calls
   `mux.RefreshActivity`, and returns the reply (with `Pane` set even if saving
   fails). The label string moves to `internal/mux`.
5. `internal/mcp/mcp.go`:
   - `createAgent` loads the state after its update and gets the window from
     `mux.Team(s, a.Team)` (empty means "the session is not running"), calls
     `harness.Launch(a, path, Nudge, "", window)`, and on failure kills
     `reply.Pane` when set.
   - `removeAgent` calls `mux.Kill(a.Target)`.
6. `internal/hook/hook.go`, `internal/wait/wait.go`: call
   `mux.RefreshActivity`. `internal/statusline/statusline.go`: after clearing
   attention, call `mux.RefreshActivity(path, id)`.
7. `cmd/hypercube/main.go`:
   - Delete the `window` case, its `display-message` path, and
     `windowAction`.
   - `stop`: load the state; `mux.Workspace`; empty is "no session for …";
     otherwise `mux.Kill(session)`. Drop the refusal checks.
   - `status`, `running`, and `stopOrDelete --all` decide "running" with
     `mux.Workspace(k.state)`.
   - `deleteTeam(root, path, team)`: drop `expectedWindow` and the refusal
     check; kill `mux.Team(s, team)` for a team, or `mux.Workspace(s)` for the
     workspace.
   - `openWorkspace`: drop the refusal check and `ConfigureSession`; the
     duplicate-team error no longer mentions Ctrl-b w. Compute `missing` with
     `mux.Team`, and `session` with `mux.Workspace` before launching. Launch
     leads with `(session, "")` and workers with `("", windows[a.Team])`.
     After a lead: record `windows[a.Team] = reply.Window`, label it with the
     team; if `session` was empty, take `reply.Session` and label it with
     `filepath.Base(root)`. On a launch error, `mux.Kill(windows[a.Team])`
     with the existing error wording. With `selectNew`,
     `mux.Select(windows[newName])`. Attach with `mux.Attach(session)`.
   - `runAgent`: drop `TMUX_PANE`, `Owns`, and setting `Target`; replace both
     `tmux.Activity` calls with `mux.RefreshActivity(path, id)`.
   - Package comment: "in the mux" instead of "in tmux".
8. Delete `internal/tmux/`.
9. `internal/workspace/workspace.go`: `Name`'s comment drops "and the tmux
   session".
10. Tests: as in the test plan.
11. `scripts/dev`: `up` builds with `make -C "$ROOT" build`; `dev()` and the
    generated `run` script drop `TMUX_TMPDIR`, `TMUX`, and `TMUX_PANE`, so
    `XDG_STATE_HOME` alone gives the dev session its own mux server. Update the
    comment above `dev()`.
12. `scripts/reinstall`: install `bin/mux` beside `hypercube`. After the
    stops, if `"$ROOT/bin/mux" ls` succeeds, print
    `reinstall: the mux server is still running other sessions; it keeps the old mux until they end`.
    Update the header comment.

## Documentation updates

- `AGENTS.md`: "in tmux" becomes "in the mux, the terminal multiplexer in
  `mux/`"; the `internal/` list says `mux` instead of `tmux`; add
  "`mux/` — the terminal multiplexer, its own Go module"; tests use "their own
  mux server"; "Never stop the user's mux server or workspaces".
- `README.md`: "built with Go and tmux" becomes "built with Go and its own
  terminal multiplexer".
- `docs/architecture.md`: the mux is the process and terminal owner; correct
  the stale wrapper sentence to "A launch wrapper runs the harness as a child
  and relaunches it when it exits"; saved pane IDs establish ownership because
  the mux never reuses an ID and an ID from an earlier server names nothing;
  "tmux targets" becomes "pane targets"; the source table's `tmux` row becomes
  `[mux](../internal/mux/mux.go)` — "Panes, windows, labels, attention, and
  attach through the mux's socket."
- `docs/installation.md`: prerequisites replace tmux with Zig 0.17 or newer
  and pkg-config (to build the mux); `make install` installs `hypercube` and
  `mux` together; `make` builds `bin/hypercube` and `bin/mux`; Upgrades adds
  that the mux server keeps the old mux while other sessions run; Development
  says "mux server".
- `docs/usage.md`: "A workspace owns one mux session"; replace the keyboard
  section with the mux's controls (Ctrl-b then `d`, `n`/`p`, `0`–`9`, `o`,
  Ctrl-b; clicks on the status bar's teams and workspaces, and on panes), the
  status bar listing the workspace's teams on the left and workspaces on the
  right, pane titles showing `busy` or `idle`, and the bell for approval on
  the pane, team, and workspace. Delete the yellow unseen-reply text and the
  `c`/`W`/`X` paragraph. Add that `hypercube` runs from a shell outside the
  mux.
