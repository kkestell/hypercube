# Keep agents and panel shells running when the app restarts

## Goal

Agents and panel shells keep running when the app process exits for any reason
other than Quit: `make install-service`, a `make dev` rebuild, a crash, or
`kill -9`. The restarted app attaches to them again and applies everything they
reported while it was down, exactly once: transcript items, open attention
requests, work state, conversation IDs, usage, replies, and deliveries. Menu
Quit and SIGINT still end every agent and shell; the sessions stay marked
running and start again at the next launch.

Each agent plugin and each panel shell runs under a holder: a small
`hypercube hold` process that owns the child's input and output and survives
the app. A holder knows nothing about plugins or shells. Its whole interface is
a directory: an input socket, an append-only output log, a lock that shows it
is alive, and an exit record. The app keeps, in the database, how far into each
plugin's output log it has applied, and commits that position in the same
transaction as the effects of the output it applied. After a restart, the app's
state is the database plus the output after that position.

## Related code

- `internal/harness/harness.go` — `Client`: starts the plugin as the app's
  child, owns its pipes, matches replies to waiting requests (`Do`), kills its
  process group, and keeps the end of its stderr. Rebuilt on a holder.
- `internal/harness/protocol.go` — commands, replies, events; `CmdShutdown`.
- `internal/harness/harnesstest/harnesstest.go` — the stand-in plugin, with its
  own serve loop.
- `plugins/internal/plugin/serve.go` — every plugin's serve loop.
- `internal/session/launch.go` — `live` (generation prefix, `starting`,
  `launched`, `quick`, `delivering`, `attention`, `pending`), `launch`,
  `startPlugin`, `end`, `drain`, `note`.
- `internal/session/event.go` — applies events; already writes the database
  only when an event has more than streaming text.
- `internal/session/deliver.go` — the delivery goroutine and its `delivering`
  flag; `internal/session/input.go` — `Input`, `Interrupt`, `Answer`, `request`.
- `internal/session/sessions.go` — `start`, `Stop`, `stopLocked`, `Delete`,
  `Restore`, `Shell`, `stopShells`, `detachLocked`, `stopClients`, `Shutdown`.
- `internal/session/children.go` — `CreateChild`, `EndChild`, `SetStatus`.
- `internal/session/inactivity.go` — reads `starting` and `delivering`.
- `internal/session/runtime.go` — `Runtime` fields, `gen`, `seqs`, timeouts.
- `internal/session/transcript.go`, `feed.go` — overlay of streaming items,
  `putItem`, `mirrorLocked`, `loadSeq`.
- `internal/store/store.go` — schema, `PutEntry` (keeps an item's position on
  update), `LastSeq`, `LastText`.
- `internal/terminal/terminal.go` — shells on a PTY the app owns, an in-memory
  256 KiB buffer, subscriber fan-out. Rebuilt on a holder.
- `internal/ui/terminal.go` — the shell WebSocket.
- `internal/mcp/mcp.go` — `Bridge` and `identity`; the server is stateless, so
  a restarted app answers the bridge's next request.
- `cmd/hypercube/main.go` — subcommands and the exit paths.
- `scripts/dev` — `stop` sends SIGTERM for both rebuilds and Ctrl-C; `clean`
  deletes the dev directory.
- `scripts/install-service` — `launchctl bootout` sends the app SIGTERM.
- `cmd/hypercube/integration_test.go` — `TestAppProcess`,
  `TestStartAppRestoresRunningSessions`; its state directory is under
  `t.TempDir()`.

## Decisions

### Verified platform behavior

Checked on this Mac with a throwaway launchd job and isolated directories:

- `launchctl bootout` and `kill -9` of a launchd job kill the processes in the
  job's process group. Children in their own process group or session survive
  both, with their own children. The holder starts in its own session. A job
  killed with `kill -9` under `KeepAlive` restarts after about ten seconds.
