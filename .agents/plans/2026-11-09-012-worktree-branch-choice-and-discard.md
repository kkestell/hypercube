# New worktree sessions on a named branch from a chosen starting point

- Task ID: 2026-11-09-012-worktree-branch-choice-and-discard
- Status: ready
- Source HEAD: aadeae94d694eafe119f44800afa6932983b8742
- Author: planner child of Hypercube session 3703b04c (Claude Code, Opus 5.5,
  native conversation ed140e50-7ea6-40f1-84c8-faaf6329d964)
- Shared design: [.agents/designs/2026-11-09-012-worktree-branch-choice-and-discard.md](../designs/2026-11-09-012-worktree-branch-choice-and-discard.md)
  (unit 1 of 2; unit 2 is 2026-11-09-013-discard-worktree-work)
- Request items: 1 and 2 of the shared design.

## Outcome and boundaries

- The composer's location menu gains a third choice, **New worktree on
  branch…**, which opens a dialog for a new branch name and its source (a
  local branch, a remote branch, or any commit or ref typed in). The session is
  created in a new worktree on that new branch, starting at that source.
- **New worktree** keeps today's behavior: branch `hypercube/session-<id>` at
  the checkout's `HEAD`. Pull request **Review** keeps it too.
- The chosen name is `store.Session.Branch`, so the pull request icon, merge
  status, **Merge into …**, archive/delete checks and branch deletion,
  unarchive, and agent instructions follow it with no further change.
- A worktree session always creates a new branch. A name that already exists
  or that Git rejects refuses creation.

Non-goals: renaming a session's branch later, following a branch the agent
switches to, setting an upstream, changing children's branches, migrating
existing sessions.

## Current system

- `Runtime.create` (`internal/session/sessions.go`) sets
  `s.Branch = workspace.SessionBranch(s.ID)` and calls
  `workspace.AddWorktree(root, dir, branch, start)`, which resolves `start`
  to a commit and runs `git worktree add -b`, tracking whether the branch
  `existed` so that a failed add does not delete it.
- `CreateSubmission(workspaceID, settings, worktree bool, start, submission,
  text, images...)`; callers: `sessionAction` `"new"` and `startReview`
  (`internal/ui/actions.go`). `Create` (bool, no start) is used by tests.
- `Unarchive` (`internal/session/removal.go`) recreates `s.Branch` with
  `AddWorktree` at `s.Tip` or `HEAD`.
- Browser: `WorktreeSelect` (`internal/ui/web/agent/worktree-select.tsx`) sets
  `Unsent.worktree` through `unsentWorktree` (`sessions/selection.ts`);
  `deliverSubmission` (`sessions/session-actions.ts`) sends `worktree: '1'`;
  `agent/draft-storage.ts` persists `{workspace, settings, worktree}`.
- Patterns to copy: `github/clone-dialog.tsx` (shadcn `Dialog` with a form)
  and `agent/model-select.tsx` (`Popover` + `Command` combobox).

## Decisions

### Branch validation — `internal/workspace/branches.go` (new file)

```go
// CheckNewBranch returns an error unless name is a valid branch name that
// does not exist in root's repository.
func CheckNewBranch(root, name string) error
// Branches lists root's local branches and remote-tracking branches by
// short name, most recently committed first, without symbolic refs such as origin/HEAD.
func Branches(root string) (local, remote []string, err error)
```

- `CheckNewBranch`: run `git check-ref-format --branch <name>` in `root`;
  refuse with `"<name>" is not a valid branch name` when it fails or prints
  anything other than `name` (it expands `@{-1}` to the previous branch).
  This rejects what `git branch` rejects, including `HEAD`, a leading `-`,
  spaces, `..`, and `.lock` (checked with Git 2.50). Then refuse with
  `a branch named <name> already exists` when `exists(root, name)`. Names
  that clash with an existing branch path (`feature` vs `feature/x`) are left
  to `git worktree add`, whose error is returned.
