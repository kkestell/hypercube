# Architecture

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
- `Mux::remove_pane(id)` hangs up the program if it is still running, removes
  the pane, moves the focus to the next live pane, and redraws the screen. A
  removed pane's program is still reaped, and gets SIGKILL on its process
  group if it outlives `HANGUP_GRACE_MS` (two seconds).
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
