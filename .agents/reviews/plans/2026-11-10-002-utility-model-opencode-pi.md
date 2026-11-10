# Plan review: utility prompts in the OpenCode and Pi plugins

## Round 1

- Task ID: 2026-11-10-002-utility-model-opencode-pi
- Mode: plan
- Author: plan-reviewer-2, Hypercube child session of 12efdd39 (Claude Code
  2.1.319, Opus 5.5); native conversation
  `62c77d0a-1fa4-451f-8bbe-42b9846440ec`. Independent of the planner
  (planner-2, `6b95bc6a-d9ec-42df-a2df-f17362be997f`).
- Plan (repository-relative): `.agents/plans/2026-11-10-002-utility-model-opencode-pi.md`,
  blob `f2b20a5bc0fc9d4b357f8227230234114da03d8f` (uncommitted)
- Guidance blobs: `.agents/research/utility-model.md`
  `7b1da4f39f4acb933c21d10304224bbd7855d6e9`. The slice 1 plan is listed as a
  dependency for context; its decisions are already in the code at source HEAD.
- Source HEAD: `eb718b37025a2e586ecdafd5d8a59290feacc138`

### Verdict

- Work size: right-sized. One existing protocol command, implemented in two
  plugins after the Claude Code reference, with two small extractions and three
  doc edits.
- Detail: appropriate.
- Readiness: ready after two small corrections.
- Actions: primary `resolve-decisions` (findings 1 and 2, both one-line
  wording fixes); no split or trim needed.

### Findings

