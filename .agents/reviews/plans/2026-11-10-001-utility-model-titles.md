# Plan review: utility model and automatic session titles

## Round 1

- Task ID: 2026-11-10-001-utility-model-titles
- Mode: plan
- Author: plan reviewer, Hypercube child session `plan-reviewer` of 12efdd39
  (Claude Code 2.1.319, Opus 5.5, effort high); native conversation
  `a3c1bc09-a8b9-44fa-8158-3a78b1a82eb1`. The coordinator does not expose
  this child's own Hypercube session ID to it. Independent of the planner
  (`9702b77b-3288-4ff3-9270-382b4430ab2e`).
- Plan: `.agents/plans/2026-11-10-001-utility-model-titles.md`
  (repository-relative), blob `f492cd660f6d27dfecb07ad9ae3d4acc6d99a571`,
  uncommitted.
- Guidance blobs: `.agents/research/utility-model.md`, blob
  `7b1da4f39f4acb933c21d10304224bbd7855d6e9` (staged, uncommitted).
- Source HEAD: `d14fceecb81e427cc04d3cbf2831151bb7e460f2`
- Request: slice 1 of GitHub issue #66, with the design points the
  coordinator recorded as agreed with the user.

### Verdict

- Work size: right-sized, at the upper end of a normal assignment.
- Detail: appropriate overall, with two gaps: one failure-path decision and
  two test oracles that cannot be built as written.
- Readiness: corrections required before execution.
- Primary action: `resolve-decisions`. Secondary: none.

### Findings

1. **Claude's non-JSON stdout is dropped, which loses the failure reason
   (required).** The Claude algorithm says "Ignore other lines", and with no
   `result` it returns `Read`'s exit reason, which carries only stderr's tail.
   The research shows Claude writes its failures on stdout with empty stderr
   ("Not logged in · Please run /login", `API Error: Connection refused`).
   The plan's live check covered only success, so it is unverified whether
   stream-JSON mode wraps these failures in a `result` line. The existing
   reader treats an undecodable line as a reportable event
   (`plugins/claude/main.go:160`, `c.line(line)`), so the code expects
   plain-text lines to appear. If they do appear, the log says only
   `claude exited: exit status 1`, and failures are reported only in that
   log. Correction: keep the last undecodable stdout line, and when no
   `result` arrives, return it joined with `Read`'s exit reason. Add a
   `generate_test.go` case where the fake prints a plain line and exits 1,
   and check that the error contains that line.

2. **Two test oracles cannot be built as written (required).**
   - "after `Rename`, applying a generated title leaves the user's title": the
     stand-in answers immediately and `!utility-hold` never answers, so no
     ordering puts a Rename between the call starting and its write. Correction:
     name the apply step, for example `Runtime.applyTitle(id, answer string) error`,
     and have the goroutine call it. Test it directly: create a session with no
     utility selection, `Rename` it, call `applyTitle`, then check that the title
     is the user's and `Renamed` is saved. Also check that `applyTitle` on a
     session that was not renamed sets the generated title, and that a deleted
     session returns nil.
   - "an effort `initializedLine` does not offer fails … and no prompt is sent":
     `fakeClaude` (`plugins/claude/main_test.go:554`) records only argv, so it
     cannot show that no user message was sent. Correction: have the fake append
     each stdin line to `"$FAKE_ARGS.stdin"`, and check that no line contains
     `"type":"user"`. Use `haiku` with `high` (the fixture offers `low,max`).

3. **The Settings row repeats its group heading (minor).** "a third
   `Group title="Utility model"` holding the `SettingsReview` model row … `label="Utility model"`".
   The Review row brings its own visible heading
   (`settings-preferences.tsx:95`, `<span>Review model</span>`). Copying it
   inside a group titled "Utility model" shows the same words twice, and
   `~/.claude/CLAUDE.md` asks for text only where it adds information.
   Correction: either put the group heading over the `ModelSelect`, **Clear**,
   and help text with no inner heading (keeping `label` for the accessible
   name), or add the row to an existing group with its own heading. Say which.

