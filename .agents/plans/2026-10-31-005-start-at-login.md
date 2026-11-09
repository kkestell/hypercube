# Start Hypercube at login with a menu bar icon

Replaces `2026-10-31-004-menu-bar-icon.md`. Builds on the uncommitted
`restoreWorkspaces` change in `cmd/hypercube/main.go`, its integration test,
and its line in `docs/installation.md`, which this plan keeps and extends.

## Goal

At login, Hypercube starts by itself. A menu bar icon has **Show Hypercube**,
which opens the browser view in the default browser, and **Quit Hypercube**.
Every saved workspace and session is running and visible, but no agent starts a
turn until the user types a prompt into that session, even if an agent was in
the middle of a turn when the computer shut down. Installing a new build
restarts the menu bar icon and browser view without stopping any agent.

## Lifecycles

Each part below starts, ends, and is restarted by one owner.

- **launchd login service** (`~/Library/LaunchAgents/com.github.kkestell.hypercube.plist`).
  macOS starts it when the user logs in, which after a reboot with automatic
  login means at boot. It runs `zsh -lic 'exec hypercube ui 4280 --menu'`, so
  Hypercube gets the user's login environment. If the process crashes, launchd
  starts it again. If the user quits from the menu, launchd leaves it stopped
  until the next login or the next `make install-service`.
- **Hypercube app process** (`hypercube ui --menu`). One process does three
  things: it draws the menu bar icon, serves the browser view on port 4280, and,
  once at startup, starts every saved workspace that is not already running.
  It owns no agents. Killing or restarting it removes the icon and drops browser
  connections for a moment; the page reconnects every second until the new
  process answers. Agents keep running.
- **Browser page**. Each open page asks the app process for a live stream. The
  app process starts one `tmux -C attach-session` child per page and relays
  between them. Closing the page ends that child and nothing else.
- **tmux server**. The first tmux command starts it, and it then detaches into
  its own process, so it outlives whatever started it. It keeps running until
  its last session closes, the user logs out, or the computer shuts down. It
  copies the environment of the command that started it into every pane it
  creates. At login, the app process's startup restore is the first tmux
  command, so the server gets the login service's environment.
- **Workspace, session, agent in tmux**. A workspace is one tmux session. Each
  Hypercube session (a team) is one tmux window. Each agent is one pane.
