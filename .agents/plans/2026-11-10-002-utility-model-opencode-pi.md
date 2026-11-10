# Utility prompts in the OpenCode and Pi plugins

- Task ID: 2026-11-10-002-utility-model-opencode-pi
- Status: ready
- Source HEAD: eb718b37025a2e586ecdafd5d8a59290feacc138
- Author: planner-2, Hypercube child session of 12efdd39 (Claude Code,
  Opus 5.5); native conversation `6b95bc6a-d9ec-42df-a2df-f17362be997f`
- Dependencies (repository-relative): slice 1 plan
  `.agents/plans/2026-11-10-001-utility-model-titles.md`; evidence
  `.agents/research/utility-model.md`. Request: slice 2 of GitHub issue #66.

## Outcome and boundaries

- The OpenCode and Pi plugins implement `generate`, so a utility model from
  either harness names new sessions as a Claude Code one does.
- Each plugin checks the model and effort against what the harness offers
  before it sends the prompt, with the same rules and messages as a session
  start.
- Docs stop saying the utility model must be a Claude Code model and say what
  each harness still loads.

The `generate` protocol, `harnessclient.Generate`, the backend, and title
handling stay unchanged; no protocol defect was found. Non-goals:
parked-session summaries (slice 3), title behavior.

## Current system

- Protocol: `GenerateRequest{Model, Effort, Prompt}` and `Generation{Text}` in
  `internal/harness/discovery.go`. `plugin.Plugin.run("generate")`
  (`plugins/internal/plugin/main.go`) cancels the context on SIGTERM and has no
  timeout; the backend's one-minute timeout and `harnessclient.run`'s
  SIGTERM-then-group-kill own it. `harnessclient.Generate` rejects blank text.
- Reference: `plugins/claude/generate.go`, a temporary directory
  `hypercube-utility-*` removed afterwards, a check before the prompt, and a
  last plain stdout line joined to `Read`'s exit reason.
- OpenCode: `launch(ctx, dir, env...)` (`server.go`) starts
  `opencode serve --stdio --port 0` with a random `OPENCODE_PASSWORD`;
  `server.call` decodes `{"data":…}` and turns a non-2xx reply into
  `OpenCode METHOD PATH: <body message or status>`. `models` (`discovery.go`)
  launches, subscribes to `/api/event`, waits in `awaitCatalog` for native
  plugins to activate, then reads `/api/model`. `selectSettings`
  (`configure.go`) holds the model/variant rules.
- Pi: `launch(dir, argv, emit, env...)` (`main.go`) runs an RPC process;
  `ag.call`, `sessionState.check`, and `ag.checkLevel` (`configure.go`) are what
  `models` and `connect` use to check a selection.
- Tests: OpenCode's `connected(t)` fake server and `fakeExecutable`, with
  `TestOwnedServerDiscoveryAndLaunch` running the real `models` through a
  password-checking proxy (`main_test.go`); Pi's `fakePi` shell script
  (`main_test.go`).

## Decisions

### OpenCode (`plugins/opencode/generate.go`)

1. `os.MkdirTemp("", "hypercube-utility-")`, removed by a deferred
   `os.RemoveAll` registered before the agent's deferred stop, so the server
   has exited before removal.
2. Launch there with no extra environment, then subscribe and `awaitCatalog`.
   Extract this sequence from `models` into one helper both use; it stops the
   agent itself on failure.
3. `GET /api/model`, then check the selection with a helper extracted from
   `selectSettings`, which then uses it too:
   `func choose(models []modelInfo, s harness.Selection) (modelInfo, error)`
   with today's rules and messages (enabled model whose `key()` matches; a
   model with variants needs one of them; a model without takes none).
4. `POST /api/experimental/generate/text` through `server.call`:

   ```json
   {"prompt":"…","model":{"providerID":"p","id":"m","variant":"low"}}
   ```

   `providerID` and `id` come from the matched `modelInfo`, not from splitting
   the string; matching `key()` is equivalent to splitting on the first slash
   and keeps IDs such as `q/b/c` intact. `variant` is the effort, omitted when
   empty (`modelRef` with `omitempty`). Decode `data` into `{text}` and return
   it unchanged.

Reasons:

- Checking first keeps one rule set for sessions and utility prompts and
  rejects a disabled model or missing variant, which the endpoint may accept.
  Waiting for the catalog matters because providers from native plugins appear
  only after activation. Live check (OpenCode 2.2.6, empty temporary
  directory): `/api/plugin` was empty at first and listed active built-in
  plugins at 0.8 s, so `awaitCatalog` ends; generation answered `OK` with and
  without `variant`; an unknown model gave 404 `Model not found: openai/nope`;
  the directory stayed empty.
- No `OPENCODE_CONFIG_CONTENT`: no session runs, so the Hypercube MCP server,
  instructions and permissions do not apply. The empty directory keeps project
  configuration and project MCP servers out. Global configuration still loads,
  so global MCP servers and plugins start for each call. The research found
  no launch override that removes them while keeping providers; document it.
