# Architecture

## Processes

- `hypercube`, run in a checkout, is the host. It runs in the user's terminal
  for the whole session and owns every agent's pty and pane and the control
  socket.
- `hypercube mcp` is each agent's MCP server, started by its harness. It
  changes the state file and worktrees itself, and asks the host to launch,
  close, and nudge panes.
- `hypercube hook <event>` runs for each harness hook event and reads and
  writes only the state file.
- `hypercube stop`, `status`, and `delete` are short-lived commands.

A workspace is named from its checkout path (`workspace::name`): the
directory's name and the start of a hash of the path. Outside Git, the
workspace is the directory itself. The name keys every file of the workspace
in the state directory, `$XDG_STATE_HOME/hypercube` or
`~/.local/state/hypercube`:

- `<name>.json`: the state file.
- `<name>.lock`: the state file's lock.
- `<name>.host.lock`: the host lock.
- `<hash>.sock`: the control socket, named from a hash of the workspace name
  so its path stays within the 104 bytes macOS allows.

## The host

`host::run` hosts a workspace:

1. It takes an exclusive, non-blocking `flock` on the host lock and writes its
   terminal's name into the file. A workspace is running while that lock is
   held, and the kernel releases it however the host ends. If another host
   holds it, `hypercube` reports the terminal that host runs in and exits.
2. It readies the state: on the first run it creates the orchestrator, with
   `no_git` set outside Git; it sets the orchestrator's branch, marks it idle,
   and marks the workers busy.
3. It removes a socket file left by an earlier host and listens on the
   control socket.
4. It takes over the terminal with a `Runtime` and launches each agent in its
   own pane, the orchestrator first. Saved sessions resume, and workers start
   with the nudge as their prompt, so they pick up messages queued while they
   were down. If the orchestrator fails to launch, the session ends with
   the error. A worker that fails to launch gets a pane without a program,
   with "failed:" and the error in its label and the error in the pane,
   marked exited with status 127 so the focus skips it, and keeps it until
   `remove_agent` closes it. Agents that don't fit in the terminal still
   launch; their panes are hidden until it grows.
5. It serves until the session ends.

The loop is single-threaded: `Runtime::wait` polls the panes, the host
terminal, the listener, the accepted connections, and a pipe that SIGHUP,
SIGINT, and SIGTERM write to. Connections are non-blocking; a request is
carried out once its line has arrived, and a connection that sends no
complete request within five seconds is dropped, so no client can stall the
panes.

The session ends on a `stop` request, a signal, the end of the terminal's
input, when every pane's program has exited, or when the host panics. The
host then removes every
pane, which sends SIGHUP to each program's process group, gives the programs
up to `HANGUP_GRACE_MS` (two seconds) before SIGKILL, restores the terminal,
removes the socket, and releases the host lock. The next `hypercube` resumes
the agents from their saved sessions and the queued messages.

### Layout and chrome

`host::layout` places the orchestrator's pane in the left 40% of the columns,
or all of them when it is alone, then a one-column rule, then the workers
stacked with equal heights. Each pane has a label row above it, and the bottom
row is the status bar. A layout in which some pane would get no row or column
fails; a launch requested on the control socket then fails with "no room
for another pane", and a terminal resized below that hides the panes until
it grows.

`TeamChrome` draws each label row as a heavy rule in `#0087ff` with the label
in black on `#87d7ff`, bold for the focused pane, followed by "exited N" once
the pane's program has exited. An exited pane keeps its place until
`remove_agent` closes it. The status bar is black on `#87d7ff` with the
checkout path. Ctrl-] moves the focus to the next live pane.

## Control socket

Each connection carries one JSON request line and gets one JSON reply line,
`{"ok":true}` or `{"ok":false,"error":"..."}`. Agents are named by ID.

- `launch {agent, prompt}`: load the agent from the state, build its command
  with `harness::command`, start it in a new pane, and lay the panes out
  again. A non-empty prompt starts its first turn.
- `close {agent}`: hang up the agent's program, remove its pane, and lay the
  panes out again. An agent without a pane is already closed.
- `nudge {agent}`: type `Check your Hypercube messages.` into the agent's
  pane and press Enter 200 ms later, encoded for the pane's keyboard modes.
  A nudge whose Enter is still pending covers the next one for the same
  pane. Fails if the agent has no pane or its program has exited.
- `stop`: reply, then end the session.

The MCP server and `hypercube stop` find the socket from the state file's
path. A request that finds no socket, or nothing listening on it, means no
host is running.

## State file

The state file holds the agents (ID, name, role, harness, model, effort,
directory, branch, session ID, activity), the queued messages, and `no_git`.
Every change takes an exclusive `flock` on the state's lock, reads the file,
and writes the result to a temporary file renamed over it, so hooks, MCP
servers, and the host change it concurrently and readers always see a whole
file.

The state's lock file is created with the state and never written, so its
modification time is the workspace's creation time. Every hook writes the
state, so the state file's modification time is its last activity.
`hypercube status` lists every workspace with a state file by last activity,
with its directory, whether its host lock is held, its number of agents, and
both times.

`hypercube delete` refuses if a worker's worktree is dirty or its branch is
not merged. Otherwise it stops the host and takes the host lock itself, so
no host starts while it works. It removes each worker's worktree and branch,
deletes the state, its lock, and the socket, and removes the host lock file
last. A host that takes the host lock checks that the file it locked is still
the one at the path, and otherwise locks the current one.

## Message delivery

