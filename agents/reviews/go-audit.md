# Go audit

An audit of the Go code in `cmd/`, `internal/` (only `ui.go` and `files.go`
in `internal/ui`), and `plugins/`, including the tests, against `AGENTS.md`
and the hypercube skill at commit b5d0ecc. The code was read whole, not as
diffs. Nothing was changed.

## Summary

The Go code does not need a rewrite. The packages are plain, the plugin
protocol is small and well specified, and most tests check behavior. The bar
is missed in four places:

1. `internal/session` makes three claims its code does not keep. Database
   writes are said to have one owner, the runtime, but they have several. A
   launch failure is handled three different ways. One function releases a
   lock that its caller took.
2. Because writes bypass the runtime, `internal/ui` learns of changes from two
   sources. One of them polls SQLite four times a second.
3. The four harness plugins each carry their own copy of the process
   lifecycle and of attention-request bookkeeping. The OpenCode plugin works
   around its SDK's concurrency with a reader shim.
4. The test fixture is copied by hand into five packages, and one integration
   test is 260 lines of dependent phases.

| Package | Verdict |
| --- | --- |
| `cmd/hypercube` | restructure (tests) |
| `internal/harness` | fine, one small fix |
| `internal/harness/harnesstest` | restructure (small) |
| `internal/menu` | fine |
| `internal/session` | restructure |
| `internal/store` | restructure (small) |
| `internal/report` | restructure (small) |
| `internal/mcp` | restructure (small) |
| `internal/ui` (`ui.go`, `files.go`) | restructure |
| `internal/terminal` | fine |
| `internal/workspace` | restructure (small) |
| `plugins/internal/plugin` | restructure |
| `plugins/claude` | restructure |
| `plugins/codex` | restructure (small) |
| `plugins/opencode` | rewrite the transport, keep the translation |
| `plugins/pi` | restructure (small) |

## Core packages

### cmd/hypercube — restructure (tests)

`main.go` is short and clear. Two small problems:

- Argument checks are split. `main` counts arguments (`main.go:32`) and
  `parseUIArgs` parses them (`main.go:45`).
- The menu and non-menu paths repeat the same signal-or-server-error `select`
  (`main.go:129-152`).

The problem is in the tests. `TestTeams` (`integration_test.go:262-520`) is
one 260-line test with about 25 phases that share state. A failure is
identified only by a comment, and it skips every later phase. Most phases
repeat tests in `internal/session`: titles, dirty-delete protection, messages
held over a restart, restore, worker-failure routing, and crash resume. The
fixture `world` (`integration_test.go:41-133`) is one of the five hand copies
described under Tests below.

Restructure: split `TestTeams` into a few focused tests of what only this
package can check: HTTP and MCP routing, isolation between sessions,
`startApp`, and workspace delete. Delete the phases the session tests already
cover. Keep `TestAppProcess`. `main_test.go` is fine.

### internal/harness — fine, one small fix

`protocol.go` is a clear, documented contract, and `decode` validates it.
`Client` (`harness.go`) has one owner for the process and the connection, and
`run` explains the pipe ownership. `config.go` and `image.go` are plain.

- `Do` handles a failed write by waiting up to one second for the process to
  end (`harness.go:176-183`), and it leaves the request in `pending`. The
  reader already closes `done` when the plugin ends. Return the exit reason
  when `done` is closed, otherwise the write error, with no wait, and delete
  the pending entry.

The tests (`client_test.go`, `harness_test.go`, `image_test.go`) are good.
`TestLateReplyIsKnown` checks for "no exit" with `select { default: }`
immediately after the input (`client_test.go:251-255`). That check can pass
before the failure it guards against arrives. Make another round trip before
checking.

### internal/harness/harnesstest — restructure (small)

The stand-in plugin's keyword list is a good contract, but `serve` is one
135-line closure around a switch. It exits with unexplained codes 90-92
(`harnesstest.go:90,108,169`), and its crash mode sleeps 100 ms (`:123`). Its
log is parsed by three separate copies: `integration_test.go:130-155`,
`client_test.go:154-165`, and `runtime_test.go:158-179`. Restructure: split
`serve` into one function per keyword, name the exit codes, export a log
reader, and host the shared fixture (see Tests).

### internal/menu — fine

A small cgo wrapper with one job.

### internal/session — restructure

This is the package that most needs work. The behavior is right and well
tested, but the following are hard to reason about.