1. **The Pi unlisted-model message names the wrong set of usable models.**
   Plan, Pi step 1: "`… with extensions off, as utility prompts run it; choose
   a model from a provider built into Pi`". Installed Pi 1.4.0 applies
   `--no-extensions` only to extension paths
   (`dist/core/resource-loader.js:409`, `:505`). Custom providers in
   `models.json` load separately (`dist/core/agent-session-services.js:59`,
   `dist/core/model-config.js`) and stay available with the utility flags, so
   the message sends those users away from models that work. The plan's own doc
   line ("models from Pi extensions are unavailable") is correct. Correction:
   end the message with something like "choose a model that no Pi extension
   provides". The test only matches `with extensions off`, so no test change is
   needed.

2. **The fake `pi` failure switch is ambiguous as written.** Verification,
   `fakePi` extension: "if `$FAKE_PRINT_FAIL` is set, print it to stderr and
   exit 1 … Default both empty." `fakePi` sets every variable with `t.Setenv`,
   so the default is set but empty. A literal "is set" test
   (`${FAKE_PRINT_FAIL+x}`) would fail every print run. Correction: say
   "non-empty" (`[ -n "$FAKE_PRINT_FAIL" ]`). Also state that each failure case
   calls `fakePi` again, so `$FAKE_ARGS.print` sits in a fresh `t.TempDir()`;
   otherwise "`$FAKE_ARGS.print` does not exist" depends on case order.

### Evidence checked

- **OpenCode extractions.** The launch, subscribe, and `awaitCatalog` sequence
  in `models` (`plugins/opencode/discovery.go:14-26`) is separate from `begin`,
  which launches with session configuration and keeps its own sequence. That
  makes it a clean helper for `models` and `generate`. `choose` keeps
  `selectSettings`'s rules and messages (`configure.go:79-93`) and returns the
  `modelInfo` that `selectSettings` needs for `ag.window`. `models` keeps its
  own lookup. `TestOwnedServerDiscoveryAndLaunch`,
  `TestCatalogWaitsForPluginActivation`, and `TestStreamFailureEndsTheOwnedServer`
  depend on neither signature. Adding a fourth line to `fakeExecutable` leaves
  the existing reads of lines 0–1 valid (`main_test.go:396-399`).
- **OpenCode lifetime and cleanup.** Defers run last-registered first, so
  `RemoveAll` registered first runs after `ag.stop`. `ag.stop` cancels
  `ag.ctx` (ending the event stream request), closes the server's stdin, waits
  up to 3 s and then kills (`plugin.Process.Stop`), and waits for
  `streamDone`. `launch` stops its own process on failure, and `ag.stop` handles
  a nil server. Cancelling during the generate request returns
  `server.request`'s wrapped error. In the test, the proxy passes its own
  request to `h.native.Config.Handler`, so the blocking `h.reply` sees the
  client disconnect.
- **OpenCode request.** `modelRef` already has `variant,omitempty`.
  `server.call` decodes `data`. The 400-reply test goes through
  `server.request`'s `message` extraction. The research confirms the endpoint,
  the variant 400, and the 404 for an unknown model.
- **Pi check.** `get_available_models`, `get_state` with `sessionState.check`,
  and `ag.checkLevel` exist with the stated behavior (`configure.go`,
  `discovery.go`). `checkLevel` reads the current model's levels, so the check
  run needs no `--thinking`. The fake script answers all three, with levels
  `off,low,high` and models `openrouter/x/m` and `openai/plain`. The
  `noslash`, `unknown`, and `minimal` cases fail before a `-p` run.
- **Pi JSON output.** In installed `dist/modes/print-mode.js:84-132`, JSON mode
  writes every session event and a header line, and never sets a non-zero exit
  for `stopReason` `error`, `aborted`, or `length`. Reading the stop reason is
  therefore required, and the plan does. `message_update` is the only event
  that is rewritten, so `message_end` carries the full message. Auto-retry
  (`dist/core/agent-session.js:3089-3122`) emits `auto_retry_start` and
  continues after the errored message has ended, so keeping the latest
  assistant message is correct. `Process.Read` drops blank lines
  (`process.go:87`), which supports the choice of `--mode json`. Pi reads a
  piped stdin as the prompt (`dist/main.js:49-66`).
- **Pi result fallback.** `Process.Read` always returns a non-nil reason
  (`pi exited…`), so "no assistant message → Read's exit reason" never returns
  an empty success.
- **Fake scripts.** In both scripts, `echo "$@"` renders the empty
  `--append-system-prompt` value as two spaces, as the plan expects. The `-p`
  branch exits before `echo "$@" > "$FAKE_ARGS"`, so `$FAKE_ARGS` keeps the
  check run's arguments. `pwd` resolves to the `hypercube-utility-` directory
  whether or not `PWD` is inherited.
- **Docs.** The settings picker (`settings-preferences.tsx`) offers every
  configured model, so "any harness's model can be chosen" is accurate. Each
  harness's per-run details go in one place (`harness-support.md`), and
  `configuration.md` links to them, as `documentation.md` asks. Every planned
  sentence states current behavior. The glossary entry stays accurate.

Checks: code and installed-source reading only, plus `plan_metrics.py`. No
builds, tests, or live model calls were needed.

### Normalization

- Active target: 600–1,500 words (experimental default; no curated references).
- Measured with `plan_metrics.py`: 1,903 words (66 in code blocks), above the
  band. The plan's own figure, 1,829 by `wc -w`, and its remark that the helper
  is not installed are both wrong. The helper is at
  `~/.claude/skills/xreview/scripts/plan_metrics.py`.
- Comparison: this matches the "normal execution assignment" references. It is
  one mechanism, built on an existing reference implementation, in two plugins
  that each need their own validation, output parsing, and cancellation. The
  length comes from per-harness evidence and exact fake-script specifications
  that the tests depend on, not from repetition or code dumps. "Current system"
  is the most removable section, but it is short orientation. The optional
  split into OpenCode and Pi that the plan offers is available but not
  required.

### Handoff

Make the wording fixes in findings 1 and 2. No other decisions are open.
Limitations: Linux was not examined. Pi behavior was read from the installed
1.4.0 source, not run.

### Execution decision

- Author: coordinator, Hypercube session 12efdd39 (Claude Code, Opus 5.5);
  native conversation `db73ed8b-a2fe-4d6b-8cfc-a41de1a7f3a3`.
- Finding 1: resolved by the coordinator in the plan. The Pi message ends
  "choose a model that no Pi extension provides".
- Finding 2: resolved by the coordinator in the plan. The fake fails when
  `$FAKE_PRINT_FAIL` is non-empty, and each failure case calls `fakePi` again
  so `$FAKE_ARGS.print` is in a fresh `t.TempDir()`.
- Both are wording corrections with no design change, so the coordinator made
  them instead of the planner.
- Final plan blob: `50ce0817f232f085ae7091b6385ed08d87119990`. Guidance:
  `.agents/research/utility-model.md` `7b1da4f39f4acb933c21d10304224bbd7855d6e9`.
- Records are uncommitted by user instruction.
- Decision: proceed.