- `Branches`: `git for-each-ref --sort=-committerdate
  --format=%(refname)%00%(symref) refs/heads refs/remotes`; skip lines with a
  symref; strip `refs/heads/` and `refs/remotes/`.
- `AddWorktree` calls `CheckNewBranch` first. Because the branch can no longer
  pre-exist, delete the `existed` bookkeeping: on a failed `worktree add`,
  delete the branch if it exists. Every caller (create, unarchive, children)
  gets the same check.

### Session creation — `internal/session/sessions.go`

```go
// NewWorktree places a new top-level session in its own worktree. Empty
// Start means the checkout's HEAD; empty Branch means hypercube/session-<id>.
type NewWorktree struct{ Start, Branch string }

func (r *Runtime) CreateSubmission(workspaceID string, settings harness.Settings,
	worktree *NewWorktree, submission, text string, images ...harness.Image) (store.Session, error)
```

- nil `worktree` means the checkout. The error "a starting point needs a
  worktree session" goes away. `Create` keeps its bool and passes
  `&NewWorktree{}` when true.
- `create` sets `s.Branch = cmp.Or(worktree.Branch, workspace.SessionBranch(s.ID))`.
  Validation happens in `AddWorktree`, before anything is saved; its error is
  returned to the browser as a failed submission (the existing path).

### Unarchive

No new logic. When a branch with the session's name now exists (the user made
one after archiving), `AddWorktree` refuses; wrap it as
`fmt.Errorf("cannot recreate the session's worktree: %w", err)`. The archived
session is unchanged, and unarchive can be retried after the user renames or
deletes that branch. Reusing the existing branch is rejected: it may hold
unrelated work, and archiving would later delete it.

### HTTP — `internal/ui`

- `"new"`: with `worktree=1`, pass `&session.NewWorktree{Start: q.Get("start"),
  Branch: q.Get("branch")}`; a `start` or `branch` without `worktree=1` is
  400 "a starting point or branch needs a worktree session".
- `startReview`: `&session.NewWorktree{Start: commit}`.
- New `GET /branches?workspace=<id>` (`internal/ui/branches.go`, registered
  in `ui.go`): reads the workspace, 409 for a non-Git workspace, and answers
  `{"current": workspace.Branch(root), "local": [...], "remote": [...]}`.
  `current` is empty when the checkout is detached.

### Browser

- `sessions/types.ts`: `Unsent.branch: NewBranch | null`, with
  `interface NewBranch {name: string; start: string}`. `branch` is non-null
  only while `worktree` is `true`.
- `sessions/selection.ts`: `unsentWorktree(state, worktree, branch = null)`
  sets both fields; `openUnsent` creates an unsent session with `branch: null`.
- `agent/draft-storage.ts`: save `branch`; `readContext` accepts a missing or
  null `branch` (drafts saved by earlier builds) or `{name, start}` strings.
- `api.ts`: `loadBranches(workspace)` through `get('branches', {workspace})`.
- `worktree-select.tsx`: three `menuitemradio` items: **Existing checkout**
  (`laptop`), **New worktree** (`git-branch`), **New worktree on branch…**
  (`git-branch`; checked while `branch` is set). The first two clear `branch`.
  The third opens the dialog; Cancel leaves the previous choice. While
  `branch` is set, the trigger shows the branch name with the `git-branch`
  icon and has `title="New worktree on <name> from <start>"`. A "…" radio
  item that opens a dialog follows the macOS "Other…" convention.
- New `agent/new-branch-dialog.tsx`, copying GitHub's **Create a branch**
  dialog (branches page): title **Create a branch**, a **New branch name**
  input (focused, prefilled from the current choice), a **Source** combobox,
  and **Cancel** / **Save** (nothing is created until the prompt is sent).
  - The dialog loads `loadBranches` on open. A failure shows the error text in
    the dialog and disables Save.
  - Source: a `Popover` + `Command` as in `model-select.tsx`, with
    `CommandInput` placeholder "Search branches or enter a commit", groups
    **Branches** and **Remote branches**, and, when the query is not empty
    and is not an exact listed name, an item **Use “<query>”** that selects
    the query text. Default: the current choice's start, else `current`, else
    `HEAD`.
  - Save is disabled while the trimmed name is empty; when it equals a local
    branch, the field shows "A branch named <name> already exists." and Save
    is disabled. Other invalid names are refused by the server at creation.
  - Save stores `unsentWorktree(state, true, {name, start})`.