- **The single-writer claim is false.** `Runtime` says it "is the only writer
  of the database" (`runtime.go:43`). Yet `AddWorkspace`, `Rename`, and `Seen`
  write through a bare `*store.DB` (`session.go:23,281,290`), and the MCP tools
  write directly (`mcp.go:273,316`). As a result the runtime's `Changes`
  channel does not cover every change, and `ui` has to poll the database (see
  `internal/ui`).
- **Launch failures have three handlers.** On first start, `start` calls
  `failed`, which records the failure and does not relaunch
  (`session.go:159-161`, `runtime.go:277`). On relaunch, a goroutine calls
  `ended`, which counts quick exits and may relaunch (`runtime.go:365-369`).
  On `AddWorker`, the caller deletes the agent and its worktree
  (`session.go:402-416`). An exit during start goes into `l.exit` for `launch`
  to find later (`runtime.go:306-309,242-244`). A reader has to trace four
  paths to learn what happens when a plugin dies.
- **A lock is handed between functions.** `ended` is called with `mu` held
  and releases it (`runtime.go:315-316,322,363,367-368`).
- **`live` is a state machine written as flags.** `starting`, `exit`,
  `delivering`, `attention`, `client`, and `pending` (`runtime.go:73-91`)
  together encode the launch states, and `mu` guards them as well as five
  `Runtime` maps and fields.
- **The transcript position is held in two places.** `seqs`
  (`runtime.go:62-64`) mirrors `MAX(seq)` in the database. When reading it
  fails, `register` logs, deletes the entry, and continues
  (`runtime.go:131-141`). Numbering then restarts at 1, and those entries
  overwrite earlier ones.
- **`next` returns five values** (`runtime.go:547`). Message attribution is
  split between it, `report.Format`, and `deliver` (`runtime.go:522-525`),
  each of which looks up the sender.
- **The two files are not split by concern.** The package doc says
  `session.go` handles lifecycle and `runtime.go` runs agents, but both hold
  `Runtime` methods for both. `runtime.go` (792 lines) mixes launch, exit,
  events, delivery, user actions, transcripts, and shutdown.
- Smaller problems:
  - `Delete` loops with `if !present { continue }` (`session.go:229-236`).
  - `Delete` ranges over `agents` before checking `err`
    (`session.go:214-220`).
  - `AddWorker` returns a non-zero agent along with an error
    (`session.go:410-416`).

Restructure:

- Make `Runtime` the only writer, with every mutation of workspaces,
  sessions, statuses, and messages behind its methods, and have it signal
  `Changes` on every write.
- Give each launch one exit path: a single function that records the end and
  applies one relaunch policy, called from start, relaunch, and `AddWorker`
  alike, with no lock handoff.
- Split `runtime.go` by concern: launch and exit, event application, message
  delivery, and user actions.
- Move the state transitions now in `report` (`Apply`, `Prompt`, `Failure`)
  into this package.

The tests need the same attention:

- They reach into internals: `f.rt.live` (`runtime_test.go:569,652,699`), a
  direct `rt.event` call (`:585`), and `settled`, which reads `mu`, `live`,
  `starting`, and `delivering` (`inactivity_test.go:11-19`).
- `fixture.created` silently skips the first one or two items of a lead made
  by `f.session()` (`runtime_test.go:30-33,175,188`), so indices such as
  `items[3]` are unreadable.
- Fixed sleeps assert that nothing happened (`runtime_test.go:270,303,654`).
- A nested `if` accepts the same order twice (`runtime_test.go:374-380`).
- Two unrelated fixtures exist side by side: `open` and `seed` in
  `session_test.go`, and `fixture` in `runtime_test.go`.

### internal/store — restructure (small)

The SQL layer is plain and the tests are good.

- `ShownWorkspace`, `ShownSession`, `ShownAgent`, and `Snapshot`
  (`store.go:461-534`) describe the browser view, and they sort for it. They
  belong in `ui`.
- `Usage` (`store.go:78`) duplicates `harness.Usage` (`protocol.go:108`), and
  `report.Apply` converts one to the other (`report.go:30`).
- `Open` deletes files left by earlier builds on every start
  (`store.go:166-168`). That is transition code with no current requirement.
- `session_activity` is a separate table for one timestamp
  (`store.go:131-133,285-289`). A `last_active` column that `SaveSession`
  leaves out would do the same.
