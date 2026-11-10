# Utility model and automatic session titles (Claude Code)

- Task ID: 2026-11-10-001-utility-model-titles
- Status: ready
- Source HEAD: d14fcee
- Author: planner, Hypercube child session `planner` of 12efdd39 (Claude Code,
  Opus 5.5); native conversation `9702b77b-3288-4ff3-9270-382b4430ab2e`
- Dependencies: none. Evidence: `.agents/research/utility-model.md`
  (repository-relative). Request: slice 1 of GitHub issue #66.

## Outcome and boundaries

- **Settings › General › Utility model** chooses a harness, model, and effort,
  saved as `utility_selection` in `config.json`. Unset by default.
- With a utility model, every new session is renamed by it after its first
  message: a top-level session's first prompt, a child's brief. A title the
  user set with Rename is never replaced. Without one, titles stay as today.
- A new plugin command, `generate`, sends one prompt to a model and returns
  its text. Only the Claude Code plugin implements it; OpenCode and Pi decline.
- The agent-facing `set_title` tool and the instruction to call it are removed.

Non-goals: OpenCode and Pi `generate` (slice 2); parked-session summaries
(slice 3); renaming existing sessions; showing title failures in the interface.

## Current system

- Titles: `report.Title` at creation in `Runtime.create`
  (`internal/session/sessions.go`); children take `ChildOptions.Title`.
  Every child path (direct, accepted offer, started proposal) goes through
  `Runtime.createChild` (`children.go`); its brief is `options.Message`.
  `Runtime.Rename` serves the browser's `rename` action and `set_title`
  (`internal/mcp/mcp.go`); `report.Instructions` tells top-level agents to
  call `set_title`.
- One-shot plugin commands (`describe`, `models`): documented in
  `internal/harness/discovery.go`, dispatched by `plugin.Plugin.run`
  (`plugins/internal/plugin/main.go`), run by `harnessclient.run`, which
  cancels with SIGTERM and kills the process group after `cancelGrace`. The
  Claude plugin's `models` reads `initialize`; `check` validates a selection.
- Precedents: `ReviewSelection` (`internal/config/config.go`), `SettingsReview`
  (`settings-preferences.tsx`), `addColumn` (`internal/store/store.go`).

## Decisions

### Protocol: `generate`

Add to `internal/harness/discovery.go` (update the comment to three one-shot
commands):

```go
// `generate` reads a GenerateRequest from stdin, sends Prompt to the model
// once, without tools, project instructions, or a saved native conversation,
// and writes a Generation. A plugin that cannot fails with the reason.
type GenerateRequest struct {
	Model  string `json:"model"`
	Effort string `json:"effort,omitempty"`
	Prompt string `json:"prompt"`
}
type Generation struct {
	Text string `json:"text"`
}
```

- Output is a JSON `Generation` holding plain text. Every one-shot command
  writes one JSON value, which `harnessclient.run` decodes. Failures exit
  nonzero with the reason on stderr, as `plugin.Main` already does.
- No system-prompt field: OpenCode's generation endpoint accepts only a
  prompt. Each plugin supplies its own neutral system prompt where it can.
- `plugin.Plugin` gains `Generate func(context.Context, harness.GenerateRequest) (string, error)`.
  `run("generate")` decodes the request; nil `Generate` fails with
  `"<Product> does not run utility prompts"`; otherwise it checks the
  executable like `models`, cancels its context on SIGTERM (no timeout of its
  own; the backend owns the timeout), and encodes the text. Plugins built
  before this change fail with `unknown command`; both are clean failures.
  The usage line in `plugin.Main` becomes `serve | describe | models | generate`.
- `harnessclient.Generate(ctx, dir, harnessID, req) (string, error)` uses
  `run` and returns an error `harness <id>: empty answer` when the text is
  blank after trimming. This is the single empty-answer check.

### Claude implementation (`plugins/claude/generate.go`)