- `opencode run` is rejected: it loads project instructions and saves a native
  session.
- Errors: `server.call`'s existing form, for example
  `OpenCode POST /api/experimental/generate/text: Variant unavailable for …`.
  A launch failure carries the process's exit reason.
- Cancellation: `ctx` ends the HTTP request; the deferred stop cancels the
  event stream, closes the server's stdin (it exits; `Process.Stop` kills it
  after 3 s otherwise), and waits for the stream goroutine. Return the error
  as received.

### Pi (`plugins/pi/generate.go`)

Two processes in one `hypercube-utility-*` temporary directory. Both get the
same restrictions:

```go
var utilityFlags = []string{"--no-session", "--no-tools", "--no-extensions", "--no-mcp",
	"--no-skills", "--no-prompt-templates", "--no-context-files", "--no-themes", "--no-approve", "--offline"}
```

1. **Check.** Split `req.Model` with `strings.Cut(…, "/")` (not
   `provider/id` → the `startAgent` error). `launch(dir, ["--mode","rpc"] +
   utilityFlags + ["--provider",P,"--model",M], no-op emit)`, then:
   - `get_available_models`; the requested `provider/id` must be listed, else
     `Pi does not offer model %s with extensions off, as utility prompts run
     it; choose a model that no Pi extension provides`;
   - `get_state` with `state.check(req.Model)`;
   - `ag.checkLevel(ctx, req.Model, req.Effort)`.

   Stop the RPC process before step 2.
2. **Generate.**

   ```
   pi -p --mode json <utilityFlags> --system-prompt 'Reply with only what the request asks for.'
     --append-system-prompt '' --provider P --model M [--thinking EFFORT]
   ```

   Start with `plugin.StartProcess("Pi", cmd)`, start `p.Read` in a goroutine,
   write the prompt to stdin, close stdin. Wait for `Read` or `ctx`; on `ctx`,
   `p.Stop()` and return `ctx.Err()`. The handler decodes `type` and
   `message.role` first, and only for `message_end` with role `assistant`
   decodes `content` as blocks (user and system messages can have string
   content) and keeps the latest such message's text blocks joined with
   `"\n"`, `stopReason`, and `errorMessage`. It keeps the last non-JSON line
   as plain text and ignores other lines.
3. **Result**, after `Read` returns:
   - last assistant message with `stopReason` `stop`: its text;
   - other stop reason: `errorMessage`, or `Pi stopped with reason "<reason>"`
     when empty;
   - no assistant message: `Read`'s exit reason (it has stderr's tail), as
     `fmt.Errorf("%s: %w", plain, readErr)` when a plain line exists.

Reasons:

- Pi accepts unknown IDs as custom models: live, with these flags,
  `--model nonexistent-xyz` gave `get_state` reporting that same ID, so
  `state.check` passes it. Only the `get_available_models` listing catches it.
  The RPC process uses the generation flags, so a model that needs an
  extension is absent there and fails before any model call. Rejected: falling
  back to extensions for such a model; extensions bring tools, prompts and
  hooks back.
- `checkLevel` gives the session rule and messages; Pi would otherwise lower
  an unsupported level silently.
- Rejected: one RPC process that also sends the prompt, which would need
  `--system-prompt` handling and a second event reader beside the agent's
  transcript translation. The second Pi start costs well under a second.
- `--mode json` instead of plain `-p`: `Process.Read` drops blank lines, which
  would join paragraphs, and the stop reason turns the observed empty exit-0
  answer (stop reason `length`) into an error with a reason. Live check
  (Pi 1.4.0, these flags, `openai-codex/gpt-5.6-luna`, `minimal`): the last
  assistant `message_end` had `stopReason:"stop"` and text `"Alpha\n\nBeta"`;
  the directory stayed empty.
- Taking the latest assistant message covers Pi's automatic retries, which
  end an errored message before the retried one.
- Without `--append-system-prompt ''`, Pi loads a global `APPEND_SYSTEM.md`.
- Errors such as missing credentials go to stderr with exit 1 and reach the
  user through `Read`'s exit reason.

### Wiring and docs

- `main` of each plugin sets `Generate: generate`.
- `docs/harness-support.md`: new `## Utility model` section after
  Requirements, a table of what each harness runs and still loads: Claude Code
  runs `claude -p` in safe mode with no tools, MCP servers, or slash commands;
  OpenCode runs a private server's text generation request, and the global
  OpenCode configuration loads, so global MCP servers and plugins start for
  each call; Pi runs `pi -p` with extensions, MCP, skills, prompt templates,
  context files, and themes off, so models from Pi extensions are unavailable.
  Add that a model or effort the harness does not offer fails before the
  prompt is sent.
- `docs/configuration.md` Utility model: replace "Choose a Claude Code
  model. …" with: any harness's model can be chosen; the call runs with no
  tools, project instructions, or saved conversation, in an empty temporary
  folder; link `harness-support.md#utility-model` for what each harness still
  loads.
