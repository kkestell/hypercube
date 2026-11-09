# Review: Review settings, the model and prompt the Review button uses

- Task ID: 2026-11-09-010-review-settings
- Work log (repository-relative): `.agents/work/2026-11-09-010-review-settings.md`
- Plan (repository-relative): `.agents/plans/2026-11-09-010-review-settings.md`, blob `6c6c9118708a572da8a1e64ae7b70726c6dde34b` (matches the log)

## Round 1

- Author: xreview worker p3-review (Hypercube child session of 777ef702), Claude Code, claude-opus-5-5, native conversation ID `3f9675be-5e2c-4ca3-bd35-c8037a5f2d59`
- Code base: d2d3d0fc1462a5dcd95a4a0126fd73d3e3a12a21
- Reviewed Code head: 8a2b392f01ef7a843ffff7772764833d642f863d (single implementation commit; base is its parent)
- Checkout: `/Users/kyle/projects/hypercube/.hypercube/777ef702` at 77d9e3c (Code head plus the work-log commit only), clean status, no untracked files
- Verdict: **accept**. The request and plan are implemented as specified, with tests at the config, server and browser layers. No findings.
- Independence: I did not write or plan this change. I read the full diff, the touched files, and the callers of the changed contracts: `config.Load`/`Validate`/`Save`, the config endpoint (`internal/ui/config.go`), the model-list migration that saves config (`internal/store/model_entries.go`), `startReview`, `saveConfig` in `api.ts`, `ModelSelect`, and the Settings dialog's draft and save flow.

### Areas the parent asked about

- **Config files without the new keys**: `Load` decodes over `defaults()`, which now sets `ReviewPrompt` to `DefaultReviewPrompt`, so a missing `review_prompt` keeps the default and a missing `review_selection` stays nil. `TestLoadDefaults` (no file) and `TestLoadPreservesConfig` ("full" and "partial" files without the keys) assert exactly that. The only non-test path that saves a config, `moveModelEntries`, starts from `config.Load()`, so it carries the default prompt and passes the new required-prompt check.
- **Clearing the review model**: **Use default model** calls `onChange({review_selection: undefined})`; `saveConfig` sends `JSON.stringify(draft)`, which drops `undefined` properties; the server decodes the absent key as a nil pointer, and `omitempty` keeps it out of the written file. Confirmed with a throwaway test (below): after the click the button disappears and the POST body has no `review_selection`.
- **Prompt validation**: `Validate` trims the prompt and refuses an empty result, with no placeholder requirement, as the plan says. `TestSave` covers a whitespace-only prompt and an invalid review selection. The trim also applies on `Load`, which is harmless. `reviewPrompt` uses a single-pass `strings.NewReplacer`, so a URL containing `{number}` is not substituted again; `TestReviewPrompt` checks repeated placeholders and an unknown `{other}`.
- **Docs**: `docs/configuration.md` § Pull request reviews documents both keys, the default, the placeholders, the fallback and the error, consistent with `reviewSettings`. The `interface.md#github` anchor exists, and `interface.md` now links there instead of repeating the rule. Adding `review_selection` and `review_prompt` to `docs/examples/config.json` is required by `TestExampleConfig` for the prompt and matches configuration.md's "shows every field" for the selection. No screenshot shows the Settings section list.

### Notes (informational)

- No committed test covers **Use default model**. The behavior is correct (above), and even a regression to `null` would decode to nil on the server, so I did not raise it as a finding.
- Because `review_prompt` has no `omitempty`, any Settings save writes the current prompt into `config.json`. A later change to `DefaultReviewPrompt` then does not reach users who have saved Settings. Other defaulted fields (`theme`, `terminal_font_size`) behave the same way, and a reset control is a plan non-goal.

### Verification

- Reused: `make check` from the repository root at Code head 8a2b392f…, exit 0, `/tmp/p3-make-check.log` (all Go packages ok incl. `internal/config`, `internal/ui`, `internal/session`; bun 449 and 28 pass, 0 fail). The log's tree was the committed tree (status clean after commit, no hooks), so it applies to the reviewed head.
- Run: a throwaway browser test outside the repository (`/tmp/p3-review-scratch/clear.test.ts`, run with `bun test` from `internal/ui` at the reviewed head): opens Settings › Review with a saved `review_selection`, clicks **Use default model**, saves, and asserts the POST body lacks `review_selection`. 1 pass. Run because the parent asked about clearing and no committed test covers it.