- Not-found errors are inconsistent. `Session` wraps `sql.ErrNoRows`
  (`:303`), while `Workspace`, `Agent`, and `Find` do not (`:258,350,379`).
  `ui.fileError` relies on the wrapped form (`files.go:46`), so it returns 404
  for a missing session and 409 for a missing workspace. Return one exported
  `ErrNotFound`.
- `Watch` (`store.go:206-232`) polls `PRAGMA data_version` only because
  writes bypass the runtime. It goes once the runtime is the only writer.

### internal/report — restructure (small)

The package mixes two concerns. Text for agents (`Instructions`, `Format`,
`Title`, `failure`) belongs here. State changes to stored records (`Apply`,
`Prompt`, `Failure`, `report.go:16-49`) do not.

- `Prompt` is a function that sets one field (`report.go:35-37`).
- `Failure` quietly does nothing for a lead (`:41-43`).
- `Format` takes a slice but is only ever called with one message
  (`runtime.go:561`).
- `Leads` returns two unrelated results, the other leads and a title map for
  every session (`report.go:87`).

Restructure: move `Apply`, `Prompt`, and `Failure` into `session`. Make
`Format` format one message and return its sender's address, so `deliver`
stops looking it up again. `report_test.go` is fine;
`TestPromptReleasesQueuedMessages` goes with `Prompt`.

### internal/mcp — restructure (small)

- The tools write the database directly (`mcp.go:273,316`), which is part of
  the single-writer problem. `sendMessage` and `setStatus` should be
  `Runtime` methods.
- The name check sits in the tool (`mcp.go:333-339`), but `store.Address`
  relies on names without slashes (`store.go:91-92`). Enforce it in
  `AddWorker`.
- `removeAgent` repeats an empty-name check that `RemoveWorker` already
  fails on (`mcp.go:359`).
- `newServer` loads the configuration and workspace on every request and
  discards their errors (`mcp.go:69-70`). The comment explains why, but the
  tool then loads the configuration a second time (`mcp.go:340`).
- `if len(profiles) > 0` wraps a `range` that needs no guard (`mcp.go:84`).
- The bridge sets an `Origin` header (`mcp.go:174-183`) only to pass the
  check that `ui.localOnly` applies for browser pages (`ui.go:313`). A request
  from an agent is not a browser request. Exempt `/mcp` from the origin check
  instead, since it already requires a known agent ID (`mcp.go:197`).
- Tests call unexported tool functions directly (`mcp_test.go:270-349`), even
  though the file has `connect` and `callTool` helpers over the real
  transport. Drive them through `callTool`.

### internal/ui (ui.go, files.go) — restructure

- **There are two change feeds.** `Handler` starts a goroutine that polls the
  database every 250 ms (`ui.go:64-68`) and another that waits on the
  runtime's `Changes` (`ui.go:69-80`). Both wake the same sockets. Once the
  runtime owns every write, one feed is enough.
- **Every wake re-reads and re-encodes everything.** Each change rebuilds the
  full snapshot (`ui.go:132-146`). For every agent in the selected session it
  also reloads, re-marshals, and byte-compares the whole transcript
  (`ui.go:167-196`). During streaming that happens on every 50 ms coalesce.
  The runtime already knows which items changed. Have it publish them (agent,
  item, sequence number) and send those.
- **Writes bypass the runtime.** Handlers call `session.AddWorkspace`,
  `session.Rename`, and `session.Seen` with the raw database
  (`ui.go:337,403,405`). The handler also reads the database to find the new
  lead (`ui.go:388-391`), which `Create` could return.
- **A dev-only hack ships in the binary.** `devReload` injects a polling
  script into `index.html` by string replacement (`ui.go:99-121`). It belongs
  in the dev script or the page's own module. The browser rewrite should
  decide where.
- The size limits for a terminal resize are written twice (`ui.go:449-455`
  and `:486`).
- `readFile` maps an empty path to `.` and then refuses `.` as a folder
  (`files.go:100-110`). That is a special case without a purpose.
- `fileError` depends on the inconsistent store errors (see `internal/store`).

The Go tests are mostly fine. `TestDevReload` asserts JavaScript text in the
served page (`ui_test.go:137`). The POST helper is written three times
(`ui_test.go:247,352,613`).

Sequence this with the browser rewrite: the data socket's messages are the
interface between them.

### internal/terminal — fine

One owner per shell, a bounded buffer, slow subscribers dropped, and process
groups killed. The tests use real shells with bounded waits.

### internal/workspace — restructure (small)