- A shell on a PTY whose master another process (the holder) owns keeps running
  when the holder's parent dies. Another process can resize the PTY by opening
  the terminal device (`/dev/ttysNNN`) with `O_NOCTTY` and setting its size;
  the shell receives `SIGWINCH` with the new size. When the holder dies, the
  shell gets a hangup and ends, so the holder must outlive the app and nothing
  else may own the master.
- An `flock` taken by the app and inherited by the holder as an extra file stays
  held after the app closes its copy, makes a new non-blocking `flock` on the
  same file fail, and is released when the holder is killed with `kill -9`.
- A Unix socket path may be at most 103 bytes; `bind` fails with "invalid
  argument" beyond that. Connecting to a socket whose listening process was
  killed fails at once with "connection refused".
- `go build -o` and `install` replace a binary with a new file; a running copy
  keeps running. Holders and plugins started from `bin/` or `~/.local/bin`
  survive rebuilds and installs.

### The holder

`hypercube hold [-terminal -cols N -rows N] <dir> <command> [args...]` runs one
child and owns its input and output. The app starts it in its own session
(`Setsid`), with standard input and output on `/dev/null`, standard error
inherited from the app, the child's working directory as its own, and the
child's environment as its own. The child gets its own process group: a pipe
child through `Setpgid`, a terminal child through `Setsid` and `Setctty`.

The holder directory:

| Entry | Meaning |
| --- | --- |
| `lock` | Locked with `flock` for the holder's whole life. The app opens and locks it, passes it to the holder as file 3, and closes its copy. The holder marks it close-on-exec so its child does not inherit it. |
| `pid` | The holder's process ID, which the holder writes first. |
| `in` | A Unix socket the holder listens on. Bytes sent on a connection go to the child's input. The app binds and listens, passes the listener to the holder as file 4, and closes its copy without unlinking the socket, so a writer can connect as soon as the holder exists. |
| `out-<position>` | The child's output in order. `position` is the offset of the segment's first byte in the whole output. The holder starts a new segment once the current one reaches 256 KiB. |
| `tty` | The terminal's device path, in terminal mode. |
| `exit` | Written, through a rename, after the child has exited and all of its output is in the segments: `{"code": 1, "status": "exit status 1", "stderr": "..."}`. `stderr` is the end of the child's standard error in line mode, up to 2 KiB. |

The holder's rules:

- **Input.** The holder serves each connection on its own goroutine. In line
  mode it reads whole lines, writes each to the child as one write under a
  lock, and drops a line cut short by the end of its connection, so an app
  killed mid-write leaves no partial command behind and two writers' lines
  never merge. In terminal mode it forwards bytes as they come.
- **Output.** Line mode keeps every segment until the app deletes it. Terminal
  mode deletes all but the newest two segments, so a shell's retained output is
  its scrollback, between 256 and 512 KiB.
- **Standard error.** In line mode the holder copies the child's standard error
  to its own, ignoring write errors, and keeps the end for the exit record. In
  terminal mode standard error is the terminal.
- **Stop.** On SIGTERM the holder ends its child's input: in line mode it closes
  the child's standard input, and in terminal mode it sends SIGHUP to the
  child's group. It kills the group if the child has not exited after 3 seconds.
- **End.** When the child exits, the holder reads its output until end of file
  or for 1 more second, since descendants can hold the pipe open, kills the
  child's group, writes `exit`, and exits. A child that fails to start gets an
  exit record with the error as its status.

`CmdShutdown`, `Client.Shutdown`, and `shutdownWait` go away: closing a plugin's
input already ends it, and the holder owns the 3-second kill.

Holders started by a build keep running that build's holder code, and running
plugins keep their build until the agent's next launch. A new plugin build
applies at each agent's next launch. A change to the holder directory or the
plugin protocol must be loaded by quitting Hypercube first; any other change
loads with `make install-service` while agents keep running.

### The `hold` package