- `deliverSubmission`: when `inWorktree` and `unsent.branch`, add
  `branch` and `start` to the `new` params. Retrying a failed creation reads
  the unsent session again, so a corrected branch applies to the retry.

## Implementation sequence

1. `internal/workspace/branches.go` and the `AddWorktree` change, with tests.
2. `NewWorktree`, `create`, `Unarchive` wrap; update `CreateSubmission`
   callers in `internal/ui` and tests.
3. `"new"` parameters, `startReview`, `/branches` handler.
4. Browser state, storage, API, menu, dialog, submission params.
5. Docs (below).

## Verification

Go (real temporary repositories, following `workspace_test.go` and the
`internal/session` fixture):

- `CheckNewBranch`: table of `feature/login` (ok), `HEAD`, `-x`, `a b`,
  `a..b`, `x.lock`, `@{-1}` after a checkout switch (refused), and an existing
  branch (refused with the exists message).
- `AddWorktree` with an existing branch name fails and leaves that branch at
  its commit; a failed add of a new name leaves no branch.
- `Branches` lists local and remote-tracking branches (bare remote + push +
  fetch) and omits `origin/HEAD` (`git remote set-head origin main`).
- Session: create with `NewWorktree{Start: <older commit>, Branch:
  "feature/login"}` → `Branch`, `Dir` checked out on it, `Base` = that commit,
  and `Merges()[id].Branch == "feature/login"`; create with an existing
  branch name errors and saves no session. Archive (after merging nothing; the
  branch has no new commits) deletes `feature/login` and records `Tip`;
  unarchive recreates it. Unarchive after `git branch feature/login` in the
  checkout fails, leaves the session archived, and succeeds after that branch
  is deleted.
- `internal/ui`: `pullRequest` matches a session in `.hypercube/` whose
  `Branch` is a custom name (extend `TestSnapshotShowsGitHubStatusAndPullRequests`);
  `"new"` with `branch` but no `worktree` is 400; `/branches` answers a Git
  workspace's lists and 409 for a non-Git workspace.

Browser (`bun test` in `internal/ui`, extending `composer.test.tsx`):

- The menu lists the three choices; choosing the third opens the dialog with
  branches from a stubbed `/branches`; entering a name and a source and
  choosing Save shows the name on the trigger; sending posts `worktree=1`,
  `branch`, and `start`.
- An existing local name disables Save with the message; a typed commit is
  offered as **Use “…”** and becomes the start.
- Choosing **New worktree** afterwards clears the branch from the request.
- Draft storage round-trips `branch` and reads a draft without it.

Commands: focused `go test`/`bun test` while working, then `make check`.

## Docs

Update facts, declaratively (see `docs/dev/documentation.md`):
`docs/usage.md` (Start a session, step 3: the third choice),
`docs/introduction.md` (a new worktree starts at the current commit or a
chosen branch or commit), `docs/storage.md` (Unarchive row: refused while a
branch with the session's name exists), `docs/lifecycle.md` (Unarchive
paragraph, same fact), and `docs/teamwork.md` line 29 if it states the start
point. Leave `docs/workflows.md` to the parent.

## Handoff

Ready; no open engineering decisions. One mechanism (a named new branch at a
chosen start) through the Git layer, session creation, two routes, and one
composer control with a dialog, plus tests: a normal assignment, comparable to
2026-11-09-009. At 1,571 words it is slightly above the 600–1,500 band because
the dialog's behavior and the validation contract need stating; splitting the
browser half from the backend would leave neither unit usable alone.
