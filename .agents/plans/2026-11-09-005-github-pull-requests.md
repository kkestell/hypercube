# List a workspace's pull requests and send pull request work to a session

- Task ID: `2026-11-09-005-github-pull-requests`
- Status: ready
- Source HEAD: `47055e42f036f863ebb18110d3803fbce3d6f38b` (plan written before 002)
- Author: planner `planner`, Claude Code, native conversation
  `1e20740e-b018-49eb-9ae2-27fe8948db63`, `claude-opus-5-5`
- Shared design (repository-relative): `.agents/designs/2026-11-09-001-github-integration.md`,
  [linked here](../designs/2026-11-09-001-github-integration.md); see Merge
  semantics.
- Prerequisite: [004](2026-11-09-004-github-workspace-header.md) committed
  (`github/` types, wording, `PullRequestIcon`, `GitHubHeader`).

## Outcome and boundaries

A GitHub workspace's dashboard lists its open pull requests between the header
and Recent Sessions. Each row shows the pull request's state, number and title
linked to GitHub, author, review decision and checks, and the actions that
apply. An action sends a fixed instruction to a session the user picks, or
opens a new session with the instruction in its composer. The browser never
merges or writes to GitHub itself. Browser-only, plus docs.

## Current system

- `sessions/workspace-dashboard.tsx:WorkspaceDashboard` (after 004) renders the
  name, `GitHubHeader`, then "Recent Sessions" as a `ul` of session buttons.
  `app/app.test.tsx` asserts every `li` in the dashboard is a recent session.
- `sessions/session-actions.ts:requestMerge` sends an instruction with
  `post(store, 'session', {action: 'input', id}, JSON.stringify({text, images:
  []}))`; `chooseSession(store, id)` shows a session.
- `sessions/selection.ts:openUnsent(state, workspace)` opens the workspace's new
  session; `agent/drafts.ts:typed(state, unsentKey(workspace), text)` sets its
  draft text.
- `shared/command-dialog.tsx:CommandDialog` wraps the shadcn command list in a
  dialog with focus handling.

## Changes

1. **Instructions** (`github/instructions.ts`, pure, tested). `PullRequestAction`
   is `'merge' | 'review' | 'checks'`. `actionsFor(pr)`: `merge` unless draft;
   `review` when review is `CHANGES_REQUESTED`; `checks` when checks are
   `FAILURE` or `ERROR`; in that order. Labels: `Merge`, `Address review`,
   `Fix checks`. `instruction(action, pr)` returns exactly:
   - merge: `Merge pull request #{n} ({url}). Check that it is approved and its checks pass, use a merge method the repository allows, and report the result.`
   - review: `Address the review of pull request #{n} ({url}): read its review comments with gh, make the requested changes on its branch {branch}, push them, and report what you changed.`
   - checks: `Fix the failing checks of pull request #{n} ({url}): read the failing checks' logs with gh, fix the cause on its branch {branch}, push the fix, and report the result.`
2. **Sending** (`github/instructions.ts` or `session-actions.ts`):
   `sendInstruction(store, sessionId, text)` posts the `input` action as
   `requestMerge` does, then `chooseSession(store, sessionId)`.
   `startInstruction(store, workspaceId, text)` runs
   `store.update(s => typed(openUnsent(s, workspaceId), unsentKey(workspaceId), text))`,
   replacing any unsent draft text; the user picks settings and sends.
3. **List** (`github/pull-requests.tsx`): `PullRequests({workspace})`, rendered
   by `WorkspaceDashboard` after the header when `workspace.github` is set, as
   a section headed `Pull Requests` (`h2`, same style as Recent Sessions) with
   its own labeled `ul`. Empty states, in order: no `account` → `Choose an
   account to see pull requests.`; `updated` 0 with an error → `Could not load
   pull requests.` (the header shows the error); `updated` 0 and no error →
   `Checking GitHub.`; no pull requests → `No open pull requests.` Each row:
   `PullRequestIcon`; `#{n} {title}` as an external link to `url`; a muted
   line `{author}` followed by `pullRequestFacts(pr)`; one small secondary
   button per `actionsFor(pr)`. When `open` exceeds the list's length, end with
   an external link `View all {open} open pull requests on GitHub` to
   `{url}/pulls`.
4. **Session picker** (`github/send-dialog.tsx`): a `CommandDialog` opened by an
   action button, titled `{label} for #{n}`. Items, in order: `New Session`,
   then the workspace's non-ended sessions, top-level and children flattened,
   with those whose `pullRequest.number` is this pull request's first, others
   in workspace order. Each session item shows its status dot (`statusOf`, as
   the dashboard does) and title. Selecting a session calls `sendInstruction`;
   `New Session` calls `startInstruction`; both close the dialog.
5. Scope the existing dashboard test's `li` assertion to the Recent Sessions
   list (for example by its accessible name) so the pull request rows do not
   count.

### Documentation

Extend `docs/interface.md` `## GitHub`: the Pull Requests list, when each action
appears, that an action sends an instruction to the session you pick (the
agent does the work with `gh` and the workspace's account) or opens a new
session with the instruction ready to send, and that the list shows up to 50
pull requests, most recently updated first.

## Verification

Browser tests with bun in `internal/ui` (`bun test`, `bunx tsc -p .`):

- `instructions.ts`: `actionsFor` table (draft; changes requested with failing
  checks gives all three but merge only when not draft; approved and passing
  gives merge only); `instruction` texts match the strings above.
- Dashboard: rows show link, author, and facts; empty states for no account,
  first check pending, never loaded with an error (`Could not load pull
  requests.`, not `No open pull requests.`), and none open; the overflow link appears only when
  `open` exceeds the rows.
- Picker: opening **Fix checks** lists `New Session`, the matching session
  before others, and no ended sessions; choosing a session posts
  `action=input&id=<session>` with body text equal to the instruction and
  selects that session; choosing `New Session` leaves the unsent session open
  for the workspace with the instruction as its draft text.
- Existing dashboard test still passes after scoping.
- `make check` before committing.

## Handoff

Ready. Work size: one list component, one picker dialog, pure instruction
functions, and two small actions with browser tests; a normal browser
assignment. Detail: actions, texts, ordering, and empty states are fixed; the
user judges spacing and look; no screenshots.
