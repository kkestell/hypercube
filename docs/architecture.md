# Architecture

## Processes

- `hypercube`, run in a checkout, is the host. It runs in the user's terminal
  for the whole session and owns every agent's pty and pane and the control
  socket.
- `hypercube mcp` is each agent's MCP server, started by its harness. It
  changes the state file and worktrees itself, reads the profiles from
  `config.json`, and asks the host to launch, close, and nudge panes.
- `hypercube hook <event>` runs for each harness hook event. It changes the
  state file, and reads `config.json` for the orchestrator's profiles.
- `hypercube stop`, `status`, and `delete` are short-lived commands. `stop` and
  `delete` act on the workspace of the working directory, of a directory
  argument, or, with `--all`, of every workspace.

Each agent's harness runs with `HYPERCUBE_AGENT_ID` and `HYPERCUBE_STATE`, the
agent's ID and its workspace's state file, which the MCP server and the hooks
read to find their agent.

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

The state directory also holds `pi-extension/index.js`.

## The host

`host::run` hosts a workspace:

1. It takes an exclusive, non-blocking `flock` on the host lock and writes its
   terminal's name into the file. A workspace is running while that lock is
   held, and the kernel releases it however the host ends. If another host
   holds it, `hypercube` reports the terminal that host runs in and exits. It
   also exits unless its input and output are terminals.
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
input, when every pane's program has exited and none is relaunched, or when
the host panics. The host then removes every pane, which sends SIGHUP to each
program's process group, gives the programs up to `HANGUP_GRACE_MS` (two
seconds) before SIGKILL, restores the terminal, removes the socket, and
releases the host lock. The next `hypercube` resumes the agents from their
saved sessions and the queued messages.

### Relaunching

When an agent's program exits, the host starts its harness again in the same
pane, resuming the saved session as at startup: `harness::command` with the
agent's session ID and the nudge as a worker's prompt. The host first marks the
agent idle (orchestrator) or busy (worker) in the state, so the new harness's
hooks change it afterwards. The pane keeps its place, its size, and its focus.
It gets a new terminal, so nothing the old program left half written carries
over, the old program's process group is hung up, and the host input aimed at
the old program is dropped: a selection, a mouse press in progress, held input,
and the rest of a paste. A nudge's Enter still pending for the pane is
cancelled.

A program that exits within `QUICK_EXIT_MS` (ten seconds) of starting has
exited quickly. After `MAX_QUICK_EXITS` (three) quick exits in a row the host
leaves the pane showing "exited N" and gives up its focus, until `remove_agent`
closes it. An exit after a longer run resets the count. A relaunch whose
harness fails to start shows the failure as a worker's failed launch does, and
is not retried.

The host handles every exit `Runtime::wait` reports before it checks whether
every pane's program has exited for good, which ends the session. Closing an
agent and ending the session hang up the programs without relaunching them.

### Layout and chrome

`host::layout` places the orchestrator's pane in the left 40% of the columns,
or all of them when it is alone, then a one-column rule, then the workers
stacked with equal heights. Each pane has a label row above it, and the bottom
row is the status bar. A layout in which some pane would get no row or column
fails; a launch requested on the control socket then fails with "no room
for another pane", and a terminal resized below that hides the panes until
it grows.

`host::zoom_layout` gives one pane the rectangle a lone pane has, so it fills
the pane area, and hides the others. A hidden pane has an empty rectangle: it
is not drawn, takes no mouse input (a press it had captured ends there), and
keeps its size, so its program sees no resize. The host zooms the focused pane
when the zoom key is pressed. It restores the normal layout when the key is
pressed again, when the focus changes, and when a pane is added or removed. The
host resizing keeps the zoom.

`TeamChrome` draws each label row as a heavy rule in `#0087ff` with the label
in black on `#87d7ff`, bold for the focused pane. The label is the agent's
name, harness, model, effort, and directory, followed by "zoomed" while
the pane fills the pane area, "exited N" once the pane's program has exited
and is not relaunched, and "↑N" while its view is scrolled N rows up. The
status bar is black on `#87d7ff`, with the checkout path on the left.

The keys and the mouse (see Input):

- Ctrl-] moves the focus to the next live pane.
- Ctrl-\ zooms the focused pane, or restores the layout.
- A click focuses the pane under the pointer.
- The wheel scrolls the pane under the pointer, unless its program reports
  the mouse.