`send_message` queues a message in the state file. If the recipient is idle,
the MCP server asks the host to nudge it, which starts a turn. Hooks deliver:

- SessionStart gives the agent its identity, team, and tools.
- UserPromptSubmit records the session ID, marks the agent busy, and delivers
  queued messages as context.
- PostToolUse delivers messages that arrived during the turn.
- Stop blocks the stop while messages are queued, delivering them, and
  otherwise marks the agent idle.

Each message reaches the model as `Message from <name> (<id>):` and its text.
The hook writes its response inside the state update, so messages it takes
stay queued if the harness cannot read the response.

## Terminal layer

The terminal layer (`src/lib.rs`) runs programs in ptys, keeps a Ghostty
terminal for each, and draws them into the host terminal. It knows nothing
about agents: the caller decides where panes go, what the chrome looks like,
and what to do when something happens.

### Panes

`Mux` holds the panes. Each pane has a stable `PaneId`, a label, a Ghostty
`Terminal`, and, once a program is attached, the program's pty master and
child process.

- `Mux::add_pane(label)` adds an empty pane and focuses it if no live pane
  has the focus.
- `Pty::spawn(argv, cwd, env, size)` starts a program in its own session with
  a new pty as its controlling terminal. `TERM` is `xterm-256color` and
  `COLORTERM` is `truecolor`. The master is non-blocking and close-on-exec,
  so later programs don't inherit it. `Mux::attach(id, pty)` gives the pane its program.
- `Mux::set_layout(&[(PaneId, Rect)])` places panes on the host terminal,
  resizes the terminals and ptys whose size changed, and redraws the screen. The caller lays
  the panes out again after adding or removing one and after the host
  resizes.
- `Mux::close_pane(id)` sends SIGHUP to the program's process group and
  closes the master. The pane stays, and shows its exit status once reaped.
- `Mux::mark_exited(id, status)` shows a pane whose program never started
  as exited with `status`, so the focus skips it.
- `Mux::remove_pane(id)` hangs up the program if it is still running, removes
  the pane, moves the focus to the next live pane, and redraws the screen. A
  removed pane's program is reaped after `HANGUP_GRACE_MS` (two seconds),
  when whatever remains of its process group gets SIGKILL.
- `Mux::send_text(id, text)` types text, and `Mux::send_key(id, key)` presses
  and releases a key. Ghostty's key encoder encodes each key from the pane's
  modes and Kitty keyboard flags, so a program that asks for every key as an
  escape code gets text that way too.

Input for a pane, from the host, from `send_text` and `send_key`, and from
its terminal's replies, is queued and written as the pty takes it. A program
that stops reading its input never blocks the loop.

### Chrome

The caller draws everything around the panes through the `Chrome` trait. The
mux calls `Chrome::draw` inside a frame with a `PaneView` of each pane (ID,
label, rectangle, focus, exit status) and the host size, whenever the panes,
the focus, an exit status, or the host size changed. `Mux::invalidate_chrome`
asks for a redraw after a change only the caller knows about, such as status
bar text.

### Rendering

Each pane captures the rows that changed since its last frame from a Ghostty
render state. A frame draws those rows at the pane's offset, then the focused
pane's cursor, inside synchronized output (mode 2026). A layout change, host
resize, or removed pane clears the screen and draws every row and the chrome.

The focused pane's input modes are mirrored onto the host: cursor and keypad
modes, mouse tracking and formats, focus events, bracketed paste, grapheme
clustering, and the Kitty keyboard flags. Focus events, the window title, and
mode changes follow the focus.

A program that holds drawing with mode 2026 shows the last frame it
completed, captured when the hold began, including when the screen is drawn
again. Resizing a pane's terminal ends its hold. A hold that lasts
`HOLD_TIMEOUT_MS` (one second) ends as if the program had ended it.

### Input

Host input goes to the focused pane. Ctrl-], in its legacy byte or as a Kitty
key press, moves the focus to the next live pane; Kitty repeats and releases
of it are consumed. SGR mouse reports inside the focused pane are translated
to its coordinates; other mouse reports are dropped.

### Runtime

`Runtime::new(mux, input, output)` takes over the host terminal on the given
descriptors: raw mode, the alternate screen, and a round trip of queries for
the host's default colors, its 16 palette colors, and its grapheme clustering
mode, so pane programs that query colors get the host's answers.

The caller drives the loop with `Runtime::wait(mux, extra_fds, deadline)`,
which polls the host input, every pane's master, SIGWINCH, and the caller's
descriptors. It feeds pane output to the terminals, routes host input, writes
queued pane input, renders at most one frame every 16 ms, ends timed-out
holds, and reaps exited programs without blocking. Before it reports an exit,
it reads what the program left in its pty and draws it. It returns when
something needs the caller:

- `Event::Ready(fd)`: one of the caller's descriptors is readable.
- `Event::PaneExited(id, status)`: a pane's program exited; the status is its
  exit code, or 128 plus the signal that ended it.
- `Event::Resized`: the host terminal changed size.
- `Event::InputClosed`: the host terminal's input ended.
- `Event::Deadline`: the deadline passed.

`Runtime::restore()` returns the host terminal to its original settings:
every mirrored mode at its default, no Kitty keyboard flags, the default
cursor, the main screen, and the original terminal attributes. Dropping the
runtime, including when `Runtime::new` fails partway, does the same.

Everything runs on one thread. The terminals and their callbacks share state
through `Rc` and `RefCell`.