4. **Accuracy fixes (minor, no behavior change).**
   - "children without a brief keep the parent's title": a child always has
     the title its parent passed to `create_session` (`prepareChild` requires
     one, `child_options.go:39`). Say "keep the title they were created with".
   - The reason given for the JSON `Generation` envelope ("Claude prints errors
     on stdout … so only a deliberate encoding … counts") is about Claude's
     stdout, which the plugin consumes and never passes through. The real
     reason is that every one-shot command writes one JSON value, and
     `harnessclient.run` decodes it. The envelope still carries plain text, so
     it fits the agreed "plain text out", and the issue contrasts plain text
     with structured output. Keep the envelope and correct the reason.
   - Add `generate` to the usage line in `plugin.Main`
     (`plugins/internal/plugin/main.go:36`).

### Stream-JSON judgment

Justified. In one Claude process, the `initialize` reply goes through the
existing `check`, so an effort Claude does not offer fails before any model
call. Plain `-p` instead warns on stderr, exits 0, and uses the default
effort. The `result` line's `subtype` and `is_error` settle success whatever
the exit code, which also covers the SIGINT case that exits 0 with an empty
answer. The prompt still goes in on stdin, and every agreed restriction flag is
kept. It copies `models` (`plugins/claude/discovery.go:80`), which already runs
`claude -p --input-format stream-json --output-format stream-json --verbose`
and sends `initialize`. `claude --help` (2.1.319) lists every flag in the plan's
command. The cost is a small line handler instead of reading stdout whole, and
finding 1 covers its one gap.

### Claims checked against the code

- **Session creation paths.** There are only two `AddSession` call sites.
  `Runtime.create` (`sessions.go:80`) serves `Create` and `CreateSubmission`.
  Its callers are the browser's new session (`ui/actions.go:125`) and pull
  request review sessions (`ui/actions.go:219`), which will also get utility
  titles, matching "every new session". `Runtime.createChild` (`children.go:50`)
  serves `CreateChild` (MCP `create_session`), `StartOffer`, and `StartProposal`.
  The brief is `options.Message`, and forks go through the same path. Placing
  `nameSession` after the save in `create` and after `r.launch` in
  `createChild` matches the code's lifetimes. `create` keeps a session whose
  first input fails, and `createChild` deletes one whose start fails.
- **`set_title` locations.** The tool and `setTitleArgs` are in
  `internal/mcp/mcp.go:23,67-75`, and `TestSetTitle` is at
  `mcp_test.go:425-447`. The instruction is at `report/report.go:89`, and the
  expected string at `report_test.go:49`. The transcript title entry is at
  `transcript-tree.ts:45` (and its test, line 55), which the plan correctly
  keeps. No user docs mention the tool.
- **Store migration.** `addColumn` calls in `Open` (`store.go:202-207`) and
  `reopen_test.go` exist as the plan describes. Every `SaveSession` caller
  reads the row inside the same transaction, so the utility write cannot be
  overwritten by a stale copy, and the plan's concurrency argument holds.
- **`harnessclient.run`** (`discovery.go:106`): SIGTERM on cancel, process
  group killed after `cancelGrace` (15 s), stderr's last line as the error with
  the plugin prefix removed, and stdout decoded as JSON. These match the plan.
  The Claude plugin's own `Stop` takes up to 3 s, so Quit is delayed at most a
  few seconds.
- **Quit and Detach** (`sessions.go:247-285`) both set `closing` under `mu`.
  Detach waits on each `l.done`. Cancelling under `mu` after `closing` is set,
  then waiting on the group, closes the race against new calls, because
  `nameSession` checks `closing` and adds to the group under the same lock.
- **Fixtures.** `newFixture` isolates `XDG_CONFIG_HOME` and
  `HYPERCUBE_TEST_LOGS`, and `Cleanup(rt.Quit)` waits for pending calls. The
  stand-in matches keywords as `"!"+word` substrings, so `!utility-fail` and
  `!utility-hold` trigger no agent keyword. It does not handle SIGTERM, so hold
  ends at once. `fakeClaude` has `FAKE_ARGS`, `FAKE_FAIL` (stderr
  "resume failed", exit 1), and `initializedLine` as cited. Also,
  `TestExampleConfig` (`config_test.go:86`) requires
  `docs/examples/config.json` to match byte for byte what `Save` writes, so
  the example's `utility_selection` must sit where the struct field sits. The
  test will catch a mismatch, so this needs no plan change.

### Normalization

- Active target: 600–1,500 words (experimental default; there is no curated
  reference set).
- Measured with `plan_metrics.py`: 2,125 words, 119 of them in fenced code,
  above the band.
- Comparison: the plan is two coupled mechanisms, a one-shot plugin command
  and the titles that use it. Each layer copies an existing pattern
  (`models`, `ReviewSelection`, `addColumn`). It compares to the "one operator
  family through parser, checker, lowering, backend" reference: several layers
  carrying one feature. The titles cannot be checked end to end without
  `generate`, and the stand-in covers that dependency inside one assignment.
  The planner's offered split is reasonable but not required.
- The excess length is mostly settled lifecycle, concurrency, and failure
  decisions plus their tests. The Handoff's self-assessment paragraph and the
  live-check narrative could be cut by about 150 words, but that is not a
  finding. Confidence is moderate: the band is provisional.

### Handoff

To pass: settle finding 1 (keep the last undecodable stdout line as the reason
when no `result` arrives, with its test). Name the apply helper and the two
oracles in finding 2. Choose the Settings layout in finding 3. Fix the wording
in finding 4. The parent can check these corrections without another
independent round.

### Checks

- Run: `git hash-object` on the plan and research note, `plan_metrics.py`,
  `claude --version` and `claude --help` (flag presence only, no model call),
  `gh issue view 66`, and code reading listed above.
- Not run: no live model call; the plan's 2026-11-10 live check is relied on
  for the success path. No builds or tests.
- Limitations: whether Claude writes non-JSON lines to stdout on failure in
  stream-JSON mode is unverified (finding 1 makes the plan correct either way).
  Linux was not considered beyond the code's existing process-group handling.

### Execution decision

- Author: coordinator, Hypercube session 12efdd39 (Claude Code, Opus 5.5);
  native conversation `db73ed8b-a2fe-4d6b-8cfc-a41de1a7f3a3`.
- Finding 1: resolved. The Claude algorithm keeps the last line that does not
  decode as JSON and, with no `result`, returns it joined with `Read`'s exit
  reason; `generate_test.go` gains the plain-line-and-exit-1 case.
- Finding 2: resolved. `Runtime.applyTitle(id, answer string) error` is named
  and tested directly (renamed, not renamed, deleted). `fakeClaude` records
  stdin to `"$FAKE_ARGS.stdin"`; the `haiku`/`high` case checks no user line.
- Finding 3: resolved. The group heading "Utility model" sits over the
  `ModelSelect`, **Clear**, and help text with no inner visible heading.
- Finding 4: resolved. Child wording, envelope reason, and usage line fixed.
- Final plan blob: `07f3b45a2631b762f0f89f79e97f1ea959481cd9`. Guidance blob
  unchanged: `.agents/research/utility-model.md`
  `7b1da4f39f4acb933c21d10304224bbd7855d6e9`.
- Records are uncommitted by user instruction: only the coordinator commits,
  once, at the end of the cycle.
- Decision: proceed.