`internal/hold` holds both sides of the directory, so its layout is one fact in
one place:

```go
// Start creates dir and starts a holder in it running argv in cwd with env.
// With size set, the child runs on a terminal of that size.
func Start(dir string, argv []string, env []string, cwd string, size *Size) (*Holder, error)
// Open attaches to the holder in dir, running or ended.
func Open(dir string) (*Holder, error)

func (h *Holder) Write(p []byte) (int, error)        // to the child's input
func (h *Holder) Output(from int64) (*Output, error)  // the output from a position
func (h *Holder) First() int64                         // the oldest retained position
func (h *Holder) Discard(before int64) error           // delete segments entirely before a position
func (h *Holder) Resize(cols, rows int) error          // terminal mode
func (h *Holder) Alive() bool                          // the lock is held
func (h *Holder) Stop() error                          // SIGTERM to a live holder
func (h *Holder) Exit() (Exit, error)                  // the exit record, once ended
func (h *Holder) Remove() error                        // delete the directory of an ended holder

// Read blocks until output is available. It returns io.EOF once the child has
// exited and all output is read, and ErrLost when the holder died without an
// exit record. Close ends a blocked Read with ErrClosed.
func (o *Output) Read(p []byte) (int, error)
func (o *Output) Close() error

// Main runs the holder process for `hypercube hold`.
func Main(args []string) error
```

`Output` reads across segments. At the end of the newest segment it checks, in
order, for a next segment, the exit record, the lock, and whether it was
closed, and otherwise sleeps 25 milliseconds and reads again. `Write` connects
to `in` once and serializes whole writes; the connection fails at once when the
holder is gone. `Resize` opens the `tty` device with `O_NOCTTY`, sets its size
with `pty.Setsize`, and closes it. `Stop` signals the `pid` only while the lock
is held, so a reused process ID is never signaled.

`Start` refuses, naming the path, a directory whose `in` would be longer than
103 bytes. Holder directories sit directly under `store.Dir()/run`: `<launch>`
for an agent's launch and `shell-<session>` for a panel shell. That makes the
longest socket path 56 bytes for the installed app and 72 for `make dev`'s
`~/projects/hypercube-dev/state`. Tests that start holders use a state
directory from `os.MkdirTemp("", "hc")` instead of `t.TempDir()`, about 93
bytes in all, removed in cleanup. `make dev` keeps its own state directory, so
its holders stay apart from the installed app's.

`hypercube` dispatches `hold` to `hold.Main`. In tests, `os.Executable` is the
test binary, so each test package that starts holders has a `TestMain` that
calls `hold.Main` and exits when its arguments start with `hold`.

### Plugin protocol

- `Command.ID` and `Reply.ID` become `int64`. IDs come from the `commands`
  table, only increase, and reach a launch in order, resends included.
- The app may send a command again after it restarts. A plugin ignores any
  command whose ID is not greater than the last ID it read, and so replies to
  each command once. `plugin.Serve` and the stand-in keep that one integer.
- `shutdown` is removed. A plugin ends when its input closes, as it already
  does.
- A reply to an ID the app has not recorded is still a protocol error.

### Durable state

The launch fields become columns of `sessions`, and commands get a table:

```sql
-- added to sessions
  launch TEXT NOT NULL DEFAULT '', position INTEGER NOT NULL DEFAULT 0,
  ready INTEGER NOT NULL DEFAULT 0, quick INTEGER NOT NULL DEFAULT 0,

CREATE TABLE IF NOT EXISTS commands (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  command TEXT NOT NULL,
  line TEXT NOT NULL,
  message INTEGER,
  seq INTEGER,
  item TEXT);
```

- `launch` is set from the moment a launch is decided until its end is
  recorded, and empty when the session has none. It is a new `store.NewID()`;
  it names the holder directory and, with a colon, prefixes the plugin's item
  IDs, replacing the generation counter `gen`. `position` is how much of the
  launch's output the database reflects. `ready` is when the plugin answered
  `start`, in Unix milliseconds, and zero while it starts. `quick` counts the
  quick exits before this launch. `store.Session` gets the four fields, and
  `SaveSession` saves them.
