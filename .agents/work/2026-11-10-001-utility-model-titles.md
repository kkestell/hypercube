# Work log: utility model and automatic session titles

- Task ID: 2026-11-10-001-utility-model-titles
- Plan: `.agents/plans/2026-11-10-001-utility-model-titles.md` (repository-relative), blob `07f3b45a2631b762f0f89f79e97f1ea959481cd9` (verified unchanged before starting)
- Guidance: `.agents/research/utility-model.md`, blob `7b1da4f39f4acb933c21d10304224bbd7855d6e9` (verified)
- Plan review: `.agents/reviews/plans/2026-11-10-001-utility-model-titles.md` (round 1 plus execution decision: proceed)
- Author: implementer, Hypercube child session of 12efdd39 (own ID unavailable) (Claude Code, Sonnet 5.5); native conversation `458c0415-bad7-44db-ad37-2ed81df27abe`
- Status: complete
- Code base: `d14fceecb81e427cc04d3cbf2831151bb7e460f2`
- Code head: uncommitted working tree on the base. The user ruled that the coordinator makes one commit at the end of the cycle, so this stage made no commits and staged or unstaged nothing. Commit list: none.

## Changes by responsibility

- Protocol: `internal/harness/discovery.go` (`GenerateRequest`, `Generation`, three one-shot commands); `plugins/internal/plugin/main.go` (`Plugin.Generate`, `run("generate")`, usage line); `internal/harnessclient/discovery.go` (`Generate`, the single empty-answer check).
- Claude plugin: `plugins/claude/generate.go`, wired in `plugins/claude/main.go`.
- Titles: `store.Session.Renamed` and its column (`internal/store/store.go`); `Config.UtilitySelection` (`internal/config/config.go`); `report.TitlePrompt` and `report.GeneratedTitle`; `internal/session/titles.go` (`nameSession`, `applyTitle`, cancel and wait helpers); runtime fields in `runtime.go`; calls in `create` and `createChild`; `Rename` sets `Renamed`; `Quit` and `Detach` cancel and wait.
- `set_title` removed: `internal/mcp/mcp.go`, the instruction in `report.Instructions`. The browser's `set_title` title entry stays, as planned.
- Stand-in `generate` in `internal/harness/harnesstest/harnesstest.go` (`UtilityTitle`, `!utility-fail`, `!utility-hold`).
- Browser: `utility_selection` in `app/types.ts`; "Utility model" group in `SettingsGeneral`.
- Docs: `docs/configuration.md`, `docs/examples/config.json`, `docs/usage.md`, `docs/dev/architecture.md`, `docs/dev/glossary.md`.

## Changed and new paths (`git status` accounts for all of them)

Modified:
`docs/configuration.md`, `docs/dev/architecture.md`, `docs/dev/glossary.md`, `docs/examples/config.json`, `docs/usage.md`,
`internal/config/config.go`, `internal/config/config_test.go`,
`internal/harness/discovery.go`, `internal/harness/harnesstest/harnesstest.go`,
`internal/harnessclient/discovery.go`, `internal/harnessclient/discovery_test.go`,
`internal/mcp/mcp.go`, `internal/mcp/mcp_test.go`,
`internal/report/report.go`, `internal/report/report_test.go`,
`internal/session/children.go`, `internal/session/runtime.go`, `internal/session/sessions.go`,
`internal/store/reopen_test.go`, `internal/store/store.go`,
`internal/ui/web/app/settings-dialog.test.tsx`, `internal/ui/web/app/settings-preferences.tsx`, `internal/ui/web/app/types.ts`,
`plugins/claude/main.go`, `plugins/claude/main_test.go`, `plugins/internal/plugin/main.go`.

New:
`internal/session/titles.go`, `internal/session/titles_test.go`,
`plugins/claude/generate.go`, `plugins/claude/generate_test.go`,
`plugins/internal/plugin/main_test.go`,
`.agents/work/2026-11-10-001-utility-model-titles.md` (this log).

Pre-existing task records left untouched: `.agents/research/utility-model.md` (staged), the plan, and the plan review.

## Deviations from the plan

None in design. Routine details:

- `GeneratedTitle` also strips curly quotes.
- `plugins/internal/plugin/main_test.go` is a new file for the nil-`Generate` test.
- The browser test for **Clear** is its own test; it starts from a config that already has `utility_selection`, because reopening Settings reloads the saved config.
- `TestSessionBoundaries` in `internal/mcp/mcp_test.go` counts 7 top-level tools instead of 8.

## Verification (all against the uncommitted working tree on `d14fcee`; macOS, Claude Code 2.1.319)

1. `make check` (run): exit 0. Includes `bun test` (464 tests incl. the two new settings tests), `tsc`, `go vet`, gofmt check, and `go test -race ./...`, all packages `ok`, including `internal/session`, `internal/harnessclient`, `plugins/claude`, `plugins/internal/plugin`, `internal/report`, `internal/config`, `internal/store`, `internal/mcp`. Full output was written to `/tmp/check.log`, not kept in the repository.
2. Live `generate` (run). The plan names `go run ./plugins/claude generate`. I built the same package (`go build -o /tmp/gen/hypercube-harness-claude ./plugins/claude`) and ran the binary with the same arguments:

