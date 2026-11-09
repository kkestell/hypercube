# Work review: 2026-11-09-004-github-workspace-header

## Round 1

- Task ID: `2026-11-09-004-github-workspace-header`
- Mode: `work`
- Author: reviewer `plan-reviewer`, Hypercube child of `1771c79b`, Claude Code,
  native conversation `7989b8d0-1436-4b4f-8ad3-10b45e003508`, model
  `claude-opus-5-5`. Independent of the implementer
  (`81a3d88d-7eb7-40c7-84c3-9135cbf43834`). This reviewer wrote the plan review,
  not the plan or code.
- Work log (repository-relative): `.agents/work/2026-11-09-004-github-workspace-header.md`
  (committed in `a0897b39f147830709e8600d752f371198755f43`)
- Plan: `.agents/plans/2026-11-09-004-github-workspace-header.md`, blob
  `41738c246985820fde77063b7e01846492ab702a` (matches the execution decision)
- Guidance: `.agents/designs/2026-11-09-001-github-integration.md`, blob
  `1be166e4fffb44898b53d6c18ad9f4fb89bd4152`
- Code base: `444646518183624627d89d802a4f9819a1db4aab` (ancestor of head, verified)
- Code head: `d18dd2ce0f2c981596d8ce91844a7e0d80d077c0`
- Checkout: HEAD `a0897b39f147830709e8600d752f371198755f43`; the only change
  after Code head is the work log. Working tree clean.

### Verdict

`changes-required`, for two small text defects. Everything else conforms to the
plan, consumes 003's shapes as committed, and follows the existing component
patterns.

### Findings

1. **The error line runs two sentences together (minor, required).**
   `internal/ui/web/github/github-header.tsx` renders
   `Could not update from GitHub: {first line}` and then ` Showing data from
   {time}.` in one span. The watcher's errors end without a period, for
   example `GitHub answered 500 Internal Server Error` or Git's
   `git fetch origin: fatal: ...`. The page then reads
   `Could not update from GitHub: GitHub answered 500 Internal Server Error Showing data from 12:30.`
   Correction: end the error sentence with a period before the age, or put
   `Showing data from {time}.` on its own line, as the plan's "each fact on its
   own line" suggests. Update the test to assert the full sentence.

2. **The docs overstate when the chooser appears (minor, required).**
   `docs/interface.md` says "When several accounts can push, or none can, the
   dashboard shows a chooser". With one signed-in account that cannot push, the
   watcher uses that account and shows no chooser (002's "none can push and one
   login: used"). A contributor without push rights would expect a chooser
   that never appears. Correction: "When several signed-in accounts can push,
   or none can and several are signed in, ...", or more simply "When more than
   one signed-in account qualifies, ...".

### Independent checks

- **Shapes.** `github/types.ts` mirrors the committed Go `Status` and
  `PullRequest` JSON tags exactly. Optional fields are optional in both:
  `choices`, `upstream`, and `error`. `Workspace.github` and
  `Session.pullRequest` are optional, matching `omitempty`.
- **Absent states.**
  - With no gh, or a non-GitHub workspace: there is no `github`, so
    `GitHubHeader` returns null and the heading keeps `mb-6`, as before.
  - Pending choice (`choices` set, `account` empty): the Radix `Select` shows
    the `Choose an account` placeholder.
  - Account set without choices: `Using {account}.`
  - Error: first line only, with the age only when `updated > 0`.
  - An empty `account` without `choices` cannot occur under 002's contract.
  Tests cover each reachable case, and the Radix-driven choice is tested
  through `chooseAccount`, as the execution decision allows.
- **Conventions.**
  - `PullRequestIcon` copies `BranchIcon`'s structure (title on the span, and
    `role="img"` and `aria-label` through `Icon`).
  - The icons are registered in `shared/icon.tsx`.
  - `SelectTrigger`'s `size="sm"` exists in `components/ui/select.tsx`.
  - `post` is used as other actions use it.
  - The session row's title stops reaching the edge when either icon shows.
    No CSS selector targets `session-item-branch`, so the new
    `session-item-pull-request` marker needs no style.
- **Wording.** `trackingText`, `pullRequestFacts`, and `pullRequestLook`
  match the plan's sentences, singular and plural forms, and priority. Table
  tests cover each branch.

### Verification

- `reused`: `make check` at Code head; output in `/tmp/check004.out`, written
  the same minute as the commit. No `FAIL`; browser bun tests 415 pass, 0 fail;
  desktop 28 pass. `bunx tsc -p .` ran inside it, with no errors.
- No new runs.

### Limitations

I can't judge the rendered layout or spacing from the code and happy-dom
tests. I haven't seen the header, the chooser's open menu, or the row icons
in a browser. The user judges the look; no screenshots exist.
