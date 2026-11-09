# Work log: Review settings, the model and prompt the Review button uses

- Task ID: 2026-11-09-010-review-settings
- Plan (repository-relative): `.agents/plans/2026-11-09-010-review-settings.md`, blob `6c6c9118708a572da8a1e64ae7b70726c6dde34b`; no guidance blobs. Dependency read: `.agents/work/2026-11-09-009-review-starts-a-worktree-session.md`.
- Author: xwork worker p3-work (Hypercube child session of 777ef702), Claude Code, claude-opus-5-5
- Status: complete
- Code base: d2d3d0fc1462a5dcd95a4a0126fd73d3e3a12a21
- Code head: 8a2b392f01ef7a843ffff7772764833d642f863d
- Implementation commits: 8a2b392f01ef7a843ffff7772764833d642f863d (work)

## Changes

Responsibilities as in the plan.

- `internal/config/config.go`: `ReviewSelection`, `ReviewPrompt`, exported `DefaultReviewPrompt` set by `defaults()`; `Validate` checks the selection, trims the prompt, and refuses an empty one.
- `internal/ui/actions.go`: `reviewSettings` prefers `ReviewSelection`; `reviewPrompt(prompt, number, url)` does the `strings.NewReplacer` substitution and `startReview` uses it. The fixed prompt text is gone from this file.
- Browser: `types.ts` (`review_selection?`, `review_prompt`, `'Review'` after `'Agents'`), `SettingsReview` in `settings-preferences.tsx`, wired in `settings-dialog.tsx`; `test-state.ts` `testConfig` carries a `review_prompt`.
- Docs: `docs/configuration.md` gains **Pull request reviews** (Settings › Review, both keys, the fallback, the placeholders); `docs/interface.md`'s Review paragraph points there.
- Tests: `config_test.go` (`TestSave` round-trips both keys and refuses a whitespace prompt and an invalid review selection; `TestLoadDefaults` and `TestLoadPreservesConfig` expect the default prompt and no selection from files without the keys), `review_test.go` (`reviewSettings` prefers the review selection; `TestReviewPrompt` replaces every `{number}` and `{url}` and leaves other braces), `settings-dialog.test.tsx` (the Review page shows the saved prompt; editing it and choosing a model saves both).

## Deviations

- `docs/examples/config.json` gains `review_selection` and `review_prompt`. `TestExampleConfig` requires the example to equal the saved format, and `review_prompt` is always written.
- Config literals passed to `Save` or the config endpoint in other tests (`internal/session/selection_test.go`, `internal/ui/config_test.go`) now include a review prompt, since an empty one is refused. Invalid-config cases in `TestSave` carry a prompt so each still fails for its own reason.

## Verification (at Code head 8a2b392f…, macOS, go 1.27.3, bun 1.4.2)

- During work: `go test ./internal/config ./internal/ui ./internal/session`; `bunx tsc -p .` and `bun test web/app/settings-dialog.test.tsx` in `internal/ui`: pass.
- `make check` from the repository root on the tree then committed unchanged (no hooks): exit 0 (bun 449 pass and 28 pass, 0 fail; all Go packages ok; `go vet` clean). Output in `/tmp/p3-make-check.log`. Status was clean after the commit.
- Browser change checked with tests and the build only, per the brief; no screenshots.

## Session provenance

| Stage | Name / ID | Harness | Native conversation ID | Model |
| --- | --- | --- | --- | --- |
| parent | 777ef702 (Hypercube session) | Claude Code | 209b0949-92b0-42bc-8596-4341a4bf0785 | Opus 5.5 |
| work | p3-work (child of 777ef702) | Claude Code | cdd9e886-9ec2-4366-9547-7ec44f1d48be (`CLAUDE_CODE_SESSION_ID`) | claude-opus-5-5 |
| review | p3-review (child of 777ef702) | Claude Code | 3f9675be-5e2c-4ca3-bd35-c8037a5f2d59 | claude-opus-5-5 |

## Limitations

- Awaiting independent review.

## Completion

- Review: `.agents/reviews/2026-11-09-010-review-settings.md`, round 1 `accept` at Code head 8a2b392f01ef7a843ffff7772764833d642f863d.
- Final reviewed range: d2d3d0fc1462a5dcd95a4a0126fd73d3e3a12a21..8a2b392f01ef7a843ffff7772764833d642f863d; later commits change workflow records only.
- Checks: `make check` exit 0 at the final Code head (reused, `/tmp/p3-make-check.log`).