- A **command** row is a command sent to the session's current launch and not
  yet answered. `line` is the exact JSON line. An input that delivers a queued
  message names it in `message`; an input carries the user item to write when
  the plugin accepts it in `item`, with the transcript position `seq` reserved
  before sending so the plugin's reply items follow it. `LastSeq` takes the
  largest of the transcript's and the reserved positions.
- Streaming items are saved with `done` false. `LastText` reads only finished
  items, so the sidebar's reply preview is unchanged. `Transcript` replaces the
  saved copy of an unfinished item with the in-memory one at the same
  position, so a streaming item never shows twice.

### Reading a plugin's output

One reader goroutine per launch reads the plugin's records from the launch's
`position` and applies them in order under `mu`. It owns the launch's
lifecycle: a launch ends only when its reader reaches the end of the holder.

- A record that only extends streaming items changes memory and is published
  to the feed, as today. Its end position is kept in memory.
- Any other record — an item that finishes or opens an attention request, an
  error item, a change of conversation, state, usage, or attention, or any
  reply — is applied in one transaction that also saves the streaming items
  changed since the last commit and the new `position`. After it commits, the
  reader deletes the segments before that position.

So the database always equals the effect of the output before `position`, and
replaying the output after `position` onto it rebuilds what was only in memory.
At attach, the reader loads the session's unfinished items as its streaming
items, and answering a request uses the item ID without the launch prefix as
the native ID (`pendingItem.native` goes away).

### Commands

Every command follows one rule. The app records it, then writes it to the
holder's input. Its reply, read from the output like any record, applies its
effect and deletes the row in one transaction. Whenever the app attaches to a
launch, it sends every recorded command again in ID order before any new one;
the plugin ignores the ones it has read. Recording and sending a command happen
under the launch's send lock, so lines reach the plugin in ID order. A launch's
commands are deleted when its end is recorded.

| Command | Effect of success | Effect of failure |
| --- | --- | --- |
| `start` | set `ready`; mark the session idle; kick deliveries | the launch ends: "Could not start the agent" |
| `input` delivering a message | dequeue the message; write its user item; kick again | note the failure; the message stays queued |
| `input` from the user | write the user item; kick the family | note the failure |
| `interrupt`, `answer` | none | note the failure |

An unanswered command keeps its row until its reply comes or its launch ends.
A plugin that never answers a delivery therefore holds back every later
delivery, and keeps the idle timeout from stopping the family, for the rest of
that launch; that is the price of delivering each message exactly once. Stopping
and starting the family clears it.

Callers that wait for an answer — `Start` and `CreateChild` for `start`, and
`Input`, `Interrupt`, and `Answer` — register a waiter for the command's ID and
wait up to `startTimeout` or `requestTimeout`. A delivery has a waiter of its
own that, after `requestTimeout` without a reply, notes "Delivering a message
got no reply within 30 seconds; later messages wait for it", as a timed-out
delivery is noted today. The reader applies a reply's effect first and then
notifies the waiter, so a caller that gave up, or an app that restarted,
changes nothing about the outcome. `Input` sets `AwaitingUser` false when it
records the command. `harness.Client.Do`, its pending replies, and the
`deliver` goroutine go away.

A delivery is in flight while its command row exists, and the launch is
starting while its `ready` is zero; `live.delivering`, `live.starting`, and the
idle check read these instead of flags. `kickLocked` records the next delivery
when the launch is ready, nothing is in flight, the root does not await the
user, and the session is running and not ended; it sends the line on its own
goroutine so the reader never blocks on the holder's input.

