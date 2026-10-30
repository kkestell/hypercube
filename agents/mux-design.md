# A Hypercube-sized tmux replacement

A design sketch, not a plan. Hypercube would be split into two programs with
a narrow interface between them: a small general-purpose terminal multiplexer
("the mux") and Hypercube itself as one of its clients.

## Why

Hypercube works around tmux in [tmux.go](../internal/tmux/tmux.go) because
tmux offers no typed events and stores custom state only as option strings:

- A `sleep 60` placeholder pane exists only so it can be configured before the
  real process starts.
- The launching environment is copied into tmux by hand.
- Exits arrive through a `pane-died` hook that runs a shell command, which then
  re-checks for stale events.
- Ownership markers are re-read before every operation.
- Status display relies on pre-rendered style strings stored in options.
- Unseen replies are inferred from `session_attached && window_active`.
- Key bindings are server-global and guarded by `if-shell`.
- Lifecycle commands from bindings need `nohup`.

An earlier Rust port replaced tmux with a libghostty-vt multiplexer and was
abandoned. It fused the multiplexer with Hypercube's
logic, and it grew to about 10k lines of terminal code that handled obscure
edge cases nobody asked for. This design keeps the two apart, and it fixes the
mux's scope before any code exists.

## The split

**The mux** owns terminals: PTYs, libghostty-vt screen state, drawing,
input, mouse, scrollback, selection, and attach. It knows nothing about
agents, harnesses, messages, worktrees, or Hypercube's state. It makes sense
on its own with plain shells in its panes.

**Hypercube** keeps everything it has now: the store, the MCP server, hooks,
`wait`, worktrees, and relaunch policy. A client package replaces `tmux.go`
and talks to the mux only through the interface below.

Upgrading Hypercube never touches running panes. Only a mux upgrade does.

## Model

One mux server per user, listening on `$XDG_STATE_HOME/mux/socket` (default
`~/.local/state/mux/socket`) and started by the first request that creates a
session. It exits when its last session is removed. It holds sessions, each session holds windows, and each
window holds panes.

| Mux | Hypercube |
| --- | --- |
| session | workspace |
| window | Hypercube session (team) |
| pane | agent |

IDs are assigned by the server and never reused while it runs. Each ID
includes a random part chosen when the server starts, so a saved ID from an
earlier server names nothing and returns an "unknown" error. A saved ID
therefore proves ownership. Names do not exist; whatever a user sees is a
label set by the client.

## Interface

JSON lines over the socket. Each request gets one reply, `{"ok":true,...}` or
`{"error":"..."}`.

### Requests

| Request | Reply | Behavior |
| --- | --- | --- |
| `spawn {session, window, dir, argv, env}` | `{session, window, pane}` | Creates the session and window if `session` or `window` is empty. The process gets exactly `env`, nothing inherited. |
| `respawn {pane, dir, argv, env}` | | Kills the pane's process if it is alive and starts the new one in the same pane with a fresh screen. |
| `kill {pane \| window \| session}` | | Hangs up the processes and removes the target. Removing a session's last window removes the session. |
| `attention {pane, on}` | | Marks the pane as needing attention with a bell icon before its label, or clears the mark. A window or session shows the bell while any of its panes is marked. |
| `collapse {pane, on}` | | Collapses a stacked pane to its title row, or opens it. The main pane cannot collapse. |
| `label {pane \| window \| session, spans}` | | Sets the text shown for the target, or restores the default when `spans` is empty. A span is `{text}`. |
| `select {window \| pane}` | | Makes the window its session's current window, or the pane its window's focused pane, and that window current. |
| `list` | sessions, windows, and panes with `alive` and `status` | A signal death's status is 128 plus the signal number. |
| `capture {pane}` | `{text}` | Returns the pane's screen as plain text. |

### Command line

The `mux` command is a socket client like any other. Its commands mirror the
requests, so the mux works by hand with plain shells and no Hypercube:

| Command | Request |
| --- | --- |
| `mux spawn [-s session] [-w window] [-c dir] [command...]` | `spawn`, running `$SHELL` when no command is given; prints the IDs |
| `mux kill <id>` | `kill` |
| `mux attention <pane> on\|off` | `attention` |
| `mux collapse <pane> on\|off` | `collapse` |
| `mux select <id>` | `select` |
| `mux label <id> [text...]` | `label`, with the text as one span; no text restores the default |
| `mux ls` | `list` |
| `mux attach [session]` | attach |

`mux attach` is the mux's own terminal client. It takes over the terminal and
detaches on Prefix d. With no session and none running, it spawns a shell in a
new session first. The server holds every screen and draws the frames; the
client only passes frames to the terminal and input, size, and focus changes
to the server. Hypercube runs it in place of `tmux attach-session`.

## Fixed behavior

These rules have no options:

- **Layout:** the first pane in a window is the main pane, on the left at 40%
  of the width. Dragging the divider changes that window's split. Other panes stack on the right in equal heights. Clicking a
  stacked pane's title collapses it to that title row or opens it again.
- **Exited panes:** a pane stays after its process exits, showing the exit
  status in its title, until it is respawned or killed.
