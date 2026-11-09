# Clone a pull request's repository into a workspace

- Task ID: 2026-11-09-007-clone-pull-request-workspace
- Status: ready
- Source HEAD: bf8e583130589a550ccc76c16f4baaedb5f1b089
- Author: xplan child session plan-clone-workspace (parent 04ef1b14), Claude Code, claude-opus-5-5
- Dependencies: 2026-11-09-006-global-pull-requests (done; plan in `.agents/plans/`)

## Outcome and boundaries

In the home view (`internal/ui/web/github/all-pull-requests.tsx`), a row whose
repository no workspace tracks gets a **Clone** button. It opens a dialog with a
suggested folder that the user confirms or edits. Hypercube clones the
pull request's target repository with the account that saw the pull request,
records that account in the clone's own Git config, adds the workspace, and
shows its dashboard. The dialog shows that cloning is under way, shows Git's
error on failure, and cancels the clone when closed.

Out of scope: desktop notifications, byte-level clone progress, adopting an
existing checkout in the chosen folder, SSH remotes.

## Current system

- `PullRequest.repo` is GitHub's `repository.nameWithOwner`: the base
  repository, never the fork. Cloning `pr.repo` therefore already clones the
  target for fork pull requests.
- `Workspace.github?.repo` (from `Watcher.pass`, `internal/github/watcher.go`)
  is the `owner/name` of each GitHub workspace; `Workspace.path` is its
  absolute checkout root.
- `Watcher.pass` takes a workspace's account from the checkout's
  `credential.githubAccount` first, so a clone that sets it needs no detection
  or chooser.
- `workspace.Fetch` (`internal/workspace/remote.go`) runs Git with
  `GH_TOKEN=<token>` and `GIT_TERMINAL_PROMPT=0` in the environment; the
  `gh auth git-credential` helper turns that into credentials.
- `POST /workspace?action=new` (`internal/ui/actions.go`) calls
  `s.rt.AddWorkspace(dir)` then `s.gh.Refresh()` and returns the ID.
- `addWorkspace` (`web/sessions/session-actions.ts`) uses
  `hypercube()?.chooseFolder()`, which exists only in the desktop app.
- `selectSession` (`web/sessions/selection.ts`) ignores a workspace key whose
  workspace is not yet in state; only session IDs wait in `state.pending`.

## Decisions

**Authentication (verified live, 2026-11-09).** The server runs:

```
git clone --quiet --config credential.githubAccount=<login> https://github.com/<repo>.git <dir>
```

with `GH_TOKEN=<login's token>` and `GIT_TERMINAL_PROMPT=0` added to the
environment, as `Fetch` does. `git clone --config` writes the setting into the
new repository before Git fetches, so credential helpers see it during the
clone. Both parts are needed: with the user's real Git config (a
`credential.githubAccount`-reading helper and a global default account), a
private `kkestell-livefront` repo cloned only with `--config`; with only the
`gh auth git-credential` helper configured, `GH_TOKEN` supplied the token. The
token appears in neither the clone's config nor `remote.origin.url`. Global Git
and gh settings are untouched. Always HTTPS, matching the token approach.

**Ownership.** The request handler owns the clone process and runs it with
`r.Context()`. Closing the dialog aborts the browser's request, which cancels
the context. The command's `Cancel` sends `os.Interrupt` instead of the default
kill, so Git removes the folder it created. A clone that fails ordinarily also
removes it (Git's own behavior). No server-side job list.

**Server action.** `POST /workspace?action=clone&repo=<owner/name>&account=<login>&path=<folder>`.

- `github.Watcher.Clone(ctx, repo, login, dir string) error` in a new
  `internal/github/clone.go`: refuses unless `repo` exactly equals the `Repo`
  of one of `login`'s pull requests in the published accounts (error
  `"not a pull request repository for <login>"`; this also rejects unknown
  logins and arbitrary repo strings). Reads the token with `w.token`, then calls
  `workspace.Clone`.
- `workspace.Clone(ctx, url, dir string, config, env []string) error` in
  `internal/workspace/remote.go`, beside `Fetch`: runs the command above with
  one `--config` per entry, `cmd.Cancel` sending `os.Interrupt`, stderr
  wrapped in `gitError` like `Fetch`.
