# Review settings: the model and prompt the Review button uses

- Task ID: 2026-11-09-010-review-settings
- Status: ready
- Source HEAD: 3233f31167e9f44a3f470fca3bca753f94a24050
- Author: parent coordinator, Hypercube session 777ef702 (Claude Code, Opus 5.5)
- Dependencies: 2026-11-09-009-review-starts-a-worktree-session (complete),
  which added the `"review"` session action with `reviewSettings` and a fixed
  prompt in `internal/ui/actions.go`. Phase 3 of 3.

## Outcome and boundaries

A new **Review** page in Settings chooses the model and prompt that
**Review** starts a session with. Both are saved in `config.json`.

- `review_selection` (optional settings: harness, model, effort). When unset,
  the review uses the existing rule: `default_selection`, else the first listed
  model at its default effort, else the existing error.
- `review_prompt` (string). `{number}` and `{url}` in it are replaced with the
  pull request's number and link. Its default is phase 2's prompt written with
  those placeholders:
  `Review pull request #{number} ({url}). Its head commit is checked out in your
  working directory. Read its description, discussion, and diff with gh, and
  report what you find. Post nothing to GitHub.`

Non-goals: per-workspace or per-repository review settings; other
placeholders; a reset-to-default control.

## Current system

- `internal/config/config.go`: `Config` (JSON document), `defaults()`, `Load`,
  `Validate` (checks `DefaultSelection` with `harness.Settings.Check`), `Save`.
  `internal/ui/config.go` serves and saves the whole document with
  `DisallowUnknownFields`, so the browser must send every field.
- `internal/ui/actions.go`: `startReview` builds the prompt with
  `fmt.Sprintf`; `reviewSettings(cfg)` holds the model rule; `review_test.go`
  covers it.
- Browser: `Config` type and `sections` list (`internal/ui/web/app/types.ts`);
  `settings-dialog.tsx` renders a section per name with a shared draft and one
  Save; `settings-preferences.tsx` has `Group`, `Field`, `DefaultSelection`
  (a `ModelSelect` over the draft's models) and the child-guidance `Textarea`
  to copy from.
- Docs: `docs/configuration.md` documents config keys and Settings pages.

## Chosen design and decisions

### Config

- `Config.ReviewSelection *harness.Settings` (`json:"review_selection,omitempty"`)
  and `Config.ReviewPrompt string` (`json:"review_prompt"`).
- `defaults()` sets `ReviewPrompt` to the default prompt above (an exported
  `DefaultReviewPrompt` constant), so a config file without the key keeps the
  default; no migration is needed.
- `Validate` checks `ReviewSelection` like `DefaultSelection`, trims
  `ReviewPrompt`, and refuses an empty prompt ("review prompt is required").
  Placeholders are optional.

### Use

- `reviewSettings(cfg)` returns `*cfg.ReviewSelection` when set, then the
  existing rule.
- `startReview` builds the prompt with
  `strings.NewReplacer("{number}", strconv.Itoa(number), "{url}", url).Replace(cfg.ReviewPrompt)`.
  Move the default text out of `actions.go`.

### Settings page

- Add `'Review'` to `sections` after `'Agents'`; add `review_selection?` and
  `review_prompt` to the `Config` type.
- `SettingsReview({draft, onChange})` in `settings-preferences.tsx`, rendered by
  `settings-dialog.tsx` for the `'Review'` section:
  - **Review model**: `ModelSelect` over `draft.models` with
    `value={draft.review_selection ?? null}`; help text: "Review starts a new
    session with this model. Without one, it uses the default model." While
    one is set, a **Use default model** ghost button beside it sets
    `review_selection` to `undefined` (omitted from the saved JSON).
  - **Review prompt**: `Textarea` (`rows={8}`) bound to `review_prompt`; help
    text names the placeholders: "`{number}` becomes the pull request's number
    and `{url}` its link. The pull request's head is checked out in the
    session's worktree."
- Saving uses the existing dialog Save; server validation errors show in the
  dialog's existing error line.

### Docs

`docs/configuration.md`: describe **Settings › Review**, both keys, the model
fallback, and the placeholders. `docs/interface.md`'s Review paragraph: say the
model and prompt come from **Settings › Review** instead of describing the
fixed defaults.

## Implementation sequence

1. `internal/config/config.go` fields, default, validation.
2. `internal/ui/actions.go`: `reviewSettings` and prompt substitution.
3. Browser types, section, `SettingsReview`, dialog wiring; update test
   fixtures that build a full `Config`.
4. Docs.

## Verification

- `internal/config` (`config_test.go`): a file without the keys loads the
  default prompt and no review selection; an empty or whitespace prompt is
  refused; an invalid review selection is refused; a saved document round-trips
  both keys.
- `internal/ui` (`review_test.go`): `reviewSettings` prefers
  `ReviewSelection`, then falls back as before; the prompt substitution
  replaces every `{number}` and `{url}` (test the helper the handler uses, so
  extract the substitution into a small function).
- Browser (`settings-dialog.test.tsx`): the Review section shows the saved
  prompt; editing the prompt and choosing a model then saving sends
  `review_prompt` and `review_selection` in the saved document.
- `make check` from the repository root.

## Handoff

Ready. Two config keys wired through validation, one handler, and one Settings
page, with tests at each layer — a normal, small assignment close to the
brief threshold, kept as a plan because it spans config, server, browser and
docs.
