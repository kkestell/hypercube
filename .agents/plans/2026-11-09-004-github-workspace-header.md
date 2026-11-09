# Show GitHub status in the workspace dashboard header and on session rows

- Task ID: `2026-11-09-004-github-workspace-header`
- Status: ready
- Source HEAD: `47055e42f036f863ebb18110d3803fbce3d6f38b` (plan written before 002)
- Author: planner `planner`, Claude Code, native conversation
  `1e20740e-b018-49eb-9ae2-27fe8948db63`, `claude-opus-5-5`
- Shared design (repository-relative): `.agents/designs/2026-11-09-001-github-integration.md`,
  [linked here](../designs/2026-11-09-001-github-integration.md); JSON shapes
  are under Shared data shapes.
- Prerequisite: [003](2026-11-09-003-github-status.md) committed (state message
  carries `github` and `pullRequest`; `github-account` action exists).

## Outcome and boundaries

A GitHub workspace's dashboard shows, under its name: the repository linked to
GitHub, the account in use (a chooser when several qualify), the branch's
ahead/behind, and any update error with the age of the data. A session whose
branch has an open pull request shows a pull request icon on its sidebar row.
Workspaces without `github` look exactly as today. The pull requests list is
unit 005. Browser-only, plus docs.

## Current system

- `internal/ui/web/sessions/workspace-dashboard.tsx:WorkspaceDashboard` renders
  `<h1>{workspace.name}</h1>`, then "Recent Sessions".
- `internal/ui/web/sessions/types.ts` defines `Workspace` and `Session` as the
  server sends them; `data-socket.ts` stores them unchanged.
- `internal/ui/web/sidebar/session-item.tsx` shows `BranchIcon` at a worktree
  row's right edge (`shared/branch-icon.tsx`: icon + color + `title`, with
  `role="img"`); `session.worktree` also chooses the title's fade classes.
- `shared/icon.tsx` maps names to Lucide icons; `components/ui/select.tsx` is
  the stock shadcn select (used in `app/settings-preferences.tsx`).
- `api.ts:post(store, path, params)` posts an action and reports failures.
- Links with `target="_blank"` open in the user's browser from the desktop app
  (`desktop/window.js` `setWindowOpenHandler`), as transcript links do.

## Changes

New directory `internal/ui/web/github/` owns GitHub display.

1. **Types** (`github/types.ts`): `GitHubStatus` and `PullRequest` mirroring
   the shared design's JSON; `Workspace.github?: GitHubStatus` and
   `Session.pullRequest?: PullRequest` in `sessions/types.ts`.
2. **Wording** (`github/wording.ts`, pure, tested):
   - `trackingText(status)`: no branch → `No branch is checked out.`; no
     upstream → `{branch} has no upstream branch.`; 0/0 → `{branch} is up to
     date with {upstream}.`; otherwise `{branch} is {a} ahead of {upstream}.`,
     `{branch} is {b} behind {upstream}.`, or `{branch} is {a} ahead of and {b}
     behind {upstream}.`, with "commit"/"commits" after each count (`1 commit
     ahead of`, `2 commits behind`).
   - `pullRequestFacts(pr)`: sentences in this order, each only when it
     applies: `Draft.`; review `Approved.`, `Changes requested.`, `Review
     required.`; checks `Checks passed.`, `Checks failing.` (FAILURE, ERROR),
     `Checks running.` (PENDING, EXPECTED).
   - `pullRequestLook(pr)`: icon and color, first match wins: draft →
     `git-pull-request-draft`, muted; checks FAILURE/ERROR →
     `git-pull-request`, `text-destructive`; CHANGES_REQUESTED →
     `git-pull-request`, amber (`text-amber-600 dark:text-amber-500`, as
     `BranchIcon` uses); APPROVED with checks SUCCESS or none → green
     (`text-green-600 dark:text-green-500`); otherwise muted.
3. **Icon** (`github/pull-request-icon.tsx`): `PullRequestIcon({pr})` renders
   the look with `role="img"` and title `Pull request #N: {title}. {facts}`,
   built like `BranchIcon`. Register `git-pull-request`
   (`GitPullRequest`) and `git-pull-request-draft` (`GitPullRequestDraft`) in
   `shared/icon.tsx`.
4. **Session row**: in `session-item.tsx`, render `PullRequestIcon` before the
   branch icon when `session.pullRequest` is set and the row is not renaming.
   Treat `session.worktree || session.pullRequest` as "has an icon" for the
   title's edge and fade classes. Update the file's header comment.
5. **Header** (`github/github-header.tsx`): `GitHubHeader({workspace})`,
   rendered by `WorkspaceDashboard` directly under the `h1` when
   `workspace.github` is set. Each fact on its own line, as small muted text:
   - The repository: an external link to `url` with text `repo`
     (`target="_blank" rel="noopener noreferrer"`). The installed Lucide has
     no GitHub logo; use no icon.
   - Account: with `choices`, a labeled shadcn `Select` ("GitHub account")
     whose value is `account`, placeholder `Choose an account`, and a line
     `Agents and terminals in this workspace use this account.` Choosing posts
     `workspace` with `{action: 'github-account', id, account}`. Without
     `choices`: `Using {account}.`
   - `trackingText(status)`.
   - With `error`: a warning icon (`text-destructive`) and `Could not update
     from GitHub: {first line of error}`, followed by `Showing data from
     {time}.` when `updated > 0`, with `time` from
     `new Date(updated).toLocaleTimeString([], {hour: 'numeric', minute:
     '2-digit'})`. No timers.

### Documentation

Extend `docs/interface.md` `## GitHub`: what the dashboard header shows,
choosing the account when several can push or none can, and a table of the
session row pull request icon (draft, checks failing, changes requested,
approved, open), whose hover text gives the details.

## Verification

Browser tests with bun in `internal/ui` (`bun test`, `bunx tsc -p .`), using
`test-state.ts` builders and `test-render.tsx`'s `stubFetch`:

- `wording.ts`: table tests for every `trackingText` branch including singular
  counts, `pullRequestFacts` combinations, and `pullRequestLook` priority
  (draft with failing checks is draft; failing checks with changes requested is
  destructive; approved with pending checks is muted).
- Dashboard: without `github`, no header content beyond the name (existing
  dashboard test still passes); with `github`, the repository link has the URL
  and `target="_blank"`, `Using kkestell.` shows, the tracking sentence shows;
  with `choices`, selecting an account posts `action=github-account` with the
  workspace ID and account (if happy-dom cannot open the Radix `Select`, call
  the component's choose handler directly and assert the request); with `error` and `updated`, the error line and
  "Showing data from" show.
- Sidebar: a session with `pullRequest` has an image named `Pull request #12:
  …`; a session without has none.
- `make check` before committing.

## Handoff

Ready. Work size: one header component, one icon, pure wording functions, and
one row change, with focused tests; a normal, modest browser assignment. Detail:
texts, priorities, colors, and the action are fixed; layout spacing is left to
the implementer within the existing dashboard style. The user judges the look;
no screenshots.