When a reader starts while its launch's `start` is unanswered — a new launch,
or one Restore attaches to or starts — a timer stops the launch if no reply
comes within `startTimeout`, and its end notes "Could not start the agent: the
plugin did not start within a minute".

### Ending a launch

`end` runs when the reader reaches the end of the holder: the exit record's
status and stderr make the reason, as `Client` does today. In one transaction
it saves the streaming items as finished, marks the session exited, clears
`launch`, deletes the launch's commands, and decides what follows:

- When the app is quitting, the session is stopped or ended, or the family is
  being deleted, nothing follows and no note is written.
- After a failed start, the session stays exited with "Could not start the
  agent: …".
- When the holder was lost, no note is written; what follows is decided by
  Restore, below.
- Otherwise it notes "The agent exited: …" and, unless this was the third quick
  exit in a row (`ready` within `quickExit`, counted in `quick`), records the
  next launch and its `start` command in the same transaction.

Then the reader removes the holder directory and, for a relaunch, starts the
next holder and reader.

Stopping no longer detaches a launch first. `Stop`, the idle timeout,
`EndChild`, `Delete`, and `Quit` change the session's marks, then call `Stop` on
each holder and wait for each reader to finish. Records the plugin writes while
it ends are still applied, so a message accepted just before a stop is
dequeued, not delivered again. `detachLocked` and `stopClients` become one
`stopLaunches` that stops and waits.

### Exit paths and startup

- **Quit** (menu Quit and SIGINT): `Runtime.Quit` sets `closing`, stops every
  launch and shell, and waits. No launch remains; sessions keep their running
  mark.
- **Detach** (SIGTERM, which `launchctl bootout` and `scripts/dev` restarts
  send, and a failed HTTP server): `Runtime.Detach` waits for sends in progress
  to finish, closes every reader's output, and returns, leaving holders,
  launches, and commands in place. A crash or `kill -9` leaves the same state
  without the call.
- **Restore** at startup:
  1. Every holder directory that is neither a session's `launch` nor the shell
     of a running session is stopped if its holder is alive and then removed.
  2. Each launch is handled by the state of its holder directory:
     - **missing** — decided but never started: its holder starts now, and it
       gets its recorded commands, starting with its recorded `start` line;
     - **alive** — it gets a reader from its `position` and its recorded
       commands again;
     - **exited** (an exit record) — it is read to the end and ends as above,
       with its note, relaunch, and quick-exit count;
     - **lost** (no live holder and no exit record) — the output it has is
       applied, and it ends without a note.
  3. A running family whose launches were all lost, or that has none — after
     Quit, logout, or a reboot — starts as today, awaiting the user. In any
     other family, a lost launch is relaunched as after an exit, without a
     note.
  4. Each running session's live shell holder is adopted by `Terminals`.

### Panel shells

A shell is a holder in terminal mode at `run/shell-<session>`, started by
`terminal.Set.Open` in the session's directory with `TERM` and `COLORTERM` in
its environment. `Set.Adopt(session, dir)` takes over a live one after a
restart. Each WebSocket reads its own `Output` from `First()`, so the replay is
the retained scrollback and the fan-out, in-memory buffer, and slow-subscriber
dropping go away. Input goes to the holder's `Write`, resizing to `Resize`, and
the `{"exit": n}` message comes from the exit record. A goroutine per shell
waits for its holder to end, then removes the directory and forgets the shell.
`Set.Close` stops every shell for Quit; Detach leaves them.

### MCP bridge

`identity.RoundTrip` retries a request whose connection was refused, every 250
milliseconds for up to 30 seconds, so a tool call made while the app restarts
waits for it. A refused connection never reached the app, so the retry cannot
repeat a tool's effect; any other failure is returned as today.

### Development session

