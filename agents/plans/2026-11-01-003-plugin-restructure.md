# Plugin restructure

## Goal

The four harness plugins each copy the process lifecycle, the idle event, and
the error-item counter, and each has local workarounds: OpenCode's reader shim
around its ACP SDK, Claude's map round trips and split startup state, Codex's
closures and error-text matching, Pi's unlocked writes and fixed waits. When
this work is done, `plugins/internal/plugin` owns what the plugins share, each
plugin holds only its harness's translation, and every harness behaves as it
does today. `internal/harness.Client.Do` stops waiting after a failed write.

Sources: `agents/reviews/go-audit.md`, sections "plugins/internal/plugin",
"plugins/claude", "plugins/codex", "plugins/opencode", "plugins/pi", and the
`Do` item under "internal/harness".

## Least costly complete design

The baseline is to keep every plugin's structure and add only what removes a
copy or a workaround the requirements name:

- One `plugin.Process` replaces four copies of start, exit reason, and stop. It
  also replaces the three copies of the line-reading loop (Claude's, Pi's, and
  `RPC.read`), so one goroutine owns reading the output and knows when the last
  line has been translated.
- `plugin.Serve` emits the idle event and waits on the process itself, which
  removes four `idle` methods and the `exited` channel.
- `plugin.Errors` replaces four error counters.
- The form and option-note helpers move from Codex and OpenCode into the shared
  package. They are existing code, not new code.
- OpenCode reuses `plugin.RPC`, as Codex does. It adds no transport of its own.

Explaining a dead connection needs no helper of its own. When the output ends,
the process's exit reason is handed to whoever reads its lines, so
`plugin.RPC`'s pending calls and Pi's call table report that reason as it is.
No other types or responsibilities are added.

## Related code

- `plugins/internal/plugin/serve.go` — `Serve`, `Agent`, `Start`, `Tail`.
- `plugins/internal/plugin/rpc.go` — JSON-RPC over newline-delimited JSON. Codex uses it, and OpenCode will.
- `plugins/internal/plugin/plugin.go` — `MCPTool`, `JSONString`, `WriteFile`.
- `plugins/claude/main.go` — process and read loop at `:86-182`, `answer` at `:258-321`, `control` at `:577-641`, startup state at `:63-65,191-198,420-423,653-659`.
- `plugins/codex/main.go`, `requests.go`, `events.go` — process at `main.go:90-139`, `call` at `:228-248`, steering at `:285-324`, requests in `requests.go`, `fail` at `events.go:276`.
- `plugins/opencode/main.go` — the ACP SDK connection and `inOrder` at `:176-223`, `call` at `:328-350`, forms at `:715-878`.
- `plugins/pi/main.go` — process at `:119-188`, `connect` at `:193-234`, `call` at `:277-318`, read loop at `:414-425`.
- `internal/harness/harness.go:153-202` — `Client.Do`.
- Tests: `plugins/*/main_test.go`, `plugins/internal/plugin/rpc_test.go`, `internal/harness/client_test.go`.

## Decisions

### `plugin.Process` and `ReadLines`

```go
// ReadLines calls handle for each non-empty line of r, including a last line
// without a newline, and returns the error that ended r.
func ReadLines(r io.Reader, handle func(line []byte)) error

// Process is a running harness CLI. It owns the CLI's pipes, reading its
// output, the reason it ended, and stopping it.
type Process struct {
	Stdin io.WriteCloser
	...
}

func Start(product string, cmd *exec.Cmd) (*Process, error)
// Read calls handle for each line of the output until the output ends, then
// returns the exit reason. Done closes after the last handle call returns.
// Call it once.
func (p *Process) Read(handle func(line []byte)) error
func (p *Process) Done() <-chan struct{}
func (p *Process) Err() error // valid once Done is closed
func (p *Process) Stop()
```

- `ReadLines` is today's loop: a `bufio.Reader` with a 1 MiB buffer and
  `ReadBytes('\n')`. It hands a line returned together with the error to
  `handle` before returning. It is exported because the test stand-ins drive
  `RPC` from an `io.Pipe` with it.