- `AddWorktree` edits `.git/info/exclude` inline (`workspace.go:86-112`) and
  ignores the `ReadFile` error. Move this into a function of its own.
- Cleanup after a failure discards git errors (`:116,121-122`).
- `setUp` and `copyIncluded` run git themselves (`:137-142,157-166`) because
  `git()` cannot take an environment or stdin. Give `git()` that ability.
- `copyIncluded` uses a type assertion where `errors.As` belongs (`:162`).
- `Dirty` and `store.Path` are exported but used only inside their packages.
- `session.AddWorkspace` calls `workspace.At("/", dir)` (`session.go:24`).
  The `base` argument exists for this one call.

In the tests, `TestWorktreeRemoval` has its own copy of the git helper
(`workspace_test.go:54-61`) that `gitRepo` already provides.

## Harness plugins

### plugins/internal/plugin — restructure

The shared package holds small helpers rather than the code all four plugins
actually share.

- The process lifecycle is copied four times: `LookPath`, pipes,
  `WaitDelay`, a stderr tail, a `died` goroutine that builds the exit reason,
  and stopping by closing stdin and then killing after 3 s. See
  `claude/main.go:86-182`, `codex/main.go:90-139`, `opencode/main.go:110-165`,
  and `pi/main.go:119-188`.
- `idle()` is copied four times (`claude/main.go:185`, `codex/main.go:251`,
  `opencode/main.go:362`, `pi/main.go:237`). So is the `errors++`
  error-item counter (`claude/main.go:669`, `codex/events.go:277`,
  `opencode/main.go:464`, `pi/main.go:507`), and so are `main` and the `serve`
  wrapper.
- Explaining a dead connection with the stderr tail is written three times
  (`codex/main.go:228-248`, `opencode/main.go:328-350`, and
  `pi/main.go:293-300`, which waits a fixed second).
- `plugin.go` collects unrelated helpers. `WriteFile`'s temp name `index-*`
  (`plugin.go:44`) comes from one caller. `JSONString` panics where
  `json.Marshal` would do.
- `Serve` has no tests of its own. Near-identical harnesses in
  `claude/main_test.go:504-641` and `pi/main_test.go:569-720` test it
  indirectly.

Restructure: add a `Process` type that owns start, the exit reason, and
stop. Add a shared error-item counter. Have `Serve` emit the idle event. Test
`Serve` here and remove the per-plugin copies.

### plugins/claude — restructure

- `answer` (`main.go:258-321`) handles permission requests and questions in
  one function. It branches on `r.questions == nil` twice, and its item search
  silently falls back to index 0 (`:297-302`).
- `control` marshals `Input` to JSON and back, discarding both errors
  (`:602-603`), because the input was decoded as `map[string]any`. Decode it
  once as `json.RawMessage`.
- One request is stored under every item ID, and `closeRequests` rescans
  `r.items` (`:328-343`). Codex and OpenCode have the same structure; a
  shared multi-item request type would serve all three.
- Startup state is spread over `ready`, `initError`, `resultText`, and
  `failure()` (`:63-65,191-198,653-659`), across two goroutines.
- The conversation update runs at the top of `line()` (`:420-423`), before
  `ready`. A failed resume can therefore emit a conversation before start
  replies.
- `blocks` and `tools` are never pruned (`:68`).
- Tests assert the private `h.conversation` field
  (`main_test.go:161,467`).

### plugins/codex — restructure (small)

- `request` keeps `parse`, `reply`, and `refuse` closures plus untyped
  `values` (`requests.go:62,80-88`). Use plain response functions for each
  kind.
- Steering failure is detected by matching the error text `"no active turn"`
  (`main.go:302`).
- `generatedImage` completes an item that has no kind (`events.go:359,362`)
  only to have `complete` hide it.
- `finished` grows for the life of the process (`main.go:68`).
- `parseForm` (`requests.go:364-384`) duplicates OpenCode's `fieldOrder`
  (`opencode/main.go:806-832`). `optionNotes` is repeated in Claude and
  OpenCode. Move both to the shared package.
- The tests are behavioral and good.

### plugins/opencode — rewrite the transport, keep the translation

- `inOrder` (`main.go:183-223`) and the `taken` channel parse every JSON-RPC
  envelope. They guess whether the ACP SDK will dispatch it, then block the
  SDK's reader until the handler takes it. This exists only because the SDK
  runs each request on its own goroutine. `plugin.RPC` already gives in-order
  handling and later replies, and Codex uses it.