One `claude` process in a fresh `os.MkdirTemp("", "hypercube-utility-")`
directory, removed afterwards:

```
claude -p --input-format stream-json --output-format stream-json --verbose
  --safe-mode --no-session-persistence --tools '' --disallowedTools 'mcp__*'
  --strict-mcp-config --disable-slash-commands
  --system-prompt 'Reply with only what the request asks for.'
  --model M [--effort E]
```

These are the agreed flags plus stream-JSON input and output. The prompt still
goes in on stdin, as a stream-JSON user message. Reasons:

- The `initialize` reply lists the models and efforts Claude offers, so
  `check(models, Selection{Model, Effort})` runs before the prompt is sent. An
  effort Claude does not offer fails without a model call; the warn-and-use-
  default behavior in the research never reaches the model.
- The `result` line carries `subtype` and `is_error`, so success never depends
  on the exit status.

Live check, 2026-11-10, Claude Code 2.1.319, empty directory under `/tmp`:
this command answered `initialize` with models and efforts, reported `tools:[]`
and `mcp_servers:[]`, and returned `result` `"OK"` with `subtype:"success"`.

Algorithm: start with `plugin.StartProcess("Claude Code", cmd)`, write the
`initialize` control request, and run `p.Read` in a goroutine with a handler
that decodes each line into the existing `message` type:

- `control_response` for `initialize`: an error subtype, `initialized` failure,
  or `check` failure records the failure and closes stdin; otherwise write
  `{"type":"user","message":{"role":"user","content":PROMPT}}`.
- `result`: `is_error` or a subtype other than `success` records
  `errors.New(m.summary())`; otherwise record `m.Result`. Close stdin.
- A line that does not decode as JSON: keep it, trimmed, as the last plain
  line. Claude can write failures such as "Not logged in · Please run /login"
  on stdout with empty stderr.
- Ignore other lines.

The caller waits for `Read` to return or `ctx` to end; on `ctx`, `p.Stop()`
and return the context error. After `Read` returns: a recorded failure is the
error; with no `result`, return `Read`'s exit reason (it carries stderr's
tail), joined after the last plain line when there is one
(`fmt.Errorf("%s: %w", line, readErr)`); otherwise return the text, empty or
not.

### Title source

Add `Renamed bool` to `store.Session`, column
`renamed INTEGER NOT NULL DEFAULT 0` (schema, `addColumn` in `Open`,
`AddSession`, `SaveSession`, `sessionColumns`, `scanSession`). `Runtime.Rename`
sets it. Only the user renames once `set_title` is gone, so the flag means "the
user chose this title". Rejected: comparing against the title the call started
with, which fails when the user types the same text. Existing rows read
`false`; no existing session gets a utility call, so that is harmless.

### When the call runs and what it receives

New `internal/session/titles.go`, `Runtime.nameSession(id, message string)`:

- Called in `create` right after the session is saved (before `r.input`, so
  it runs alongside the agent's start; a created session is kept even if its
  first input fails), and in `createChild` right after `r.launch` succeeds (a
  child whose start fails is deleted).
- Returns at once when `message` is blank (image-only prompts keep "Image";
  children without a brief keep the title they were created with), when `config.Load`
  fails (log it), or when `UtilitySelection` is nil.
- Under `mu`: return if `closing`; otherwise create
  `context.WithTimeout(context.Background(), r.utilityTimeout)`, store its
  cancel in `r.utilityCancels[id]`, and `r.utilityCalls.Add(1)`. The goroutine
  calls `harnessclient.Generate` with the selection and
  `report.TitlePrompt(message)`, passes the answer to `r.applyTitle`, logs any
  error with `logf("naming session %s: %v", id, err)`, then removes its cancel
  under `mu` and calls `Done`.
- `Runtime.applyTitle(id, answer string) error`: `report.GeneratedTitle(answer)`,
  then `r.update(id, …)` that sets the title only when `!s.Renamed`.
  `store.ErrNotFound` (session deleted) returns nil.