- `Start` reports a missing CLI from `cmd.Err` (set by `exec.Command`) as
  `"<product> is missing: install <name>"`, with `name` taken from `cmd.Args[0]`.
  The products are "Claude Code", "Codex", "OpenCode", and "Pi". `Start`
  reports a failed start as `"start <name>: <err>"`. It sets `WaitDelay` to one
  second and sends stderr to `os.Stderr` and a tail of the last 2 KiB. `Tail`
  becomes an unexported type in `process.go`.
- Stdout is a pipe the process owns (Claude's current approach), not
  `cmd.StdoutPipe`. `Wait` closes a `StdoutPipe` as soon as the process exits,
  which can drop output not yet read. Claude depends on reading that output: a
  failed resume prints its reason as a result line and exits.
- When the process exits, a watcher goroutine records the reason
  (`"<name> exited[: <wait error>][: <stderr tail>]"`, the format all four use
  today) and sets a one-second read deadline on the pipe. Children that keep the
  pipe open then cannot hold the reader forever.
- `Read` runs `ReadLines` on the pipe. After the output ends, it waits for the
  exit, closes the pipe, closes `Done`, and returns the reason. `Read` is the
  one owner of "every line has been translated": `Done` cannot close before the
  handler for the last line, terminated or not, has returned. Each plugin calls
  `go p.Read(handle)` once, right after `Start`.
- `Stop` closes stdin and kills the process if `Done` has not closed after
  three seconds. Writers keep their own write locks. Closing an `*os.File`
  during a write is safe, so `Stop` takes no lock.

### `plugin.RPC` takes lines

```go
func NewRPC(w io.Writer, handle func(Message)) *RPC
// Line handles one line of input: a response for a call, or a request or
// notification for handle.
func (c *RPC) Line(line []byte)
// End ends the connection: pending and later calls fail with err. io.EOF
// becomes "connection closed".
func (c *RPC) End(err error)
```

`RPC.read` is deleted; `line` becomes the exported `Line`, and the end of
`read` becomes `End`. A plugin connects with
`rpc := plugin.NewRPC(p.Stdin, ag.handle)` and
`go func() { rpc.End(p.Read(rpc.Line)) }()`. A pending `Call` therefore fails
with the exit reason. This deletes Codex's and OpenCode's `call` explanation
code and its three-second wait. Tests connect with
`go func() { rpc.End(plugin.ReadLines(pipe, rpc.Line)) }()`.

### `Serve` and `Agent`

```go
type Agent struct {
	Process      *Process // Serve returns Process.Err() when it ends by itself
	Conversation string   // reported with the idle event that follows start's reply
	Input        func(text string, images []harness.Image) error
	Interrupt    func() error
	Answer       func(request, choice, text string) error
	Stop         func()
}

type Start func(a harness.StartArgs, emit func(harness.Event)) (Agent, error)
```

`Serve` writes `Event{Conversation: agent.Conversation, State: Idle}` after a
successful start reply. In its loop it selects on `agent.Process.Done()`
instead of an `exited` channel; before start, that channel is nil. Because
`Done` closes only after the last line's handler returns, and handlers emit
synchronously, the last event is written before `Serve` returns the exit
reason.

Each plugin's `Conversation` is what its `idle` reports today:

- Claude: `a.Conversation`.
- Codex: the thread.
- OpenCode: the session.
- Pi: the session when resumed, otherwise empty.

`Stop` stays a function because Codex and OpenCode also cancel their context.

### `plugin.Errors`

```go
// Errors numbers a harness's error items. The owner's lock guards it.
type Errors struct{ n int }

// Item returns the next finished error item, with IDs error-1, error-2, ...
func (e *Errors) Item(text string) harness.Item
```

### Shared forms (`plugins/internal/plugin/form.go`)

- `Field`, the union of Codex's `field` and OpenCode's `formField`: `Type`,
  `Title`, `Description`, `Enum`, `OneOf []Option`, and `Items{Enum, AnyOf}`.
- `Option`: `Const`, `Title`, `Description`.
- `(Field) Options() []Option`: Codex's `choices` order, `Items.AnyOf` then
  `Items.Enum` for an array and `OneOf` then `Enum` otherwise.
- `ParseForm(properties json.RawMessage) (keys []string, fields map[string]Field, err error)`
  is Codex's `parseForm`, plus OpenCode's check that no key repeats.
- `OptionNotes(text string, labels, notes []string) string` is Codex's
  `optionNotes`. Codex passes option titles, and OpenCode and Claude pass
  descriptions, as each does today.

### Claude

- **Startup in one place.** `startClaude` gives `p.Read` a handler that owns
  the startup state in local variables, `started` and `failure`, and reports
  through a buffered channel `ready` that `startClaude` waits on, with the
  existing 60 s timeout. Until claude answers `initialize`, the handler does
  the following:
  - The initialize response sets `started`. It sends either the initialize
    error or the kept `failure`, and otherwise nil.
  - A `result` line is a failed start. Its text, formatted as `result` formats
    it, becomes `failure`. The line does not go to `line`.
  - Other lines go to `line`.

  After `started`, every line goes to `line`. When `p.Read` returns without
  `started`, the goroutine sends `failure` if it is set, and otherwise the exit
  reason. This deletes `ready`, `initError`, `resultText`, `failure()`, and the
  read loop. `line` never sees a pre-ready result, so the conversation update
  moves into `result`, and a failed resume can no longer emit a conversation.
- **Input decoded once.** `control` decodes `Input` as `json.RawMessage`. A
  permission request keeps the raw input as `request.input` and sends it back
  as `updatedInput`. A question request decodes the questions from the same
  raw input, and a decode error denies the request, as an empty question list
  does today. Once every question has an answer, `withAnswers(input, answers)
  (json.RawMessage, error)` decodes the input into `map[string]json.RawMessage`,
  sets `answers`, and marshals it. `answer` returns its error.
- **Answers by kind.** `answer` looks up the request and calls
  `answerPermission(r, item, choice, text)` or `answerQuestion(r, item, choice,
  text)`, chosen by `r.questions != nil`. Each handles only its own kind.
  `answerQuestion` finds the item's index with `slices.IndexFunc`. The item is
  always in its request, so a missing one panics instead of falling back to
  item 0. Both finish through one `finish(r, index, result)`, which emits the
  done item and deletes the entry.

### Codex

- **Requests as plain data.** `request` holds `id`, `method`, `items`,
  `answers []string` (by item), `given`, and the data its response needs:
  - `decline bool`: an approval that Codex lets the user decline.
  - `permissions json.RawMessage`.
  - `questionIDs []string`.
  - `form []formField`, each a key and a `plugin.Field` by item. It is nil for a
    message-only elicitation.

  The builders (`commandRequest`, `fileRequest`, `permissionsRequest`,
  `questionsRequest`, and `elicitationRequest`) fill these fields. Two methods
  switch on `method` and call plain functions:
  - `response() (any, error)` calls `approvalResponse(choice, decline)`,
    `permissionsResponse(choice, permissions)`,
    `questionsResponse(questionIDs, answers)`, or
    `elicitationResponse(form, answers)`.
  - `refusal() any` returns the refusal each kind sends today.

  `formValue(f plugin.Field, answer string) (any, error)` converts a form
  answer to a boolean, number, or one-item array. `answer` calls it when a
  form field is answered, so a bad number is refused at once, as today.
  `elicitationResponse` calls it again to build the content. This deletes
  `parse`, `values`, `reply`, `refuse`, and `approval`.
- **Steering failure from the turn state.** The real CLI (codex-cli 0.166.0)
  answers a steer with no active turn with
  `{"code":-32600,"message":"no active turn to steer"}`. -32600 is the generic
  invalid-request code, which other refused steers also use, so the code cannot
  tell the cases apart.

  `thread/read {threadId}` returns `thread.status.type`. Its values are
  `notLoaded`, `idle`, `systemError`, and `active`. The real CLI reports `idle`
  for a thread with no turn running.

  When a steer fails and `ag.turn` still names the steered turn, `input` reads
  the thread. It starts a new turn only when the status is `"idle"`. For any
  other status, or when `thread/read` fails, it returns the steer's error. The
  steer error text is no longer read.
- `input` checks `ag.rpc.Done()` where it checks `ag.done` today.

### OpenCode

- Speak ACP over `plugin.RPC`, connected as in "`plugin.RPC` takes lines".
  `handle` runs on the reading goroutine and does not wait:
  - `session/update` goes to `update`.
  - `session/request_permission` is answered at once. A params decode error
    becomes `RespondError(-32602, err.Error())`, which removes the discarded
    error.
  - `elicitation/create` goes to `form`, which shows the questions and keeps
    the request's ID.
  - Any other request gets `RespondError(-32601, …)`. Other notifications are
    ignored.
- `answer` and `closeRequests` reply with `rpc.Respond(r.id, …)`: `{"action":
  "accept","content":…}` or `{"action":"cancel"}`. An unshowable or empty form
  is answered at once from `form`. This deletes `inOrder`, `taken`,
  `request.reply`, and the blocked handler goroutines.
- `call(ctx, method, params, result any) error` mirrors Codex's `call`. The
  prompt goroutine and `ended` stay. `input` checks `ag.rpc.Done()` where it
  checks `ag.done` today.
- Local structs cover only the fields the plugin reads or writes:
  - initialize result: `protocolVersion`, and
    `agentCapabilities.sessionCapabilities.resume`.
  - session result: `sessionId` and `configOptions`. A config option has `id`,
    `type`, and `options`, where each entry has a `value` or a group's
    `options`.
  - session update: `sessionId` and `update`. The update has `sessionUpdate`,
    `messageId`, `content` (raw, because a chunk has one block and a tool call
    has a list), `toolCallId`, `name`, `status`, `rawInput`, `used`, `size`,
    `cost.amount`, and `_meta["opencode/child-session"]`.
  - prompt: content blocks.
  - permission: `options[].kind` and `optionId`.
  - elicitation: `mode` and `requestedSchema.properties`, read with
    `plugin.ParseForm`.

  Decoding `_meta` directly deletes `child`'s marshal round trip, and the update
  struct deletes `toolCall`'s second decode of the params.
- The wire output stays the same. Before deleting the SDK, record what the
  current tests' stand-in receives for `initialize`, `session/new`,
  `session/resume`, `session/set_config_option`, `session/prompt`,
  `session/cancel`, and each response. Write the same fields, leaving out only
  the SDK's protocol defaults: `fs` false, `terminal` false, and `auth`
  terminal false. These mean the same as absent fields.
- `sortedKeys` becomes `slices.Sorted(maps.Keys(m))`.
- Drop `github.com/coder/acp-go-sdk` with `go mod tidy`. Nothing else imports
  it.

### Pi

- `connect` keeps what it reads in locals and assigns `window`, `session`,
  `cost`, `announced`, and `context` under `mu` at the end.
- A failed write in `call` returns the write error (`"pi is not running: …"`)
  at once. Serve reports the exit reason when the process ends.
- The read loop is deleted. Pi connects with
  `go func() { ag.end(p.Read(ag.line)) }()`. `end(err)` records the reason and
  closes an `ended` channel. `call` selects on `ended` instead of `done` and
  returns `"<command>: <reason>"`. The agent loses `cmd`, `done`, and `reason`.

### `internal/harness.Client.Do`

On a failed write, delete `pending[cmd.ID]`. Return `c.exitErr()` if `done` is
closed, and otherwise the write error, without waiting.

### Left as is

`JSONString` keeps its panic: none of the moved code needs a change to it. Out
of scope: Claude's shared multi-item request type, the pruning of `blocks` and
`tools`, Codex's `generatedImage` and `finished`, OpenCode's `parent()`, and
Pi's `context.Background()`.

## Test plan

### `plugins/internal/plugin`

New `process_test.go` (real `sh`):

- A missing CLI names the product and the command.
- `sh -c 'echo out; echo oops >&2; exit 3'`: `Read` hands `out` to the handler,
  then returns `sh exited: exit status 3: oops`. `Done` closes and `Err`
  matches.
- `sh -c 'printf "{\"last\":1}"; exit 3'`: the handler receives the final line
  without a newline, and `Done` is still open while that handler runs. The test
  blocks in the handler and checks this.
- Children that keep stdout open (`sh -c 'sleep 3 & exit 0'`): `Read` returns
  within about one second and `Done` closes.
- `Stop` on `cat` ends it by closing stdin. `Stop` on a process that ignores
  stdin (`sleep 30`) kills it after three seconds.
- An `RPC` connected to a `Process` whose CLI exits fails a pending `Call` with
  the exit reason. This replaces Codex's `TestErrorsExplainADeadConnection`.

`rpc_test.go` keeps its cases, connecting through `ReadLines` and `End`. It
adds a case where a final response without a newline still reaches its `Call`.

New `serve_test.go`. A stand-in `Start` returns an `Agent` around a real
process and records its calls:

- A start reply comes first, then the idle event with `Agent.Conversation`.
- Commands before start reply `not started`. A second start replies
  `already started`, and so does a start without arguments.
- An unknown command is an error.
- `input`, `interrupt`, and `answer` reach the agent.
- A malformed command makes `Serve` return `malformed command`.
- `shutdown` replies, returns nil, and stops the agent.
- Closing the input returns nil and stops the agent.
- A process that ends by itself makes `Serve` return `Process.Err()`.
- **The last event precedes the exit.** The stand-in's process runs
  `printf '{"text":"last"}'; exit 3`, and its handler emits an event for each
  line. When `Serve` returns the exit reason, its output already holds the
  `last` event.

`form_test.go`:

- `ParseForm` keeps the schema's order and rejects a repeated key or
  properties that are not an object.
- `Options` and `OptionNotes` keep the outputs the moved tests check today.

### Per plugin

The tests check the same behavior as today, minus what the shared tests cover.

- **Claude:**
  - Delete `connection`, `connect`, `send`, `until`, `replyTo`, and the
    `TestServe*` tests (`main_test.go:504-696`).
  - `TestStart` calls `startClaude` with the fake `claude`. It checks the
    arguments as `TestServe` does, sends input, and checks the text through
    `emit`.
  - `TestStartResumes` checks for `--resume`.
  - `TestStartFailedResume` uses `badresume.jsonl` and a variant whose
    `session_id` differs from the requested one. Both return the "No
    conversation found" error, and neither emits an event.
  - `TestExitEndsTheProcess` uses `FAKE_EXIT`. `Process.Err()` contains
    `exit status 3` and `boom`.
  - `newClaude` drops `ready`. The question and permission tests stay as they
    are.
  - Add `TestQuestionInputIsKept`: an `updatedInput` keeps unknown input fields
    beside `answers`.
- **Codex:**
  - `TestSteerRejectedAfterTheTurnEnded` gets a `thread/read` handler that
    returns `idle`, and checks that a new turn starts.
  - `TestSteerFailureOnALiveTurnIsReturned` returns `active`.
  - A table case covers each other outcome: `notLoaded`, `systemError`, an
    unknown status, and a failed `thread/read`. Each returns the steer's error
    and starts no turn.
  - A case where the steer fails with "no active turn to steer" but the thread
    is `active` returns the error. This shows the text is no longer read.
  - `TestInputWhenNotRunning` closes the stand-in's output and waits for
    `rpc.Done()`.
  - Delete `TestErrorsExplainADeadConnection`.
  - The request tests stay as they are and check the same responses.
- **OpenCode:**
  - The stand-in never answers `session/prompt` by itself. It keeps the open
    prompts' IDs, and `h.endTurn(t, stopReason)` answers the oldest. `feed`
    answers a fixture's response line with the open prompt's ID.
  - Tests that play a turn call `h.input` first, so the plugin's own state
    makes the turn busy. Remove every `h.busy` assignment and direct
    `h.ended` call.
  - `TestInputDuringATurnWaits` and `TestInterruptCancelsAForm` replace their
    20 ms sleeps with `h.sync` round trips.
  - The "stopping" case of `TestPromptFailures` starts a turn, cancels the
    agent's context, and checks that the turn ends idle with no error item.
  - `TestEndOfTurnFinishesOpenItems` checks `h.rpc.Done()` stays open.
  - Delete `TestDroppedMessageDoesNotBlock` and `TestServeProtocolErrors`. The
    shared `Serve` tests cover the second.
  - Add `TestBadPermissionParamsAreRefused` (-32602).
  - `TestBegin` checks the same wire fields as today.
- **Pi:**
  - Delete `connection` through `replyTo` and the `TestServe*` tests that the
    shared tests cover (`main_test.go:569-720` and
    `TestServeRejectsCommandsBeforeStart`).
  - Turn `TestServe`, `TestServeResumesTheSession`,
    `TestServeFailedResumeIsAnErrorReply`, `TestServeRejectsAnotherModel`, and
    `TestServeEndsWhenPiExits` into `startAgent` tests with the fake `pi`. They
    keep their checks, except the idle event: they check `Agent.Conversation`.
  - `TestInputWhenNotRunning` expects the write error.
  - Add `TestPendingCallEndsWithTheExitReason`: `end(reason)` ends a waiting
    call with that reason.
- **`internal/harness`:** the existing `TestCrashFailsWaitingRequests` and
  `TestPluginThatEndsReportsItsDiagnostics` cover `Do`. The window between a
  failed write and `done` closing cannot be hit reliably, so `Do` gets no new
  test.

## Implementation plan

Each step builds and passes `make check` on its own. Step 3 changes `Start`,
`Agent`, and `RPC`'s signatures, so it is one worker's step over all four
plugins. Everything that can be done before it is done first, in parallel.

1. **Shared helpers** (`plugins/internal/plugin/form.go`, `form_test.go`,
   `plugin.go`, `plugin_test.go`). Add `Errors`, `Field`, `Option`,
   `ParseForm`, and `OptionNotes`, with their tests. Change `WriteFile`'s temp
   pattern to `"." + name + "-*.tmp"`. Nothing uses the new code yet.
2. **Per-plugin work on today's process code.** These steps can run in
   parallel, since each touches only its own files. Each also replaces its
   error counter with `plugin.Errors`.
   - **2a. Claude** (`plugins/claude/main.go`, `main_test.go`): input decoded
     once, answers split by kind, and `plugin.OptionNotes`, with
     `TestQuestionInputIsKept`. Startup waits for step 3.
   - **2b. Codex** (`plugins/codex/requests.go`, `main.go`, `events.go`,
     `main_test.go`): requests as plain data, the shared form helpers (deleting
     `field`, `option`, `choices`, `parseForm`, and `optionNotes`), and
     steering from `thread/read`, with the steering tests.
   - **2c. OpenCode** (`plugins/opencode/main.go`, `main_test.go`, `go.mod`,
     `go.sum`): the move to `plugin.RPC`, using today's
     `NewRPC(stdout, stdin, handle)` and today's process code; local structs;
     form replies by ID; the shared form helpers; the permission error; and
     `sortedKeys`. Then drop the SDK. Its `call` keeps Codex's current
     explanation code until step 3. It is the largest step. Keep `update`,
     `chunk`, `toolCall`, `toolOutput`, `form`, `parent`, and `answer` in their
     current shape, with only their parameter types changed. All the OpenCode
     test changes land here.
   - **2d. Pi** (`plugins/pi/main.go`, `main_test.go`): `connect` under `mu`,
     and the write-failure wait removed, with `TestInputWhenNotRunning`
     expecting the write error.
   - **2e. `internal/harness`** (`internal/harness/harness.go`): `Do`.
3. **`Process`, line-fed `RPC`, and `Serve` across all plugins**, after all of
   step 2 (`plugins/internal/plugin/process.go`, `process_test.go`, `rpc.go`,
   `rpc_test.go`, `serve.go`, `serve_test.go`, `claude/main.go`,
   `codex/main.go`, `opencode/main.go`, `pi/main.go`, and the four
   `main_test.go`).
   - Add `ReadLines` and `Process`, and make `RPC` take lines through `Line`
     and `End`.
   - Change `Agent` and `Start`, emit idle from `Serve`, and delete `Idle`,
     `exited`, each `idle` method, `Tail`'s exported name, and the plugins'
     `died`, `stop`, `LookPath`, pipe, and read-loop code.
   - Each plugin's `stop` becomes `proc.Stop()`, after `cancel()` for Codex and
     OpenCode.
   - Codex's and OpenCode's `call` lose their explanation code. Pi's `call`
     uses `end` and `ended`.
   - Claude's startup handler replaces `ready`, `initError`, `resultText`, and
     `failure()`, because it is built on `Process.Read`.
   - Test changes:
     - Delete the Claude and Pi `connection` harnesses and convert their
       `TestServe*` tests to start-level tests as listed, including Claude's
       failed-resume variant and `newClaude` without `ready`.
     - Delete Codex's `TestErrorsExplainADeadConnection`.
     - Move the stand-ins in the Codex and OpenCode tests to
       `NewRPC(w, handle)` with `ReadLines`.
4. Run `make check`, then `make dev` for a live smoke test of one agent per
   installed harness: start, a prompt, and an interrupt. Stop it and run
   `scripts/dev clean`.