- **Screen chrome:** a status bar on the bottom row lists the current
  session's windows on the left and the sessions on the right, and each pane
  has a title row. Clicking a
  session, window, or pane selects it.
- **Progress:** a pane whose program reports progress with OSC 9;4 shows it
  in its title: a circle filled in eighths and the percentage, or an icon for
  indeterminate, error, or paused. The progress clears when the program exits.
- **Bell and window title:** a pane's bell, or an `attention` mark turning on,
  rings the attached terminal's bell. The terminal's window title is
  "session | window | pane" for what the client shows, and detaching restores
  the previous title.
- **Icons:** the bell and progress icons are Nerd Font symbols, so they show
  only if the attached terminal's font has them.
- **Default labels:** until a client sets a label, a pane shows the title its
  program sets, or else its command name. A window shows its first pane's
  label, and a session shows its first window's label.
- **Keys:** the prefix is Ctrl-b. The keys are `d` detach, `n` and `p`
  next and previous window, `0`-`9` window by number, `o` next pane, and
  Ctrl-b to send a literal Ctrl-b.
- **Input:** keys go to the focused pane as typed. The client sets the outer
  terminal's keyboard mode to the one the focused pane requested.
- **Sizes:** with no client attached, a pane is 80 by 24.
- **Mouse and scrollback:** a pane that requests mouse tracking receives the
  mouse. Otherwise the wheel scrolls its scrollback, and a drag selects text
  and copies it through OSC 52 on release. Each pane keeps 10,000 lines of
  scrollback.

## Hypercube on the mux

- `spawn` replaces the placeholder pane, the environment copying, and
  `respawn-pane -k`. Hypercube saves the pane ID from the reply as the
  agent's `Target`.
- Saved pane IDs replace the `@hypercube_*` markers, `Owns`, `FindPane`,
  `FindWindow`, and `WorkspaceSession`. Only the pane ID is saved: a team's
  window is the one holding its lead's pane, and a workspace's session is the
  one holding its panes, both found through `list`. An "unknown" error means
  the saved ID is stale.
- `hypercube agent` runs the harness as a child process instead of replacing
  itself with it. When the harness exits, the wrapper relaunches it with the
  saved conversation and applies `QuickExits`; after the last quick exit the
  wrapper exits and the pane shows the status. This replaces the `pane-died`
  hook, `hypercube exited`, `Restart`, `Dead`, and the extra lock, and the mux
  needs no exit events. The wrapper catches SIGINT rather than ignoring it,
  since an ignored signal stays ignored in the child.
- `label` replaces the format strings, escaping, and stored style options.
  Hooks, `wait`, and the status line set labels through the socket, and
  `attention` marks agents that need the user.
- Hypercube's `c`, `W`, and `X` bindings and its error reporting through
  `display-message` go away; `hypercube new`, `hypercube` in another
  directory, and `hypercube delete-session` do the same from a shell.
  Unseen-reply marks go away with `markReply` and the two session hooks.

## Non-goals

The mux has no configuration file, format language, hooks, `run-shell`,
general splits or layout commands, zoom, copy mode,
`send-keys`, scripting, plugins, splash screen, or restore after its own
restart.

## Accepted limitations

These are documented, not handled in code:

- A mux restart or upgrade ends every pane's process. Hypercube resumes agents
  from saved conversations.
- Keys typed while the client queries the terminal at attach may be lost.
- Panes that do not fit the terminal are not drawn until it grows.
- One client per session; a second attach detaches the first.
- Text selection works within one pane at a time.

## Testing

Tests talk to the socket only: spawn `sh` or a stand-in in a pane, write to
it, and assert on `list` and screen text. Hypercube's tests run
against a real mux server with isolated state, as they now run against an
isolated tmux server.

## Milestones

1. **The mux on its own.** The server, `spawn`, `respawn`, `kill`, `list`,
   and `capture`; the command line; `mux attach` with the fixed layout, chrome,
   default labels, built-in keys, mouse, scrollback, and selection. Done when
   shells can be spawned, used, killed, detached from, and reattached to by
   hand, and the socket tests pass. Nothing in Hypercube changes.
2. **Hypercube on the mux.** The few mux changes Hypercube depends on, then
   Hypercube's switch from tmux to the mux. Done when Hypercube runs its
   teams on the mux, its tests drive the mux through the socket, and nothing
   in Hypercube calls tmux.

### Milestone 1 tasks

Tasks 1 to 5 are checked by tests through the socket, using `capture` to read
the screen. Tasks 6 to 8 are checked by hand.

1. **Skeleton and minimal working path.** The `mux/` module with its own
   `go.mod` and Makefile, which builds libghostty-vt and runs the tests; the
   top-level `make check` runs it. The server listens on
   `$XDG_STATE_HOME/mux/socket` and the command line starts it on first use.
   `spawn` creates a session, window, and pane whose PTY feeds an 80 by 24
   terminal with 10,000 lines of scrollback. `capture`, and `mux spawn
   [command...]`, which prints the IDs. A bare `mux attach <session>` shows
   the one pane full-screen and detaches on Ctrl-b d.
   - Check: with `XDG_STATE_HOME` set, a test spawns `sh` with a given
     directory and environment, writes `pwd; env`, and `capture` shows exactly
     that directory and environment. By hand, `mux spawn` then `mux attach`
     shows a working shell, and Ctrl-b d returns to the outer shell.