### Prompt and cleanup (`internal/report`)

`TitlePrompt(message)` cuts the message to its first 4,000 characters (enough
for the gist, bounds cost and latency) and returns:

```
Write a title for a coding session from its first message below. Use at most 48 characters, sentence case, no quotes, and no ending punctuation. Reply with only the title.

First message:
<message>
```

`GeneratedTitle(answer) (string, error)` takes the first line that is not
blank after trimming whitespace, surrounding quotes, backticks, and asterisks,
and one trailing period,
and returns `Title(line)` (collapsed whitespace, 48-character cut with "…").
No such line is an error.

### Timeout, cancellation, failure, concurrency

- `utilityTimeout` is a `Runtime` field, one minute (cold Claude answered in
  about 2 s, 100 KB prompts in 5 s); tests shorten it.
- `Quit` and `Detach`, after setting `closing`, collect and call every cancel
  in `utilityCancels`, then wait on `utilityCalls` after their existing work.
  Deleting a session does not cancel its call; the write finds no session.
- Failure: the session keeps its existing title and the reason goes to the
  service log (`~/.local/state/hypercube/ui.log`). Nothing retries, and a call
  lost to a backend restart is not repeated: titles are cosmetic, and retrying
  a misconfigured model only repeats the failure.
- Concurrent rename: both writes are SQLite write transactions, so they
  serialize. The utility write reads `Renamed` inside its transaction, so a
  user rename before it wins, and a rename after it replaces the utility title.
- No limit on concurrent calls: there is one per created session.

### Config and Settings

- `Config.UtilitySelection *harness.Settings` (`json:"utility_selection,omitempty"`),
  validated with `Check` like `ReviewSelection`; update the `Config` comment.
- Browser `Config` type gains `utility_selection?: Settings`.
- `SettingsGeneral` gains a third `Group title="Utility model"`. Its heading is
  the only visible heading; under it, copied from the `SettingsReview` model
  row without that row's `<span>` heading: a flex row with `ModelSelect` over
  `draft.models` (`label="Utility model"` as its accessible name) and, while
  set, a ghost **Clear** button that sets `utility_selection` to `undefined`;
  then the help text: "Names each new session after its first message.
  Without one, a session's title is the start of its first message. Titles you
  set with Rename stay." The General page has no screenshot, so none change.
- The picker lists every model entry. Choosing an OpenCode or Pi model gives
  logged failures and today's titles until slice 2; `configuration.md` says to
  choose a Claude Code model.

### Removing `set_title`

- Delete the tool and `setTitleArgs` from `internal/mcp/mcp.go`, `TestSetTitle`
  from `mcp_test.go`, the instruction line from `report.Instructions`, and
  `"set_title"` from the expected strings in `report_test.go`.
- Keep the `set_title` entry in `internal/ui/web/transcript/transcript-tree.ts`:
  saved transcripts contain these calls and should keep their readable title.

### Docs

- `docs/configuration.md`: new `## Utility model` after Session defaults:
  where it is, the `utility_selection` field (default not set, **Clear**),
  the title rule (user prompt or child brief; Rename wins; without one or on
  failure the title stays; failures in the service log), that it must be a
  Claude Code model, and that it runs with no tools, MCP servers, project
  instructions, or saved conversation, in an empty temporary folder.