`scripts/dev` restarts the app after a rebuild with SIGTERM, so dev agents keep
running across rebuilds. Ctrl-C, `scripts/dev stop`, and `clean` send SIGINT,
which ends every dev agent and shell, and wait for the app to exit. Before it
deletes the dev directory, `clean` also ends any holder still running under the
dev state directory, as one left by a crashed dev app would be: for each
`run/*/pid` whose process is a `hypercube hold`, it sends SIGTERM and waits for
it to exit.

## Test plan

- `hold`: a line-mode child (`cat`) echoes written lines; output read from a
  saved position after `Open` in a fresh `Holder` continues without gaps or
  repeats; segments rotate at 256 KiB and `Discard` deletes only whole earlier
  segments; the exit record has the status and the end of stderr; `Read`
  returns `io.EOF` after the last output and `ErrLost` after the holder is
  killed with `kill -9`; `Close` ends a blocked `Read`.
- `hold` input: a connection closed mid-line has that line dropped, and the
  next connection's lines arrive whole; lines from two connections never merge;
  a line larger than a pipe buffer (8 MiB) arrives whole; `Write` fails at once
  after the holder is killed; `Start` refuses a directory whose socket path is
  over 103 bytes.
- `hold`: a holder started by a helper process keeps its child running after
  the helper is killed with `kill -9`, and a new `Open` reads and writes it.
- `hold`: `Stop` closes a line child's input and kills a child that ignores it
  after 3 seconds; a terminal child gets SIGHUP; terminal mode keeps only the
  newest two segments; `Resize` makes `stty size` report the new size.
- `plugin.Serve` and the stand-in ignore a command whose ID is not greater than
  the last one read, and reply once to each ID.
- `harness`: a plugin under a holder starts, takes input, and reports; its end
  reports the exit status and last stderr line; a malformed record and a reply
  to an unknown ID still end the connection.
- `session`, restarting a `Runtime` on the same database while holders keep
  running (a new `Runtime`, `Detach` on the old one):
  - events the stand-in writes while no runtime runs appear once in the
    transcript, and work state, conversation, and usage are current;
  - a text item streaming across the restart finishes with all its text, and
    `Transcript` shows it once both before and after a commit;
  - an attention request opened before the restart can be answered after it;
  - a delivery held unanswered (`!hold`) across the restart is dequeued once,
    its user item written once, and the stand-in's log shows the input once;
  - a recorded command whose line never reached the plugin is delivered after
    the restart;
  - a single-agent session whose plugin exits while no runtime runs has the
    output it wrote applied, the exit noted, and is relaunched with its
    conversation; quick exits still count across the restart;
  - a lost launch (holder killed with `kill -9`) in a family with a live
    member has its output applied and is relaunched without a note;
  - a family whose launches were all lost starts awaiting the user;
  - a launch decided but never started is started with its recorded `start`
    line, and its `startTimeout` applies;
  - holder directories that belong to no launch or running shell are removed.
- `session`: a delivery with no reply within `requestTimeout` is noted once and
  holds back later deliveries until the reply comes.
- `session`: Stop, the idle timeout, `EndChild`, `Delete`, and `Quit` leave no
  holder processes and no holder directories; a message accepted during a stop
  is not delivered again at the next start.
- `terminal`: a shell's output and input survive a new `Set` adopting its
  holder; the replay holds output written before the restart; Stop and Close end
  only the shells they should.
- `mcp`: `Bridge` answers a request that it sends before the app listens, once
  the app starts within the retry window.
- `TestAppProcess`: SIGTERM leaves the agents' processes running and the
  restarted app continues their transcripts without a new start and without
  awaiting the user; `kill -9` behaves the same; SIGINT ends every agent and
  shell, and the next app starts the session awaiting the user.
- `TestStartAppRestoresRunningSessions` uses `Quit`.

## Implementation plan

1. Add `internal/hold` (`hold.go` for the app side and directory layout,
   `holder.go` for `Main`, `output.go` for `Output`) with its tests and
   `TestMain`. Dispatch `hypercube hold` in `cmd/hypercube/main.go` and add it
   to the usage text.
