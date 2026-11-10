# Work log: utility prompts in the OpenCode and Pi plugins

- Task ID: 2026-11-10-002-utility-model-opencode-pi
- Plan (repository-relative): `.agents/plans/2026-11-10-002-utility-model-opencode-pi.md`,
  blob `50ce0817f232f085ae7091b6385ed08d87119990` (matches the accepted plan)
- Plan review (repository-relative): `.agents/reviews/plans/2026-11-10-002-utility-model-opencode-pi.md`
  (round 1 and the coordinator's execution decision, proceed)
- Guidance blob: `.agents/research/utility-model.md` `7b1da4f39f4acb933c21d10304224bbd7855d6e9`
- Author: implementer-2, Hypercube child session of 12efdd39 (Claude Code,
  Sonnet 5.5); native conversation ID: see the handoff message (not exposed to the
  session as a file).
- Status: complete
- Code base: `eb718b37025a2e586ecdafd5d8a59290feacc138`
- Code head: uncommitted working tree on the Code base. By user instruction
  nothing was committed, staged or unstaged; the coordinator commits once.
- Commits: none.

## Changed paths

Modified: `plugins/opencode/configure.go`, `plugins/opencode/discovery.go`,
`plugins/opencode/main.go`, `plugins/opencode/main_test.go`,
`plugins/pi/main.go`, `plugins/pi/main_test.go`, `docs/configuration.md`,
`docs/dev/architecture.md`, `docs/harness-support.md`.

New: `plugins/opencode/generate.go`, `plugins/opencode/generate_test.go`,
`plugins/pi/generate.go`, `plugins/pi/generate_test.go`, this log.

## What changed

- OpenCode: `launchCatalog` (launch, subscribe, `awaitCatalog`; stops the agent
  on failure) is extracted from `models` and shared with `generate`. `choose`
  is extracted from `selectSettings` with the same rules and messages.
  `generate` follows the plan: temp dir removed after the server stops, check
  via `choose`, `POST /api/experimental/generate/text` with `providerID`/`id`
  from the matched model and `variant` only when set.
- Pi: `generate` runs a check process (`--mode rpc` + `utilityFlags`) that
  requires the model in `get_available_models`, then `state.check` and
  `checkLevel`; then a `pi -p --mode json` run with the prompt on stdin. The
  latest assistant `message_end` gives the text, stop reason and error message.
- Both plugins set `Generate: generate`.
- Docs: new `## Utility model` in `harness-support.md`; `configuration.md` and
  `architecture.md` updated as planned.
- Tests per plan: `connected` serves the generate reply, `fakeExecutable` logs
  `$(pwd)` as line 3; `fakePi` handles `-p` runs (`$FAKE_ARGS.print/.dir/.stdin`,
  `$FAKE_PRINT`, `$FAKE_PRINT_FAIL` non-empty test) and defaults both empty.

## Deviations

- Claude Code's "what still loads" cell in the docs table reads "Your Claude
  Code login." The plan did not specify that cell; the research says safe mode
  keeps the normal login.
- OpenCode `TestGenerateRejectsASelectionTheServerDoesNotOffer` adds a third
  case (`p/m` with no effort) beyond the plan's two.
- Pi failure table: the "retried" case (errored message followed by a `stop`
  one) asserts success with the second text, as the plan lists.

## Checks (all against the uncommitted working tree on the Code base)

Environment: macOS, OpenCode at `~/.opencode/bin/opencode`, Pi at
`/opt/homebrew/bin/pi`.

- `gofmt -l plugins` and `go vet ./plugins/opencode ./plugins/pi`: no output.
- `go test ./plugins/opencode ./plugins/pi`: `ok` for both.
- `make check`: exit 0. Trimmed tail: `ok .../plugins/claude`, `ok .../plugins/opencode`,
  `ok .../plugins/pi`, `ok .../plugins/scripted`, all other packages `ok`;
  `bunx tsc` and `bun test` ran (act() warnings are pre-existing).
- Live `generate`:
  - `echo '{"model":"openai/gpt-5.6-terra","effort":"low",...}' | go run ./plugins/opencode generate` → `{"text":"Stabilize Flaky Parser Test"}`, rc 0.
  - `echo '{"model":"openai-codex/gpt-5.6-luna","effort":"minimal",...}' | go run ./plugins/pi generate` → `{"text":"Fix Flaky Parser Test"}`, rc 0.
  - OpenCode effort `bogus`: `model openai/gpt-5.6-terra: OpenCode does not offer effort "bogus"; see list_models`, rc 1.
  - Pi effort `bogus`: `Pi model openai-codex/gpt-5.6-luna does not accept thinking level "bogus"; see list_models`, rc 1.
  - Pi `openai-codex/nonexistent-xyz`: `Pi does not offer model openai-codex/nonexistent-xyz with extensions off, as utility prompts run it; choose a model that no Pi extension provides`, rc 1.
  - No `hypercube-utility-*` directories remained in the temp directory after these runs.
- Live dev check against the running session on port 4281 (its plugins in
  `bin/dev` were rebuilt at 06:05, after the sources changed). Original
  `utility_selection`: `{"effort":"low","harness":"claude","model":"haiku"}`.
  - Set `{"harness":"opencode","model":"openrouter/~deepseek/deepseek-flash-latest","effort":"low"}`;
    created a Claude Code haiku session through `POST /session?action=new` with
    the first message "Please fix the flaky parser test in the tokenizer
    package…". Its title became `Fix flaky tokenizer parser test`.
  - Same with the Pi entry: title became `Fix flaky tokenizer parser test`.
  - The title is not the start of the message, so the utility model named it.
  - Deleted both test sessions (`4d934de4`, `6a5255c5`); both are gone from the
    database. An earlier attempt without an `Origin` header was rejected
    (`forbidden origin`) and created nothing.
  - Restored the config by copying the saved original back; `cmp` shows it is
    byte-identical.

## Limitations

- Linux not tested. The dev check used the title as the observable result; the
  plugins' `generate` output itself was checked by the live `generate` runs.
- I did not read the dev service log for generate errors; both titles changed.

## Repair 1: work review round 1

Review: `.agents/reviews/work/2026-11-10-002-utility-model-opencode-pi.md`. Code
base unchanged (`eb718b37025a2e586ecdafd5d8a59290feacc138`); Code head is still
the uncommitted working tree, now including these edits. Nothing committed.

- Finding 1: the OpenCode rejection test asserts each case's message
  (`does not offer effort "minimal"`, `does not offer model "p/missing"`,
  `needs an effort`).
- Finding 2: the proxy helper moved to `main_test.go` as `owned`;
  `TestOwnedServerDiscoveryAndLaunch` and the generate tests use it.
- Finding 3: the Pi table is `TestGenerateResults`; each row has `text` and
  `want`, with one assertion rule (`want == ""` means success with `text`).
- Finding 4: `docs/harness-support.md` drops the third column and follows the
  table with the OpenCode and Pi sentences from the review;
  `docs/configuration.md` link text reads "what each harness runs".
- Review notes: `errors.New(o.failure)` in `plugins/pi/generate.go`; the `print`
  variable in the Pi test is now `prompt`.

Checks on this working tree: `gofmt -l plugins` and `go vet` clean;
`go test -count=1 ./plugins/opencode ./plugins/pi` ok for both; `make check`
exit 0.

## Completion

- Plan review: `.agents/reviews/plans/2026-11-10-002-utility-model-opencode-pi.md`
  (round 1; coordinator execution decision `proceed`).
- Work review: `.agents/reviews/work/2026-11-10-002-utility-model-opencode-pi.md`
  (round 1 `changes-required`; coordinator execution decision `accept` after
  Repair 1).
- Commits: one coordinator commit, by user instruction. Code base
  `eb718b37025a2e586ecdafd5d8a59290feacc138`.

## Session provenance (all stages)

| Stage | Coordinator session | Harness | Native conversation | Model |
| --- | --- | --- | --- | --- |
| Coordinator | 12efdd39 | Claude Code | `db73ed8b-a2fe-4d6b-8cfc-a41de1a7f3a3` | Opus 5.5 |
| xplan | `planner-2`, child of 12efdd39 | Claude Code | `6b95bc6a-d9ec-42df-a2df-f17362be997f` | Opus 5.5, high |
| xreview plan | `plan-reviewer-2`, child of 12efdd39 | Claude Code 2.1.319 | `62c77d0a-1fa4-451f-8bbe-42b9846440ec` | Opus 5.5, high |
| xwork and Repair 1 | `implementer-2`, child of 12efdd39 | Claude Code | unavailable: not exposed inside the session | Sonnet 5.5, medium |
| xreview work | `work-reviewer-2`, child of 12efdd39 | Claude Code | `c2153148-974a-4c8d-bdf7-0971dc153e50` | Opus 5.5, high |
