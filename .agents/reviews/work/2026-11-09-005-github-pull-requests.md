# Work review: 2026-11-09-005-github-pull-requests

## Round 1

- Task ID: `2026-11-09-005-github-pull-requests`
- Mode: `work`
- Author: reviewer `plan-reviewer`, Hypercube child of `1771c79b`, Claude Code,
  native conversation `7989b8d0-1436-4b4f-8ad3-10b45e003508`, model
  `claude-opus-5-5`. Independent of the implementer
  (`1fe21930-643c-4558-9309-1420ebd06b1c`). This reviewer wrote the plan review,
  not the plan or code.
- Work log (repository-relative): `.agents/work/2026-11-09-005-github-pull-requests.md`
  (committed in `3ff25108873ce63fe31a22b6564429268e84fbd2`)
- Plan: `.agents/plans/2026-11-09-005-github-pull-requests.md`, blob
  `735790eef7d8c8123f3e1b190d5096ad4abab8e7` (matches the execution decision)
- Guidance: `.agents/designs/2026-11-09-001-github-integration.md`, blob
  `1be166e4fffb44898b53d6c18ad9f4fb89bd4152`
- Code base: `0ab05a68a43a709ce17089250cecb277875360a0` (ancestor of head, verified)
- Code head: `2efde1dbef924b82c8397af8b09df3dacfc7bbc8`
- Checkout: HEAD `3ff25108873ce63fe31a22b6564429268e84fbd2`; the only change
  after Code head is the work log. Working tree clean.

### Verdict

`accept`. No findings.

### Independent checks

- **Plan-review finding 1.** `pull-requests.tsx` `emptyText` checks, in
  order: no account → `Choose an account to see pull requests.`; `updated` 0
  with an error → `Could not load pull requests.`; `updated` 0 without one →
  `Checking GitHub.`; no rows → `No open pull requests.` With 003's
  token-error rule, a failed token read lands on "Could not load", not on a
  permanent "Checking". A table test covers all four.
- **Instruction texts.** `instruction` returns exactly the plan's three
  strings, with number, URL, and head branch. `actionsFor` gives `merge`
  unless the pull request is a draft, `review` on `CHANGES_REQUESTED`, and
  `checks` on `FAILURE`/`ERROR`, in that order. Both are tested by exact string
  and table.
- **Existing send and offer mechanisms.**
  - `sendInstruction` posts the `input` action with `{text, images: []}`, the
    same request as `requestMerge`, then calls `chooseSession`. When the post
    fails, `post` already shows the reason.
  - `startInstruction` composes the existing `openUnsent` and `typed` on
    `unsentKey(workspace)`. `typed` writes `state.drafts[key]`, which
    `draftOf` and the composer read. The user still picks settings and sends
    through the normal composer path.
  - Both paths are tested through the dialog: the request's parameters and
    body, the selected session, and the unsent draft text.
- **Picker.** It flattens top-level sessions and their children, drops
  ended sessions, and puts sessions whose `pullRequest.number` matches first,
  with `New Session` at the top. It uses `CommandDialog` and the dashboard's
  `statusOf` call unchanged.
- **Dashboard.** The list renders between the header and Recent Sessions.
  Recent Sessions' `ul` is labeled, and `app.test.tsx` scopes its `li`
  assertion to it. The overflow link appears only when `open` exceeds the
  rows.
- **The ".." fix.** `github-header.tsx` `sentence` adds a period only when the
  first line lacks one. The new test uses the SSH `(publickey).` case.
- **Whole request across 002–005.**
  1. gh accounts only, absent without gh: 002.
  2. Account from `credential.githubAccount`, else the one pusher, else a
     saved choice; repository from the tracked remote with an `origin`
     fallback; github.com only: 002 and 003, with the chooser in 004.
  3. `GH_TOKEN` in agents and panel shells: 002.
  4. No new MCP tools.
  5. Background fetch: 003.
  6. Header: 004. Pull requests list with review, checks, author, and the
     three actions to a chosen or new session: 005. Session row state: 003
     and 004.
  7. Polling once a minute: 002 and 003.

  Nothing is missing.

Notes, not findings:
- `github-header.tsx` now reads `const time =(ms: number)`; the space
  before `(` was dropped in this range.
- A pull request with no author renders an empty first span in the facts
  line. It is invisible apart from one gap.

### Verification

- `reused`: `make check` at Code head; output in `/tmp/check005.out`, written
  the same minute as the commit. No `FAIL`; browser bun tests 426 pass, 0 fail;
  desktop 28 pass. `bunx tsc -p .` ran inside it, with no errors.
- `reused`: `bun test web/github web/app` (65 pass), from the work log.
- No new runs.

### Limitations

I can't judge the rendered layout from the code and happy-dom tests: row
spacing, button placement, and the dialog's look. No screenshots exist; the
user judges the look. The agents' handling of the instructions, such as
merging with `gh` or pushing fixes, is outside what tests can show.