- **Agent pane process**. The pane runs `hypercube agent`, which records the
  pane in the state file and replaces itself with the harness (`claude`,
  `opencode`, `codex`, or `pi`), resuming the saved conversation. tmux owns the
  harness process. The harness starts its own helpers: `hypercube mcp` (the
  agent's tools, alive as long as the harness), hook commands that run
  `hypercube report` and exit, and `hypercube wait` (a waiter that watches the
  state file while the agent is idle and starts a turn when messages are
  queued). When the harness exits, tmux's `pane-died` hook runs
  `hypercube exited`, which relaunches it in the same pane unless it exited
  quickly three times in a row.
- **State files** (`~/.local/state/hypercube/<workspace>.json`). The durable
  record of every workspace: its agents, their conversation IDs, titles, and
  queued messages. They survive everything above. A workspace is registered
  while its state file exists.

What happens on each event:

| Event | App process | tmux server and agents | State files |
| --- | --- | --- | --- |
| Login or boot | launchd starts it; it restores every saved workspace | Restore starts the server and every agent, resuming conversations; every restored session waits for the user | Read; launch details rewritten |
| `make install-service` | Stopped and started from the new build | Untouched; new hook and tool calls use the new binaries at once | Untouched |
| Quit Hypercube | Exits; launchd leaves it stopped | Untouched | Untouched |
| App process crash | launchd restarts it | Untouched | Untouched |
| Harness exits | — | `pane-died` relaunches it in its pane | Launch details rewritten |
| Logout or shutdown | Ends | Ends; agents mid-turn lose that turn | Kept, so the next login restores them |

## State

The state files stay the only durable record, in JSON. tmux holds only facts
about live processes: which panes exist and which agent each one runs. Restore
needs nothing that the state files lack, and the new "waiting for the user"
flag is one more field in them. SQLite would change the storage format without
changing what is stored or who owns it, so this change does not need it.

Two places copy state into tmux, because the browser view reads everything
through a tmux control client: `tmux.Activity` copies busy, attention, status,
and usage into pane options, and `retitle` copies the title into the window
name. Both are rewritten from the state files on every change and on every
launch, so they cannot drift for long. This change adds no new copy.

## Related code

- `cmd/hypercube/main.go` — `serveUI` listens, calls `restoreWorkspaces`, and
  serves; `restoreWorkspaces` calls `openWorkspace` for each saved workspace
  whose tmux session is missing; `openWorkspace` relaunches missing teams and
  passes `mcp.Nudge` to relaunched workers; `running` exists only for
  `scripts/reinstall`.
- `internal/store/store.go` — `Agent`, `State.TeamLead`. Session-wide facts such
  as `Title` live on the session's lead.
- `internal/wait/wait.go` — `Wait` delivers queued messages when the agent is
  idle. Every harness plugin's waiter goes through it.
- `internal/report/report.go` — the `prompt` case runs when the user submits a
  prompt, and also when a waiter's delivery starts a turn.
- `internal/mcp/mcp.go:314` — a newly created worker launches with `Nudge`;
  this stays.
- `internal/ui/ui.go` — `workspaceAction`'s `start` case exists only for
  `scripts/reinstall`; `internal/ui/ui_test.go:99` covers it.
- `scripts/install-service` — writes the property list, unloads the old service,
  waits for the port, and loads the new one.
- `scripts/reinstall`, `Makefile` (`install-service`, `reinstall-ui`,
  `reinstall`).
- `cmd/hypercube/integration_test.go` — `TestTmuxNativeLifecycle`; its stand-in
  harness reports `prompt` for typed lines and logs what each report returns.

## Decisions

- **One app process.** The menu bar icon, the browser view, and the startup
  restore run in `hypercube ui --menu`. The icon is present exactly while the
  view serves, and one `launchctl` command restarts both.
- **Menu.** The standard form for a menu bar app: `Show Hypercube`, a separator,
  and `Quit Hypercube` (⌘Q). Show opens `http://localhost:<port>/` with
  `NSWorkspace openURL:`. Quit calls `[NSApp terminate:]`, which exits with
  status 0.
- **launchd restarts only after a failure.** `KeepAlive` becomes
  `<dict><key>SuccessfulExit</key><false/></dict>`, so Quit stays quit and a
  crash or a failed serve (exit status 1) is restarted. `RunAtLoad` stays.
- **No dependency for the icon.** `internal/menu` is one Go file and one
  Objective-C file through cgo. The Objective-C side does everything itself, so
  there are no callbacks into Go: it sets the accessory activation policy (no
  Dock icon, not in the app switcher), creates the status item with the SF
  Symbol `cube` as a template image so it follows light and dark menu bars,
  builds the menu, and runs `[NSApp run]`.
- **Main thread.** Cocoa must run on the main thread. `internal/menu` locks the
  main goroutine to it in `init`, and `serveUI` calls `menu.Run` on the main
  goroutine after starting the HTTP server in another goroutine. If serving
  fails, that goroutine prints the error and calls `os.Exit(1)`.
- **Flag.** `hypercube ui [port] [--menu]`, parsed by hand in a new
  `parseUIArgs`. Only the property list passes `--menu`; `make dev`, tests, and
  manual runs show no icon.
- **Restore order.** `serveUI` listens first, so a port in use fails before
  anything starts; then restores; then shows the icon and serves. Restore runs
  on every app start and skips workspaces whose tmux session exists, so a
  restart after `make install-service` touches no running agent.
- **Restored sessions wait for the user.** A new `Agent.AwaitingUser` field,
  set only on leads like `Title`, marks a session that is waiting for the user.
  `openWorkspace` sets it on the lead of every existing team it relaunches,
  whether at login or from `hypercube start`, and never on a team it creates.
  `State.AwaitingUser(team)` reports it. `wait.Wait` delivers nothing while the
  agent's session is waiting, so queued messages stay queued. The `prompt`
  report clears the flag on the session's lead. While a session waits no waiter
  delivers, so any `prompt` in it is the user's own, typed into any of the
  session's agents. Once cleared, the session's other waiters see it on their
  next poll and deliver as usual. Sessions are released one at a time, so
  prompting one session does not wake another.
- **Relaunches send no prompt.** `openWorkspace` launches every relaunched
  agent, worker or lead, with an empty prompt. A harness resumed without a
  prompt sits idle, including one that was mid-turn: all four plugins pass the
  saved session and add a prompt only when one is given. `mcp.Nudge` remains
  the first prompt of a newly created worker.
- **One install command.** `make install-service` builds, installs, rewrites the
  property list, and restarts the service. It is both the first install and the
  upgrade. `reinstall-ui`, `reinstall`, and `scripts/reinstall` go away, with
  `hypercube running` and the view's `start` action, which only
  `scripts/reinstall` used. The service's unload kills the app process and its
  `tmux -C` children; the tmux server is in its own process session, so it and
  the agents survive. Running harnesses keep the launch arguments and the
  `hypercube mcp` and `hypercube wait` processes they started with until they
  next launch.

## Test plan

- `TestParseUIArgs` (`cmd/hypercube/main_test.go`): `{}` is 4280 without menu;
  `{"5000"}` is 5000 without menu; `{"--menu"}` is 4280 with menu;
  `{"5000", "--menu"}` and `{"--menu", "5000"}` are 5000 with menu. Errors for
  `{"0"}`, `{"70000"}`, `{"abc"}`, `{"5000", "6000"}`, and `{"--bogus"}`.
- `wait` tests (`internal/wait/wait_test.go`): an idle agent with queued
  messages whose lead has `AwaitingUser` gets nothing and keeps waiting;
  clearing the flag delivers the messages to that waiter.
- `report` tests (`internal/report/report_test.go`): `prompt` from a worker
  clears its lead's `AwaitingUser` and leaves other sessions' leads waiting.
- `TestTmuxNativeLifecycle`, replacing the "stopped queue delivered after
  resume" steps: after the view restores the stopped workspace, wait until the
  lead and `otherWorker` have relaunched and started waiters; check that both
  leads have `AwaitingUser`, that neither launch carried `Nudge`, and that
  after one second neither queued message was delivered. Type a line into the
  lead's pane; its `prompt` report returns "queued while stopped", and
  `otherWorker` still has "other queued while stopped" queued. Type a line into
  the other session's lead; `otherWorker` then wakes with its message. The
  "closed window resumed" step still sees `otherWorker` relaunch with its saved
  session.
- `internal/ui/ui_test.go`: drop the `/workspace?action=start` case.
- Live, run by the user after `make install-service`:
  - A template cube icon appears and no Dock icon. Show Hypercube opens the
    view. The icon is legible in light and dark menu bars.
  - `make install-service` again: the icon goes and comes back, the page
    reconnects, and `tmux list-panes -a` shows the same pane processes.
  - Quit Hypercube: the icon goes, it stays gone, and agents keep running.
  - Reboot: the icon appears, every workspace and session is in the view, every
    agent shows its resumed conversation, none is busy, and a message queued
    before the reboot is delivered only after the user prompts that session.
  - Panes have color: `NO_COLOR` is absent from `tmux show-environment -g`.

## Implementation plan

1. `internal/store/store.go`: add `AwaitingUser bool \`json:"awaiting_user,omitempty"\``
   to `Agent`, documented as set on a session's lead, and
   `func (s *State) AwaitingUser(team string) bool`.
2. `cmd/hypercube/main.go` `openWorkspace`: in the `store.Update` that collects
   `launch`, set `a.AwaitingUser = true` on leads of relaunched teams other than
   the new `team`; launch every agent with an empty prompt and remove the
   `mcp.Nudge` choice.
3. `internal/wait/wait.go`: treat a waiting session like a busy agent in both
   the unlocked check and the locked recheck.
4. `internal/report/report.go` `prompt` case: clear `AwaitingUser` on
   `s.TeamLead(a.Team)`.
5. `internal/menu/menu.go` and `internal/menu/menu.m` (new): `init` locks the OS
   thread; `Run(url string)` passes the URL to the Objective-C function that
   sets the activation policy, creates the status item and menu, and runs the
   application.
6. `cmd/hypercube/main.go`: `parseUIArgs`; `serveUI` listens, restores, starts
   `http.Serve` in a goroutine that exits the process with status 1 on failure,
   and then calls `menu.Run` with `--menu` or blocks on serving without it;
   allow `ui` with up to two arguments; update `usage`; remove `running`.
7. `internal/ui/ui.go`: remove the `start` case from `workspaceAction` and its
   comment line; update `internal/ui/ui_test.go`.
8. `scripts/install-service`: pass `ui $PORT --menu`; use the `KeepAlive`
   dictionary.
9. `Makefile`: remove `reinstall-ui` and `reinstall`; describe
   `install-service` as installing the build and restarting the service.
   Delete `scripts/reinstall`.
10. Tests from the test plan.

## Documentation updates

- `docs/installation.md`: the service paragraph says Hypercube starts at login
  with a menu bar icon whose Show Hypercube opens the view, that it starts
  every saved workspace that is not running, that restored sessions start no
  turn until you type a prompt into them, and that Quit Hypercube stops the
  icon and view until the next login while agents keep running. Upgrades
  becomes one paragraph: `make install-service` installs the build and restarts
  the icon and view without stopping agents; running agents use the new build
  for hooks and tools at once and for launch arguments when they next launch.
- `docs/usage.md`: the `ui` row gains `[--menu]`; remove the `running` row; the
  Resume section says a relaunched session starts no turn and delivers no
  queued message until you prompt it.
- `AGENTS.md`: remove the `scripts/reinstall` line; "never run
  `make reinstall`" becomes "never run `make install-service`"; the
  `reinstall-ui` sentence becomes "After a change is merged, the user runs
  `make install-service` from the original checkout to load it."