2. **Exits and `list`.** Exited panes stay with `alive` false and their
   status. `list`, `mux ls`, and spawning into an existing session or window.
   - Check: `sh -c 'exit 3'` lists status 3; `sh -c 'kill -TERM $$'` lists
     status 143; a second spawn appears under the same window; unknown IDs
     return errors.
3. **`kill` and server exit.** Kill a pane, window, or session; removing a
   session's last window removes the session. The server exits after its last
   session. IDs are never reused. `mux kill <id>`.
   - Check: a test for each kind of target; an ID after a kill is new; killing
     the last session removes the socket, and the next `mux spawn` starts a
     new server.
4. **`respawn`.**
   - Check: respawn a dead pane and a live pane; `list` shows the same pane ID
     alive, and `capture` no longer shows the old output.
5. **Layout and resizing.** The main pane on the left at 40% and the rest
   stacked on the right. The attached terminal's size sets the pane sizes,
   which go back to 80 by 24 on detach. Panes that do not fit are skipped.
   Focus, input routing, and cursor placement.
   - Check: a test attaches at a given size and checks `stty size` in each
     pane through `capture`.
6. **Bars and default labels.**
   - Check by hand: `printf '\033]0;hello\007'` changes the labels, and
     `exit 3` shows the status in the pane's title.
7. **Keys, keyboard mode, and attach rules.** `n`, `p`, `0`-`9`, and `o`;
   Ctrl-b Ctrl-b sends a literal Ctrl-b; the keyboard mode follows the focused
   pane; one client per session; `mux attach` with nothing running spawns
   `$SHELL`; reattaching redraws.
   - Check by hand.
8. **Mouse, scrollback, and selection.**
   - Check by hand: `seq 20000` keeps the last 10,000 lines; a drag copies
     through OSC 52; vim with `mouse=a` gets the mouse; clicks switch sessions,
     windows, and panes.
9. **Acceptance.** The hand walkthrough plus `make check`.

### Milestone 2 pieces

In order.

1. **Relaunch check.** Before anything else, check by hand in `make dev`
   that Claude Code and OpenCode behave when `hypercube agent` runs them as
   child processes: Ctrl-C, exit, and relaunch. If they do not, stop and
   redesign how Hypercube learns of exits.
2. **Mux changes.** IDs with the per-server random part; `select {window |
   pane}`; only `spawn` and a bare `mux attach` start a server, so `mux ls`
   and other requests leave none behind; `attention` on panes only, rolled
   up to windows and sessions. Checked by socket tests.
3. **Relaunch in the wrapper.** `hypercube agent` runs the harness as a child
   and relaunches it, still on tmux. Delete `hypercube exited`, `Restart`,
   `Dead`, and the `pane-died` hook. Checked by Hypercube's tests.
4. **Hypercube's switch.** A small mux client package in place of
   `internal/tmux`: spawn with the full environment and labels; kill, label,
   attention, and select; lookups through `list` from saved pane IDs; attach
   by running `mux attach`. Remove the `c`, `W`, and `X` bindings and the
   `display-message` path. `scripts/dev`, `make install`, and
   `scripts/reinstall` build and run the mux. The integration tests attach
   through the socket, select, and send input instead of using `send-keys`
   and control mode, and the corrupted-marker test goes away.

### Later

Each is its own change after the switch, made when it is needed.

- **Keys and prompts.** If `c`, `W`, or `X` come back, Hypercube runs the
  attach client in its own process. The attach request carries the keys to
  bind, key presses and answers come back on the attach connection, and
  prompts, confirms, messages, and switching to another session go up it.
  This needs no client IDs, global key table, or event subscription.
- **Unseen replies.** One request that marks a pane unless a client shows its
  window and clears the mark when one does. The server already knows what
  each client shows.
- **Switching.** Running `hypercube` inside a pane moves the attached client
  instead of nesting a second one.
- **Colored labels.** Spans with `fg` and `bold`.
- **Frame pacing.** At most about 60 frames a second, if a smoke test with
  several busy agents shows full redraws are a problem.
- **Crash logging.** A server log under `$XDG_STATE_HOME/mux/`, so a panic
  leaves a trace.

## Decisions

- The mux has its own Makefile, and the top-level `make check` runs it, so
  checks need Zig 0.17 or newer and pkg-config.
- The mux is its own Go module in `mux/` in this repository. It cannot import
  Hypercube's packages; Hypercube uses the mux only through the mux's client
  package.
- The mux is written from scratch in Go on `go.mitchellh.com/libghostty`
  (cgo; no API stability yet). It links `libghostty-vt`, built from Ghostty's
  source with Zig 0.17 or newer and found through pkg-config. Planning and
  implementation do not consult the Rust port's code.
- Desktop notifications (OSC 9 and OSC 777) are not passed through yet. They
  are planned for later.