2. In `internal/harness/protocol.go`, make IDs `int64`, remove `CmdShutdown`,
   and document the resend rule. Make `plugin.Serve` and the stand-in ignore
   IDs not greater than the last one read; drop the stand-in's `shutdown`
   handling and `!linger`. Update `serve_test.go`.
3. Rebuild `harness.Client` on a holder: `Launch(dir, plugin string)` starts
   `<plugin> serve` under a holder, `Attach(dir)` opens an existing one,
   `Send(cmd)` writes one line, `Read(from, handle)` calls `handle(record, next)`
   for each record with the position after it and returns why the plugin
   ended, and `Stop`, `Discard`, `Remove`, and `Close` pass through. Delete
   `Do`, the pending replies, `Shutdown`, `Kill`, and `tailBuffer`. Update
   `client_test.go` and add `TestMain`.
4. In `internal/store`, add the `launch`, `position`, `ready`, and `quick`
   columns to `sessions` and `store.Session`, the `commands` table and its
   methods (`AddCommand`, `Commands`, `DeleteCommand`), make `LastSeq` include
   reserved positions, and make `LastText` read finished items only. Update
   `store_test.go`.
5. In `internal/session`, add `commands.go` for recording, sending, resending,
   applying replies, and waiters. Rework `launch.go` around the reader (`run`,
   `apply`, `end`) and the slimmed `live`; make `event.go` commit `position`
   with its durable writes and save changed streaming items; reduce
   `deliver.go` to `kickLocked`, the reply effect, and the delivery's
   no-reply note; move `Input`, `Interrupt`, and `Answer` onto recorded
   commands; make `Transcript` replace saved unfinished items with the
   in-memory ones; replace `detachLocked`, `stopClients`, and `Shutdown` with
   `stopLaunches`, `Quit`, and `Detach`; rewrite `Restore` as above; update
   `children.go` and `inactivity.go`. Remove `gen`. Add `TestMain` and
   `restart_test.go`, move the fixture's state directory to
   `os.MkdirTemp("", "hc")`, and update the existing tests.
6. Rebuild `internal/terminal` on holders with `Adopt`, and update
   `internal/ui/terminal.go` to read each connection's `Output`. Update
   `terminal_test.go` and the UI's terminal tests.
7. Add the refused-connection retry to `internal/mcp/mcp.go` with its test.
8. In `cmd/hypercube/main.go`, map menu Quit and SIGINT to `Quit`, and SIGTERM
   and a server failure to `Detach`. Update the integration tests, with a short
   state directory.
9. In `scripts/dev`, split `stop` into a restart that sends SIGTERM and a quit
   that sends SIGINT, used by Ctrl-C, `stop`, and `clean`, and make `clean` end
   remaining holders under the dev state directory before deleting it.
10. Update the documentation below.

## Documentation updates

- `docs/architecture.md` — Ownership: agents and shells run under holders that
  outlive the app; Quit and SIGINT end them, SIGTERM and crashes leave them;
  at startup each launch is attached, finished, or restarted, and a family
  whose agents were all lost starts awaiting the user. Delete the generation
  sentence. Describe the output position and the command rule in a sentence
  each, including that an unanswered delivery holds back later ones. Panel
  shell: the holder keeps its scrollback. Add `hold` to the source table.
- `docs/harness-support.md` — integer IDs, resent commands and the rule that a
  plugin ignores an ID not greater than the last one it read, no `shutdown`,
  the plugin ends when its input closes.
- `docs/installation.md` — Upgrades: agents and shells keep running across
  `make install-service`; a new plugin build applies at each agent's next
  launch; holder-directory or protocol changes need Quit first. Removing the
  service starts with Quit. Development: rebuilds keep dev agents running,
  Ctrl-C and `make dev-stop` end them, and `make dev-clean` also ends holders a
  crashed dev app left.
- `AGENTS.md` — add `hold` to the package list.
