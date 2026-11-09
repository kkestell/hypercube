# Work log: clone a pull request's repository into a workspace

- Task ID: 2026-11-09-007-clone-pull-request-workspace
- Plan (repository-relative): `.agents/plans/2026-11-09-007-clone-pull-request-workspace.md`, blob `b94ca01899c97ccf1d9371142f7600c9a9bc6bd8`; no guidance blobs
- Execution decision: `.agents/reviews/plans/2026-11-09-007-clone-pull-request-workspace.md` (parent 04ef1b14, "proceed")
- Author: xwork worker work-clone-workspace, Claude Code, claude-sonnet-5-5
- Status: complete
- Code base: b47be44042971d36c8f99693393d01e937516e00, the HEAD before the first implementation commit; 5210fca7b4e3f821a8079b5e94e36816ebba1920 differs from it only by a record-only commit
- Code head: f4929060216eb6348d10e8f37912cfb1cb1ecb71
- Implementation commits: f4929060216eb6348d10e8f37912cfb1cb1ecb71

## Changes

Responsibilities as in the plan.

- `internal/workspace/remote.go`: `Clone` (interrupts Git on cancel).
- `internal/github/clone.go`: `Watcher.Clone`, `ErrNotPullRequestRepo`.
- `internal/ui/actions.go`: `action=clone`, `absolutePath`.
- `internal/ui/web/api.ts`: `cloneWorkspace`.
- `internal/ui/web/github/`: `clone.ts`, `clone-dialog.tsx`, `pull-request-row.tsx` (trailing node), `all-pull-requests.tsx` (Clone button).
- `internal/ui/web/sessions/selection.ts`: pending workspace-key selection.
- `docs/interface.md`: Clone description.
- Tests: `workspace/remote_test.go`, `ui/github_test.go` (`TestCloneAction`), `web/github/clone.test.ts`, `clone-dialog.test.tsx`, `all-pull-requests.test.tsx`, `web/sessions/selection.test.ts`.

## Deviations

- Unlisted: `github.ErrNotPullRequestRepo` lets the handler return 400 for a refusal and 409 for Git failures.
- The existing `all-pull-requests.test.tsx` expectations changed because untracked rows now show a Clone button.
- `TestCloneAction` compares the workspace path after resolving the temporary folder's symlinks, because macOS records `/private/var/...`.

## Verification (all at Code head f492906…, macOS, `GIT_CONFIG_GLOBAL` isolated in Go tests)

- `go test -race -count=1 ./internal/workspace ./internal/github ./internal/ui`: pass (workspace 34.5s, github 30.3s, ui 37.7s).
- `bunx tsc -p .` in `internal/ui`: no output (pass).
- `bun test` in `internal/ui`: 450 pass, 0 fail.
- `make check`: exit 0 (full output in `/tmp/xwork-check.log`).
- Live, `make dev` server on port 4281 (not restarted): `POST /workspace?action=clone&repo=livefront/agent-skills&account=kkestell-livefront&path=/tmp/xwork-clone-live/agent-skills` → 200 with ID `5b21cc95` (the repo is private and invisible to the default gh account). `git config --local credential.githubAccount` = `kkestell-livefront`; `remote.origin.url` = `https://github.com/livefront/agent-skills.git`; `grep -E 'gh[opsu]_' .git/config` found nothing. The workspace's GitHub status then showed account `kkestell-livefront`, repo `livefront/agent-skills`, 4 pull requests. A second clone of `livefront/sigma` abandoned after 2s by the client left no folder and no Git process (cancel and cleanup work). Cleaned up with `action=delete&id=5b21cc95` (200) and removal of the folder.
- Not exercised: the browser dialog in a real browser; it is covered by `clone-dialog.test.tsx`.

## Session provenance

| Stage | Name / ID | Harness | Native conversation ID | Model |
| --- | --- | --- | --- | --- |
| parent | 04ef1b14 (Hypercube session) | Claude Code | 90b40c9b-b89d-4bd1-af98-13494c3d11de | claude-opus-5-5 |
| plan | plan-clone-workspace (child of 04ef1b14) | Claude Code | unavailable (not confirmed by the session) | claude-opus-5-5 |
| work | work-clone-workspace (child of 04ef1b14) | Claude Code | bf385e4a-0504-4980-adfe-c2caf7149ebc | claude-sonnet-5-5 |
| work review | review-clone-workspace (child of 04ef1b14) | Claude Code | unavailable (not confirmed by the session) | claude-opus-5-5 |

## Limitations

- Work review: `.agents/reviews/work/2026-11-09-007-clone-pull-request-workspace.md`, round 1 `accept` for the full range ending at `f4929060216eb6348d10e8f37912cfb1cb1ecb71`. After that head, only workflow records changed.
- The Clone dialog and the dashboard opening after a clone were not exercised in a real browser; component and selection tests cover them.
- Cancelling a clone has no automated test; the live run showed an abandoned clone leaves no folder.