- `docs/dev/architecture.md`: replace "Only the Claude Code plugin implements
  it." with "The Claude Code, OpenCode, and Pi plugins implement it."

## Implementation sequence

1. OpenCode: extract the launch/subscribe/catalog helper and `choose`; switch
   `models` and `selectSettings` to them (existing tests must pass unchanged);
   add `generate`; wire it; tests.
2. Pi: `utilityFlags`, `generate`; wire it; tests.
3. Docs.

## Verification

OpenCode, `plugins/opencode/generate_test.go`, built like
`TestOwnedServerDiscoveryAndLaunch` (`connected(t)`, a proxy checking the
launch password, `fakeExecutable`). Add a default
`/api/experimental/generate/text` reply `{"data":{"text":"Fix the parser"}}`
to `connected`, and `"$(pwd)"` as a fourth logged line in `fakeExecutable`.

- `p/m`, `low`: returns `Fix the parser`; one generate call with body
  `prompt` and `model` `{providerID:p, id:m, variant:low}`; no `/api/session`
  call; launch line 0 is `serve --stdio --port 0`, line 2 (config content)
  empty, line 3 contains `hypercube-utility-` and no longer exists.
- `q/b/c`, no effort: `model` is `{providerID:q, id:"b/c"}` without `variant`.
- `p/m` with `minimal`, and `p/missing`: errors from `choose`; no generate call.
- A 400 reply `{"_tag":"InvalidRequestError","message":"Variant unavailable for p/m: low"}`
  (via `h.reply`): the error contains that message.
- A generate handler that blocks until its request context ends: cancel `ctx`
  after the call arrives; `generate` returns an error within 5 s and the
  temporary directory is gone.

Pi, `plugins/pi/generate_test.go`. Extend `fakePi`: before its
`echo "$@" > "$FAKE_ARGS"` line, when `$1` is `-p`, write the arguments to
`$FAKE_ARGS.print`, `pwd` to `$FAKE_ARGS.dir`, stdin to `$FAKE_ARGS.stdin`;
if `$FAKE_PRINT_FAIL` is non-empty (`[ -n "$FAKE_PRINT_FAIL" ]`), print it to stderr and exit 1; otherwise play
`$FAKE_PRINT` (a JSONL file the test writes) and exit 0. Default both empty.

- `openrouter/x/m`, `high`, an output with a `system` `message_end` (string
  content) and an assistant `message_end` with text `"Fix the parser\n\nMore"`
  and `stopReason:"stop"`: returns exactly that text. `$FAKE_ARGS` (check run)
  has `--mode rpc`, every `utilityFlags` entry, `--provider openrouter --model x/m`;
  `$FAKE_ARGS.print` has `-p --mode json`, every flag,
  `--system-prompt Reply with only what the request asks for.`,
  `--append-system-prompt  --provider` (empty value), `--thinking high`;
  stdin is the prompt; the directory contains `hypercube-utility-` and is gone.
- Table of failures, each case calling `fakePi` again so `$FAKE_ARGS.print`
  is in a fresh `t.TempDir()`; for the first three it does not exist:
  - `FAKE_MODEL=unknown`, model `openrouter/unknown`: `with extensions off`;
  - effort `minimal`: `does not accept thinking level`;
  - model `noslash`: `is not provider/id`;
  - an errored assistant message followed by a `stop` one: the second text;
  - `stopReason:"error"`, `errorMessage:"rate limited"`: `rate limited`;
  - `stopReason:"length"` without `errorMessage`: `"length"`;
  - `FAKE_PRINT_FAIL=No API key found for openrouter`: that text.

Commands: `go test ./plugins/opencode ./plugins/pi`, then `make check`.

Live checks, recorded in the work log (use models the machine offers):

```sh
echo '{"model":"openai/gpt-5.6-terra","effort":"low","prompt":"Write a title for: fix the flaky parser test"}' | go run ./plugins/opencode generate
echo '{"model":"openai-codex/gpt-5.6-luna","effort":"minimal","prompt":"Write a title for: fix the flaky parser test"}' | go run ./plugins/pi generate
```

Each prints a JSON `text`. The same with effort `bogus` fails before a model
call, and Pi with `openai-codex/nonexistent-xyz` fails with
`with extensions off`. Then `make dev`, set an OpenCode utility model, start
a session, see its title change; repeat with Pi.

## Handoff

Ready; no open engineering decisions or blockers. One normal assignment: two
plugin commands that copy the Claude command's shape, two small extractions
in OpenCode, and three doc edits.

Size: 1,829 words by `wc -w` (43 in code blocks); `plan_metrics.py` is not
installed here. Above the 600–1,500 band because two harnesses each need
their validation, error and cancellation decisions with live evidence, and
their tests depend on fake-script changes that must be exact. Slice 1's plan
was above the band for the same reason. A split, if wanted: OpenCode (step 1)
and Pi (step 2) are independent; docs go with whichever lands last.
