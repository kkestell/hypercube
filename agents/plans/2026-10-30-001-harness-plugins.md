# Harness plugins

## Goal

Hypercube's core knows Claude Code and OpenCode by name: `internal/harness`
builds their command lines, `internal/hook` and `internal/wait` speak Claude's
hook format and exit-status protocol, and `internal/statusline` renders
Claude's status line. Adding Codex or Pi would mean changing all of them.

When the work is done, each harness is supported by a plugin: an executable
named `hypercube-harness-<name>` installed beside `hypercube`. The core runs
the plugin to launch the harness, and the harness reaches Hypercube through
two general commands: `hypercube report <event>` and `hypercube wait`. The
core (`cmd/hypercube`, `internal/`) and the tmux package name no harness. A
later harness needs only a new plugin. This version implements only what the
Claude Code and OpenCode integrations do today, with the same behavior.

## Related code

- `internal/harness/harness.go` — `Command` builds both harnesses' argv and
  environment; `Launch` opens the tmux pane running `hypercube agent`.
- `internal/harness/config.go` — config schema; `Spec.check` accepts only
  `claude` and `opencode`.
- `internal/harness/opencode.js` — OpenCode plugin source, embedded and
  written to the state directory with the executable path substituted.
- `internal/hook/hook.go` — handles Claude-named events, skips native
  subagents by `agent_id`, writes Claude's response format, builds the team
  facts and failure messages.