- `docs/examples/config.json`: add `utility_selection` (claude, haiku, low).
- `docs/usage.md` Rename paragraph: titles come from the first message or the
  [utility model](configuration.md#utility-model); a title you set stays.
- `docs/dev/architecture.md` plugin protocol: three one-shot commands; describe
  `generate` and that the backend uses it for the utility model.
- `docs/dev/glossary.md`: add **utility model**: "The harness, model, and effort
  chosen in Settings for short background prompts, such as naming sessions.
  A utility prompt runs without tools and saves no native conversation."

## Implementation sequence

1. Protocol types, `plugin.Plugin.Generate`, `harnessclient.Generate`.
2. Claude `generate`, wired in `main` as `Generate: generate`.
3. Store column, config field, `report` functions, runtime changes.
4. Stand-in `generate` in `internal/harness/harnesstest`, documented in its
   package comment: log `Entry{Kind: "generate", Model, Effort, Text: prompt}`
   to `generate.log`; answer the exported constant `UtilityTitle`; when the
   prompt contains `!utility-fail`, print a reason to stderr and exit 1; when
   it contains `!utility-hold`, block until killed.
5. Remove `set_title`; browser setting; docs.

## Verification

- `harnessclient` (`discovery_test.go`, shell-script plugins): the request
  JSON reaches stdin; a text answer returns; blank text fails with
  `empty answer`; exit 1 with stderr reports the stderr line.
- `plugins/internal/plugin`: `run("generate")` with nil `Generate` fails with
  the product name.
- `plugins/claude/generate_test.go` with `fakeClaude`, extended to write
  `pwd > "$FAKE_ARGS.dir"` and to append each stdin line to
  `"$FAKE_ARGS.stdin"`:
  - a success `result` returns its text; argv contains each restriction flag,
    `--model`, `--effort`, and the system prompt; the working directory was an
    empty temporary directory that is gone afterwards;
  - an `is_error` result fails with its text;
  - `haiku` with `high` fails with `does not accept effort`, and no line of
    `"$FAKE_ARGS.stdin"` contains `"type":"user"`;
  - `FAKE_FAIL` fails with its stderr text;
  - a fake that prints a plain, non-JSON line and exits 1 fails with an error
    containing that line (`FAKE_BEFORE` pointing at a file with that line,
    plus `FAKE_FAIL`).
- `internal/report`: `GeneratedTitle` table (quotes, trailing period, leading
  blank lines, overlong cut, blank answer fails); `TitlePrompt` contains the
  message and cuts a long one; `Instructions` no longer mentions `set_title`.
- `internal/config`: `utility_selection` round-trips; an invalid one is refused.
- `internal/session/titles_test.go` (config written with `config.Save` under
  the fixture's `XDG_CONFIG_HOME`, utility `test`/`m`/`low`; wait with
  `f.rt.utilityCalls.Wait()`):
  - a created session gets `UtilityTitle`; the logged request has model `m`,
    effort `low`, and a prompt containing the message;
  - without a utility selection, the title is `report.Title(text)` and
    `generate.log` has no entries;
  - `!utility-fail` keeps the first-message title;
  - `applyTitle`, called directly on sessions created without a utility
    selection: after `Rename`, the user's title stays and `Renamed` is saved;
    on a session not renamed, the generated title is set; on a deleted
    session, it returns nil;
  - a child created with a title and brief gets `UtilityTitle`;
  - with `!utility-hold`, `Quit` returns and the title is unchanged.
- `internal/store`: an old database gains `renamed` reading `false` (extend
  the existing reopen test).
- Browser (`settings-dialog.test.tsx`): on General, choosing a utility model
  and saving posts `utility_selection`; **Clear** then save omits it.
- `make check`.
- Live check: `echo '{"model":"haiku","effort":"low","prompt":"Write a title for: fix the flaky parser test"}' | go run ./plugins/claude generate`
  prints a JSON `text`; with `"effort":"minimal"` it fails with
  `does not accept effort`. Then `make dev`, choose a Claude utility model,
  start a session, and see its title change. Record results in the work log.

## Handoff

Ready; no open engineering decisions or blockers.

Size: one normal assignment at its upper end; each layer copies an existing
pattern. Length is above the 600–1,500 word band because of the decisions the
request asked to settle and their tests. A split, if wanted: unit A is steps
1–2 with their tests; unit B is the rest.

After `make install-service`, the backend uses the new plugin for `generate`
and stops serving `set_title` at once; running agents keep the old
instruction until they restart.