- Shift-PageUp scrolls the focused pane up a page. While it is scrolled,
  PageUp, PageDown, Up, and Down scroll it, Esc and q return it to the
  bottom, and any other key returns it to the bottom and goes to its
  program.
- A drag selects text and copies it to the clipboard on release.

Labels and the status bar are clipped to whole grapheme clusters, measured in
columns.

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

`launch` fails if the agent already has a pane or the terminal has no room for
another. `launch` and `close` end any zoom.

## State file

The state file holds the agents (ID, name, role, harness, model, effort,
directory, branch, session ID, activity), the queued messages (sender,
recipient, text), and `no_git`.
Every change takes an exclusive `flock` on the state's lock, reads the file,
and writes the result to a temporary file renamed over it, so hooks, MCP
servers, and the host change it concurrently and readers always see a whole
file.

The state's lock file is created with the state and never written, so its
modification time is the workspace's creation time. Every hook writes the
state, so the state file's modification time is its last activity.
`hypercube status` lists every workspace with a state file by last activity,
with its directory, whether its host lock is held, its number of agents, and
both times. A workspace's directory is its orchestrator's.

A worker created without a directory has the branch `hypercube/<name>` and
the worktree `<checkout>/.hypercube/<name>`, created from the orchestrator's
HEAD. `.hypercube/` is in the checkout's local Git excludes, and the directory
is removed with the last worktree. A workspace outside Git has no branches or
worktrees: each worker works in the directory `create_agent` gives it.

`hypercube delete` refuses if a worker's worktree is dirty or its branch is
not merged. Otherwise it stops the host and takes the host lock itself, so
no host starts while it works. It removes each worker's worktree and branch,
deletes the state, its lock, and the socket, and removes the host lock file
last. A host that takes the host lock checks that the file it locked is still
the one at the path, and otherwise locks the current one.

`hypercube stop --all` lists the directory of every workspace whose host lock
is held, and `hypercube delete --all` lists every workspace with a state file.
Each command asks `Stop all of these? [y/N]` or `Delete all of these? [y/N]`
and continues only on `y` or `yes`; any other answer ends it with
"stop cancelled" or "delete cancelled". It then runs the single-workspace
command on each workspace in turn, with the same refusals. A workspace that
fails does not stop the rest: afterwards the command exits with an error that
lists each failure as `<directory>: <reason>`. With nothing to act on, it
prints `No workspaces to stop.` or `No workspaces to delete.`.

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

The terminal layer (`mux`, `pty`, `runtime`, and `winch`) runs programs in
ptys, keeps a Ghostty terminal for each, and draws them into the host terminal.
It knows nothing about agents: the caller decides where panes go, what the
chrome looks like, and what to do when something happens.

### Panes

`Mux` holds the panes. Each pane has a stable `PaneId`, a label, a Ghostty
`Terminal`, and, once a program is attached, the program's pty master and
child process.

- `Mux::add_pane(label)` adds an empty pane and focuses it if no live pane
  has the focus.
- `Pty::spawn(argv, cwd, env, size)` starts a program in its own session with
  a new pty as its controlling terminal. `TERM` is `xterm-256color` and
  `COLORTERM` is `truecolor`. The master is non-blocking and close-on-exec,
  so later programs don't inherit it. `Mux::attach(id, pty)` gives the pane
  its program.
- `Mux::set_layout(&[(PaneId, Rect)])` places panes on the host terminal,
  resizes the terminals and ptys whose size changed, and redraws the screen.
  A pane given an empty rectangle is hidden and keeps its size. The caller
  lays the panes out again after adding or removing one and after the host
  resizes.
- `Mux::close_pane(id)` sends SIGHUP to the program's process group and
  closes the master. The pane stays, and shows its exit status once reaped.
- `Mux::mark_exited(id, status)` shows a pane whose program never started
  as exited with `status`, so the focus skips it.
- `Mux::restart(id, label)` readies a pane whose program has exited for a
  new one: a new terminal of the same size, a replaced label, and the old
  program's process group hung up. It drops the host input aimed at the old
  program: the selection, a mouse press in progress, held input, and the rest
  of a paste through its end marker. The caller then attaches the new program
  with `Mux::attach`, or shows why there is none with `Mux::mark_exited`. The
  pane keeps its place, size, and focus.