- `internal/wait/wait.go` — idle delivery; writes messages to stderr and the
  CLI exits 2 (Claude's `asyncRewake` protocol); reads `agent_id` from stdin.
- `internal/statusline/statusline.go` — Claude's status line and clearing of
  the approval flag.
- `cmd/hypercube/main.go` — `runAgent` calls `harness.Command` and execs the
  result; CLI cases for `hook`, `wait`, and `statusline`.
- `cmd/hypercube/integration_test.go` — stand-in `claude` and `opencode`
  executables that call `hypercube hook` and `hypercube wait`.
- `Makefile`, `scripts/dev`, `scripts/reinstall` — build and install only
  `bin/hypercube`.

## Decisions

- **Plugins are executables, found beside `hypercube`.** The core resolves
  `filepath.Join(filepath.Dir(os.Executable()), "hypercube-harness-"+name)`.
  An executable boundary is what lets Codex or Pi be added without a core
  release, in any language. Looking only beside the executable keeps a dev
  build paired with its own plugins and an installed build with the installed
  ones, with one lookup rule. `harness.Launch` checks the plugin exists before
  opening a pane, so `create_agent` and `start` return a clear error for an
  unknown harness. `Spec.check` requires only a non-empty harness.
- **Shipped plugins are Go programs in `plugins/claude` and
  `plugins/opencode`**, built to `bin/hypercube-harness-claude` and
  `bin/hypercube-harness-opencode`. They import nothing from `internal/`; they
  use only the protocol below, so they show what a third-party plugin needs.
  The two share no code; their few lines of common JSON handling are written
  in each.
- **Launch.** In the pane, `hypercube agent` runs `<plugin> launch` with this
  JSON on stdin and execs what it prints, as `runAgent` does now:

  ```json
  {"hypercube": "/path/to/hypercube", "env": {"HYPERCUBE_AGENT_ID": "…", "HYPERCUBE_STATE": "…"},
   "agent": "<agent id>", "role": "lead", "model": "…", "effort": "…",
   "session": "<conversation to resume, or empty>", "prompt": "<initial prompt, or empty>",
   "state_dir": "<Hypercube's state directory>"}
  ```

  Output: `{"argv": [...], "env": {...}}`. `env` in the input is the
  environment to give Hypercube processes the harness starts (the MCP server
  `hypercube mcp`). `state_dir` is where a plugin may keep files; the OpenCode
  plugin writes its JavaScript there, as now. The plugin's stderr goes to the
  pane. The core type is `harness.LaunchInput`; the output type is
  `harness.LaunchOutput`.
- **Reports from the harness.** `hypercube report <event>` reads a JSON
  object and always writes one response object, `{"context": "…"}` with
  `context` omitted when empty. Events, named for Hypercube's concepts rather
  than any harness's:

  | Event | Input | Effect | `context` |
  | --- | --- | --- | --- |
  | `start` | | | team facts |
  | `prompt` | `session` | records the conversation ID, marks busy | queued messages |
  | `tool` | | | queued messages |
  | `stop` | | marks idle when nothing is queued | queued messages; the plugin continues the turn with them |
  | `failed` | `error` | sends a worker's lead "My turn failed: …", marks idle | |
  | `approval` | | flags attention | |
  | `answered` | | clears attention | |

  The package is renamed `internal/hook` → `internal/report`, with
  `report.Run(event, stdin, stdout)` and `report.Format`. `failed` trims and
  truncates `error` to 2000 characters; choosing which harness field holds the
  error moves to the plugins. `answered` loads the state without the lock and
  does nothing when attention is not set, because the Claude plugin reports
  it on every status line redraw.
- **Room for approvals in Hypercube's prompt.** `approval` is a synchronous
  request with a response object, and the Claude plugin sends it from the
  synchronous `PermissionRequest` hook. Today the response is empty and the
  harness asks the user in its own dialog. A later version can hold the
  report until the user answers in Hypercube's prompt and return a decision
  field that the plugin translates. Nothing for that is built now.
- **Messages to the harness.** `hypercube wait` takes no input, blocks until
  the agent is idle with queued messages, prints them on stdout, and exits
  with status 2. It exits 0 with no output when a newer waiter replaces it or
  its parent process ends. The `agent_id` check leaves the core.
- **Claude plugin** (`hypercube-harness-claude`), Claude Code CLI only, as on
  the user's Team subscription:
  - `launch` prints today's `claude` argv. Hook commands become
    `'<plugin>' hook '<hypercube>' <Event>`; the async waiter becomes
    `'<plugin>' wait '<hypercube>'`; the status line becomes
    `'<plugin>' statusline '<hypercube>'`, with `--limits` for a lead.
  - `hook <hypercube> <Event>` reads Claude's hook input, skips events with
    `agent_id` except `PermissionRequest` (native subagents), maps
    `SessionStart`→`start`, `UserPromptSubmit`→`prompt` (with `session_id`),
    `PostToolUse`→`tool`, `Stop`→`stop`, `StopFailure`→`failed` (first
    non-empty of `error_details`, `last_assistant_message`, `error`),
    `PermissionRequest`→`approval`, runs `hypercube report`, and writes
    Claude's response: `{"decision":"block","reason":…}` for `Stop`, otherwise
    `hookSpecificOutput.additionalContext`. Empty context writes nothing.
  - `wait <hypercube>` reads the hook input, returns for a native subagent,
    then points stdout at stderr and execs `hypercube wait`. The exec keeps
    Claude as the waiter's parent, so the waiter still ends with Claude, and
    Claude's `asyncRewake` reads the messages from stderr with status 2.
  - `statusline <hypercube> [--limits]` runs `hypercube report answered`,
    ignoring errors, and renders today's status line.
- **OpenCode plugin** (`hypercube-harness-opencode`): `launch` writes the
  embedded `opencode.js` to `<state_dir>/opencode-plugin/index.js` with the
  hypercube path substituted, and prints today's argv and environment. The
  JavaScript calls `report` with the events above, sends `{session}` with
  `prompt` and `{error}` with `failed`, reads `context`, and starts a turn
  with a waiter's stdout on status 2.
- **The integration test uses a stand-in plugin** `hypercube-harness-test`,
  written beside the built `hypercube`. It runs the test binary, which prints
  argv that start the stand-in harness, adding `--resume <session>` when
  resuming. The stand-in harness calls `report` and `wait` directly.

## Test plan

- `internal/report`: today's hook tests on the new events and response:
  `start` facts, `prompt` records the session and delivers, `stop` continues
  with queued messages and otherwise marks idle, `tool` delivers, a failed
  write keeps messages queued, `failed` routes to the worker's own lead,
  `approval` sets and `answered` clears attention.
- `internal/wait`: replacement and delivery on stdout; a failed write keeps
  messages queued.
- `internal/harness`: config validation, including a missing harness; a
  missing plugin makes `Launch` fail with an error naming the harness.
- `plugins/claude`: `launch` output for a new lead (hooks call the plugin
  with the hypercube path, async waiters, status line `--limits`, prompt last)
  and a resumed worker (`--resume`, no `--limits`); `hook` against a stand-in
  hypercube script: `Stop` with context blocks, `UserPromptSubmit` passes
  `session_id` and returns `additionalContext`, a subagent's event is not
  reported but its `PermissionRequest` is; status line rendering, moved from
  `internal/statusline`.
- `plugins/opencode`: `launch` writes the plugin with the path substituted,
  uses `ses_hypercube_<agent>` for a new conversation and the saved session
  otherwise, sets `model#effort`, and passes `--prompt`.
- `cmd/hypercube` integration test passes unchanged in behavior through the
  stand-in plugin, including resume with the saved session.

## Implementation plan

1. Rename `internal/hook` to `internal/report`. Replace Claude event names
   with the events above, the input with `{session, error}`, and the response
   with `{context}`, always written inside the update. Drop the `agent_id`
   check. Add `answered`. Update its tests.
2. In `internal/wait`, drop stdin decoding and the `agent_id` check; `Run`
   takes the output writer. In `main.go`, `wait` writes to stdout and exits 2
   on delivery.
3. In `internal/harness`, delete `opencode.js` and the harness cases. Add
   `LaunchInput`, `LaunchOutput`, a `plugin(name)` path helper, and `Command`
   that runs `<plugin> launch` and returns argv and env. `Launch` checks the
   plugin exists first. `Spec.check` requires a non-empty harness.
4. Delete `internal/statusline`. In `main.go`, replace `hook` with `report`,
   remove `statusline`, and update the usage text.
5. Add `plugins/claude/main.go` (`launch`, `hook`, `wait`, `statusline`) and
   its tests, moving the status line code and test.
6. Add `plugins/opencode/main.go` (`launch`) with `opencode.js` moved beside
   it and updated for `report`, `context`, and stdout waits; add its test.
7. Replace the integration test's stand-in `claude` and `opencode` with the
   stand-in plugin and a `test` harness in its config.
8. `Makefile`: `build` builds all three binaries; `install` installs them;
   `check` runs `gofmt` on `plugins` too. `scripts/reinstall` installs every
   built binary. `scripts/dev` builds with `make build`.

## Documentation updates

- `docs/harness-support.md`: describe plugins, where they are found, and the
  launch, report, and wait protocol; point the Claude and OpenCode sections at
  their plugins.
- `docs/architecture.md`: source table rows for `harness`, `report`, `wait`,
  and the two plugins; remove `hook`, `statusline`, and `opencode.js`.
- `docs/configuration.md`: `harness` names an installed plugin; drop the
  "claude or opencode" restriction.
- `docs/installation.md`: `make install` installs the plugins beside
  `hypercube`.
- `AGENTS.md`: the package list under Code, and `plugins/`.