- Forms are answered by blocking the SDK's handler goroutine on `r.reply`
  (`:95,512-518`) instead of replying later by ID.
- The SDK types lack fields, so the code re-decodes params or round-trips
  maps through JSON to read them (`:553-565,621-628,740-750`).
- `parent()` attaches a subagent by matching the child session's title to a
  running tool's description (`:680-698`). That is a heuristic.
- `permission()` discards an unmarshal error (`:704`).
- `sortedKeys` (`:801-808`) is `slices.Sorted(maps.Keys(m))`.
- Tests set `h.busy` and call `h.ended` directly, and they sleep 20 ms
  (`main_test.go:537-538,801,809,822`). `TestDroppedMessageDoesNotBlock`
  (`:563`) tests the workaround. The stand-in server
  (`main_test.go:39-150`) nearly copies Codex's (`codex/main_test.go:36-158`).

Rewrite: speak ACP over `plugin.RPC` with small local structs for the fields
the plugin reads, and answer forms by request ID as Codex does. That deletes
`inOrder`, `taken`, the reply channels, and the JSON round trips. `update`,
`chunk`, `toolCall`, and `form` can stay largely as they are.

### plugins/pi — restructure (small)

- Pi keeps its own call table (`main.go:68-71,277-318,476-504`). That is
  justified, because Pi's protocol is not JSON-RPC. But its write-failure path
  waits a fixed second for the process to die (`:295-300`).
- `connect` writes `window`, `session`, `cost`, `announced`, and `context`
  without `mu` (`:213,229-232`), while the reader goroutine reads them. This
  breaks the locking rule the struct states (`:73`).
- `input` and `interrupt` use `context.Background()` (`:323,342`), so stop
  does not cancel a pending call.
- `pi.js` is fine.

## Tests across packages

- **The fixture is copied by hand five times.** Each copy sets up an isolated
  state directory, XDG config, `config.json`, a git repo, the stand-in plugin,
  a logs directory, and database read helpers: `integration_test.go:41-133`,
  `runtime_test.go:35-166`, `ui_test.go:306-350`, `mcp_test.go:23-57`, and
  `report_test.go:15-69`. The git helper is copied five times and `open()`
  four times. Move one fixture into `harnesstest` and keep thin wrappers.
- **Setup errors are dropped.** For example: `session_test.go:28-29,46,77,88`,
  `mcp_test.go:28-29,41-43,293-299`, `ui_test.go:189-190,240-242`,
  `workspace_test.go:19,31,33`, and `store_test.go:102-103,126`. A broken
  setup then shows up as an unrelated failure.
- **Fixed sleeps assert that nothing happened**
  (`integration_test.go:439,449,472`, `runtime_test.go:270,303,654`).
  Trigger a delivery attempt and then check that the queue is unchanged.

## Recommendation and order of work

Restructure; do not rewrite. The Go code's problems are local and named
above. The browser rewrite does not have to wait for them, apart from the
data socket.

1. **One writer, one change feed.** Route every write through `Runtime`:
   workspaces, rename, seen, status, and messages. Signal `Changes` on every
   write, delete `store.Watch` and the polling goroutine in `ui`, move
   `Shown*` and `Snapshot` into `ui`, and move `Apply`, `Prompt`, and
   `Failure` from `report` into `session`. Agree with the browser rewrite on
   a data socket that sends changed items instead of re-diffing whole
   transcripts.
2. **Session runtime.** Give each launch one exit path with one relaunch
   policy and no lock handoff. Split `runtime.go` by concern. Remove `seqs`'
   silent fallback. Rewrite the session tests to use public behavior only.
3. **Shared test fixture.** Put it in `harnesstest` together with the log
   reader, check setup errors, split `TestTeams`, and drive the MCP tests
   through the transport. Doing this before or alongside step 2 makes step 2
   safer.
4. **Plugin process lifecycle.** Add a shared `Process`, idle event, and
   error counter, and test `Serve`. Then add the shared multi-item request
   type and form helpers.
5. **OpenCode transport onto `plugin.RPC`.**
6. **Small fixes:**
   - `harness.Do`'s one-second wait
   - the store's `ErrNotFound`, the leftover-file cleanup, and
     `session_activity`
   - `workspace`'s git helper and exclude-file function
   - the `/mcp` origin exemption
   - Claude's `answer` split
   - Codex's closures
   - Pi's locking