```
$ echo '{"model":"haiku","effort":"low","prompt":"Write a title for: fix the flaky parser test"}' | hypercube-harness-claude generate
{"text":"Fix flaky parser test"}          (exit 0)

$ echo '{"model":"haiku","effort":"minimal","prompt":"x"}' | hypercube-harness-claude generate
Warning: Unknown --effort value 'minimal' — ignoring it and using the default effort. Valid values: low, medium, high, xhigh, max.
hypercube-harness-claude: Claude Code model haiku does not accept effort "minimal"; see list_models      (exit 1)
```

   The warning line comes from claude at launch; the plugin then refuses before sending any prompt.
3. `make dev` session (run, without user interaction): `scripts/dev up` started the dev backend on 4281 with the isolated state under `~/projects/hypercube-dev`. Through its HTTP API I saved `utility_selection` claude/haiku/low (`POST /config`, 204) and created a session (`POST /session?action=new`) with the text "Please fix the flaky parser test in the lexer package so it stops failing on CI". Reading `sessions` in the dev database, the title became `Fix flaky lexer parser test on CI` (`renamed` 0) within 3 seconds and stayed. I deleted the session, removed `utility_selection` from the dev config, and stopped the backend (`scripts/dev stop`; port 4281 free). The browser view itself was not driven; the title change was read from the database.
4. Not run live: the Rename-wins-after-generation race and `Quit` with a held call. Both are covered by `internal/session/titles_test.go` (`TestApplyTitle`, `TestQuitCancelsAHeldUtilityCall`).

## Session provenance

| Stage | Coordinator session | Harness | Native conversation | Model |
| --- | --- | --- | --- | --- |
| xwork implementer | unavailable: list_sessions does not show the caller; child of 12efdd39 | Claude Code 2.1.319 | `458c0415-bad7-44db-ad37-2ed81df27abe` | Sonnet 5.5 (`claude-sonnet-5-5`) |

## Outstanding

- Awaiting independent `xreview work` on this log and the whole working-tree diff against `d14fcee`.
- Nothing is committed; the coordinator commits once.

## Repair 1 (work review round 1, `changes-required`)

Code base stays `d14fceecb81e427cc04d3cbf2831151bb7e460f2`; Code head is still the uncommitted working tree. Nothing was committed, staged or unstaged.

Findings addressed (`.agents/reviews/work/2026-11-10-001-utility-model-titles.md`):

1. `plugins/claude/generate.go`: the reading goroutine now owns `failure`, `text`, `got` and `plain` and sends one `outcome` value, including `Read`'s result, over the channel. The result is decided from that value. `mu` and the locking in `fail` are gone, and the `sync` import with them.
2. `internal/session/titles.go`, `sessions.go`: `utilityCancelsLocked` and `stopUtilityCalls` are replaced by `cancelUtilityCallsLocked`. `Quit` and `Detach` call it under `mu` right after `closing` is set, and only `r.utilityCalls.Wait()` runs at the end.

Changed paths in this repair: `plugins/claude/generate.go`, `internal/session/titles.go`, `internal/session/sessions.go`. The path list above is otherwise unchanged.

Verification (run, against the working tree after the repair):
- `go test -race ./plugins/claude ./internal/session`: both `ok` (3.999s and 96.587s).
- `make check`: exit 0. UI `bun test` 465 pass, desktop `bun test` 28 pass, `tsc`, `go vet` and the gofmt check clean, and `go test -race ./...` all `ok`. Output was written to `/tmp/check2.log`.
- The earlier live `generate` and `make dev` results were taken before this repair. The repair changed no protocol or title behavior, and the plugin tests cover the changed function.

## Completion

- Work review: `.agents/reviews/work/2026-11-10-001-utility-model-titles.md`
  (round 1 `changes-required`; coordinator execution decision `accept` after
  Repair 1).
- Plan review: `.agents/reviews/plans/2026-11-10-001-utility-model-titles.md`
  (round 1; coordinator execution decision `proceed`).
- Commits: one coordinator commit holds the code, tests, docs, and records, by
  user instruction. Code base `d14fceecb81e427cc04d3cbf2831151bb7e460f2`.

## Session provenance (all stages)

| Stage | Coordinator session | Harness | Native conversation | Model |
| --- | --- | --- | --- | --- |
| Coordinator | 12efdd39 | Claude Code | `db73ed8b-a2fe-4d6b-8cfc-a41de1a7f3a3` | Opus 5.5 |
| Research | `utility-model-research`, child of 12efdd39 | Pi 1.4.0 | unavailable: not requested at the time | GPT-6.1 Sol, high |
| xplan | `planner`, child of 12efdd39 | Claude Code | `9702b77b-3288-4ff3-9270-382b4430ab2e` | Opus 5.5, high |
| xreview plan | `plan-reviewer`, child of 12efdd39 | Claude Code 2.1.319 | `a3c1bc09-a8b9-44fa-8158-3a78b1a82eb1` | Opus 5.5, high |
| xwork and Repair 1 | `implementer`, child of 12efdd39 | Claude Code 2.1.319 | `458c0415-bad7-44db-ad37-2ed81df27abe` | Sonnet 5.5, medium |
| xreview work | `work-reviewer`, child of 12efdd39 | Claude Code 2.1.319 | `df5e7729-3a7c-4479-8594-3c9686d14d65` | Opus 5.5, high |