- Handler: `path` must be absolute or start with `~/` (expand with
  `os.UserHomeDir`, then `filepath.Clean`); otherwise 400 `"the folder must be
  an absolute path"`. Missing parents are fine; Git creates them. A refusal from
  `Watcher.Clone` is 400; a Git failure is 409 through `refused`, carrying Git's
  message (for an existing non-empty folder: "destination path … already exists
  and is not an empty directory"). On success: `s.rt.AddWorkspace(dir)`,
  `s.gh.Refresh()`, write the workspace ID, as `action=new` does.

**Existing folder or interrupted clone.** Git accepts a missing or empty folder
and refuses a non-empty one; Hypercube adds no check of its own. A clone killed
without the interrupt (Hypercube itself dies) can leave a partial folder; the
retry then shows Git's "already exists" error naming it, and the user picks
another folder or deletes it.

**Not tracked.** A row is untracked when no workspace has
`github?.repo.toLowerCase() === pr.repo.toLowerCase()`. A workspace whose
GitHub status is missing counts as not tracking anything.

**Suggested folder.** Pure function: take the first workspace in
`state.workspaces` whose `github.repo` owner equals the pull request's owner,
ignoring case; if none, the first workspace. Suggest its path's parent plus `/`
plus the repository name. The home view only exists with workspaces, so a base
always exists. Example: a `MinneapolisStarTribune/a` workspace at
`/Users/kyle/work/strib/a` gives `/Users/kyle/work/strib/b` for
`MinneapolisStarTribune/b`.

**Dialog.** One dialog for both the browser and the desktop app, built on
`components/ui/dialog.tsx`: title "Clone {repo}", description "Clones with
{account} and adds a workspace.", a labeled **Folder** text input prefilled with
the suggestion, and the buttons Cancel and Clone. In the desktop app only
(`hypercube()` defined), a **Choose…** button calls `chooseFolder()` and, if a
folder comes back, sets the field to `<chosen>/<name>`. While cloning: the
field and Clone are disabled, Clone reads "Cloning…" beside `BusyDot` (`web/shared/busy-dot.tsx`), and Cancel stays enabled. Failure shows the server's message in the
dialog in destructive text and re-enables editing, so the user can change the
folder and retry. Closing the dialog by any means aborts a running request; an
abort shows nothing. The dialog calls `fetch` directly with an
`AbortController` (a new `cloneWorkspace(params, signal)` in `web/api.ts`
returning `{workspace} | {error}`), not `post`, so errors stay in the dialog
instead of the toast.

**After the clone.** The dialog closes and the page selects the new workspace's
dashboard, which lists the pull request with its actions once the refreshed
pass arrives. The POST can return before the state message that lists the
workspace, so `selectSession` sets `pending` for a workspace key that is not in
state yet, and `receivedWorkspaces` selects a pending workspace key once its
workspace exists. The Clone button disappears from rows of that repository
because it is now tracked.

## Implementation sequence

1. **`internal/workspace/remote.go`**: `Clone` as decided.
2. **`internal/github/clone.go`**: `Watcher.Clone` as decided, reading the
   published accounts under `w.mu`.
3. **`internal/ui/actions.go`**: the `clone` case; update the
   `workspaceAction` comment.
4. **`web/api.ts`**: `cloneWorkspace`.
5. **`web/github/clone.ts`**: `tracks(workspaces, repo)` and
   `suggestedFolder(workspaces, repo)`.
6. **`web/github/clone-dialog.tsx`**: the dialog. Props: `pr`, `account`,
   `onClose`.
7. **`web/github/pull-request-row.tsx`**: an optional trailing node rendered
   after the actions. **`all-pull-requests.tsx`**: for untracked rows, a
   secondary small **Clone** button (`aria-label="Clone {repo}"`) that sets
   local `{pr, account}` state; render the dialog when set.
8. **`web/sessions/selection.ts`**: pending workspace selection as decided.
9. **Docs**: read `docs/dev/documentation.md` first. In `docs/interface.md`'s
   GitHub section, describe the Clone button, the suggested folder, the
   recorded `credential.githubAccount`, and that the dashboard opens after.
   No screenshots.

## Verification

Go, `internal/workspace/remote_test.go`: `Clone` from a local bare repository
with `config` `credential.githubAccount=alice` yields that local setting and
the given URL as `remote.origin.url`. The interrupt on cancel relies on Git's
own cleanup and has no automated test; a reliable one needs a clone slow
enough to interrupt.

Go, `internal/ui/github_test.go`, modeled on
`TestStateCarriesGitHubStatusAndAccountChoice`: set `GIT_CONFIG_GLOBAL` to a
temporary file with `url.<bareRoot>/.insteadOf = https://github.com/`, create
`<bareRoot>/o/r.git` with one commit, and have `githubtest` give alice a pull
request in `o/r` and one in `o/missing`. Wait for the accounts, then:

- `action=clone&repo=o/r&account=alice&path=<tmp>/new/r` → 200; the body is a
  workspace ID whose path is that folder; its local `credential.githubAccount`
  is `alice`; `remote.origin.url` is `https://github.com/o/r.git`.
- Same request again → 409 containing "already exists".
- `repo=o/s` or `account=carol` → 400; relative `path=r` → 400.
- `repo=o/missing` → 409 and no folder at the target.

Browser (`bun test` in `internal/ui`):

- `clone.test.ts`: `tracks` ignores case and skips workspaces without GitHub;
  `suggestedFolder` uses the same owner's parent, falls back to the first
  workspace, and ignores owner case.
- `all-pull-requests.test.tsx`: Clone appears only on untracked rows.
- `clone-dialog.test.tsx` with a stubbed `fetch`: prefilled folder; submit
  sends `repo`, `account` and the edited `path`; "Cloning…" while pending;
  an error response shows its text and re-enables the field; success selects
  the workspace dashboard key (pending until the workspace is in state);
  closing aborts the request's signal and shows no error.
- `selection` test: a pending workspace key becomes selected when
  `receivedWorkspaces` brings that workspace.

Commands: `go test -race ./internal/workspace ./internal/github ./internal/ui`,
`bunx tsc -p .` and `bun test` in `internal/ui`, then `make check`.

Live, on the running `make dev` at port 4281 (do not restart it): with no
workspace for `kkestell/hypercube-pr-lab`, its rows show Clone; clone it to a
folder under `/tmp`, confirm the dashboard opens and lists its pull requests,
`git -C <folder> config --local credential.githubAccount` names the row's
account, and `remote.origin.url` has no token. Then delete that workspace in the
app and remove the folder.

## Handoff

Ready. Work size: one normal assignment, one mechanism (clone-to-workspace)
through a Git helper, a server action, and one browser dialog with its
selection hook; about seven production files and five test files. Detail:
authentication was verified live; ownership, validation, failure, folder, and
post-clone behavior are decided; markup and helper names are left to the
implementer.