- `Mux::release_focus(id)` moves the focus off a pane to the next live pane.
  A pane whose program exits keeps the focus until the caller releases it.
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
label, rectangle, focus, exit status, and how many rows its view is scrolled
up) and the host size, whenever the panes, the focus, an exit status, a
scrolled view, or the host size changed. `Mux::invalidate_chrome`
asks for a redraw after a change only the caller knows about, such as status
bar text.

### Rendering

Each pane captures the rows that changed since its last frame from a Ghostty
render state. A frame draws those rows at the pane's offset, then the focused
pane's cursor, inside synchronized output (mode 2026). A layout change, host
resize, or removed pane clears the screen and draws every row and the chrome.

A pane draws its terminal's viewport. Scrolling moves the viewport through
Ghostty's scrollback, and new output leaves a scrolled viewport where it is.
Scrolling a pane ends its hold. Selected cells are drawn in reverse video.

The focused pane's input modes are mirrored onto the host: cursor, keypad,
backspace, and Alt-key modes, reverse video, focus events, bracketed paste,
grapheme clustering, and the Kitty keyboard flags. Focus events, the window
title, and mode changes follow the focus. The host always reports mouse buttons
and drags in SGR form (modes 1002 and 1006), and motion without a button (mode
1003) while the focused pane asks for it.

Bells pass through to the host. So do desktop notifications, OSC 9 and OSC
777: `Pane::feed` scans the program's output for them before Ghostty sees
it, keeping the scan's state between reads so a sequence split across reads
is found, and writes each one to the host terminated by BEL. Notifications
over 4096 bytes and sequences cut off by another control character are
dropped.

A program that holds drawing with mode 2026 shows the last frame it
completed, captured when the hold began, including when the screen is drawn
again. Resizing a pane's terminal ends its hold. A hold that lasts
`HOLD_TIMEOUT_MS` (one second) ends as if the program had ended it.

### Input

Host input goes to the focused pane. A bracketed paste, from `ESC [ 200 ~`
through `ESC [ 201 ~`, goes as it is to the pane focused when it began. Outside
a paste, these are routed elsewhere:

- Ctrl-], in its legacy byte or as a Kitty key press, moves the focus to the
  next live pane; Kitty repeats and releases of it are consumed.
- Ctrl-\, in the same two forms, is reported to the caller as
  `Event::Zoom(id)` for the focused pane, once per press, and goes to no pane.
- Shift-PageUp, in legacy or Kitty form, scrolls the focused pane up a page.
- While the focused pane is scrolled, PageUp and PageDown scroll it a page,
  Up and Down a row, and Esc and q return it to the bottom. Any other key
  returns it to the bottom and goes to the program. Kitty key releases are
  consumed.
- Mouse reports go to the pane under the pointer. A button press focuses it, and
  that button's motion and release go to the same pane, clamped to its edges. A
  pane whose program tracks the mouse, and whose view is not scrolled, gets
  every report in its coordinates, encoded by Ghostty's mouse encoder for its
  tracking mode and format. In any other pane, the wheel scrolls three rows, or
  on the alternate screen with alternate scroll mode (1007) sends Up or Down
  three times, and a left-button drag selects text linearly through Ghostty's
  selection, from where the button was pressed to where it was released. A press
  released where it began is a click. On release, the selection's text, with
  soft wraps joined and trailing spaces trimmed, goes to the host's clipboard as
  OSC 52. The selection stays highlighted until the next press or until input is
  typed into its pane.

A key or escape sequence cut off by the end of a read, up to 256 bytes, waits
for the next read. After `INPUT_HOLD_MS` (25 ms) without the rest, what was held
is taken as it is: a lone Esc is the Esc key, and anything else goes to the
focused pane as typed. The runtime wakes for that deadline. A paste's end cut
off by a read waits for the next read without a deadline.

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
- `Event::Zoom(id)` and `Event::Focus(id)`: the zoom key was pressed with
  that pane focused, or the focus moved to that pane. They come in the order
  they happened.
- `Event::Resized`: the host terminal changed size.
- `Event::InputClosed`: the host terminal's input ended.
- `Event::Deadline`: the deadline passed.

`Runtime::restore()` returns the host terminal to its original settings:
every mirrored mode at its default, no Kitty keyboard flags, the default
cursor, the main screen, and the original terminal attributes. Dropping the
runtime, including when `Runtime::new` fails partway, does the same.

Everything runs on one thread. The terminals and their callbacks share state
through `Rc` and `RefCell`.
