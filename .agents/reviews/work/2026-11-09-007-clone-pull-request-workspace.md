# Work review: clone a pull request's repository into a workspace

## Round 1

- Task ID: 2026-11-09-007-clone-pull-request-workspace
- Mode: work
- Author: xreview child session review-clone-workspace (parent 04ef1b14), Claude Code, model claude-opus-5-5; native conversation ID unavailable (not exposed to this session)
- Work log (repository-relative): `.agents/work/2026-11-09-007-clone-pull-request-workspace.md`
- Plan (repository-relative): `.agents/plans/2026-11-09-007-clone-pull-request-workspace.md`, blob `b94ca01899c97ccf1d9371142f7600c9a9bc6bd8` (matches the execution decision); guidance blobs: none
- Code base: `b47be44042971d36c8f99693393d01e937516e00`. The log records `5210fca7b4e3f821a8079b5e94e36816ebba1920`; the only commit between them (`b47be44`) changes the plan review record alone, so the reviewed code range is identical.
- Code head: `f4929060216eb6348d10e8f37912cfb1cb1ecb71`
- Checkout: `hypercube/session-04ef1b14` at `7be3ea2f028d3f1d66d33de1cffaaa1208509a51` (work-log commit only after Code head); clean tree.
- Independence: reviewer is a different session and model from the implementer (work-clone-workspace, claude-sonnet-5-5).

### Verdict: accept

No findings. The range implements the plan's decisions without scope additions beyond the recorded `ErrNotPullRequestRepo` deviation, which is the minimal way to tell a refusal (400) from a Git failure (409).

Invariants checked:

- **Repo/account check.** `Watcher.hasPullRequestIn` reads `w.accounts` under `w.mu` and requires an exact `Repo` match among that login's pull requests, so unknown logins and arbitrary repository strings are refused before any token is read or Git runs. The URL and `--config` value are built only from validated values.
- **Path.** `absolutePath` expands `~/`, requires an absolute path and cleans it; relative input is 400. The path is always absolute, so it cannot be read as a Git option.
- **Token.** Passed only as `GH_TOKEN` in the child environment. `gitError` carries only the argument list (URL, config entry, folder), so neither the stored config nor the error text returned to the browser contains it. The unit test asserts the token is absent from `.git/config`; the live run grepped for it.
- **Ownership and cancellation.** The handler runs `workspace.Clone` with `r.Context()`; `cmd.Cancel` sends `os.Interrupt` so Git removes its folder. No job list. The live run confirmed an abandoned clone leaves neither folder nor process.
- **Dialog.** Close by any route calls `close`, which aborts the controller; unmount also aborts. After `await`, an aborted signal returns before any state change, so an abort shows nothing. Errors stay in the dialog via `cloneWorkspace`, not the toast. Field, Choose… and Clone are disabled while running; Cancel stays enabled.
- **Pending selection.** `selectSession` now records a missing workspace key as `pending`; `receivedWorkspaces` selects it once `targetExists`, then the final `attach` attaches it. The other caller that can pass an unknown workspace key, boot from `location.hash` in `main.tsx`, now waits for the workspace instead of dropping it, which is harmless (a deleted workspace's key only stays pending until the next selection).

### Verification

All reused from the work log at Code head `f492906…` (macOS); no fresh runs were needed because the evidence matches the reviewed revision and no uncertainty remained after reading the code and tests.

- reused: `go test -race -count=1 ./internal/workspace ./internal/github ./internal/ui` — pass. Inspected `TestCloneRecordsConfigBeforeFetching` and `TestCloneAction`: they assert the local `credential.githubAccount`, `remote.origin.url`, 409 on a non-empty folder, 400 for an unlisted repo, an unknown account and a relative path, and no folder after a failed clone.
- reused: `bunx tsc -p .`, `bun test` (450 pass), `make check` — pass. Inspected `clone.test.ts`, `clone-dialog.test.tsx` (request parameters, busy state, error re-enable, abort with no error, pending workspace key), `all-pull-requests.test.tsx`, `selection.test.ts`.
- reused: live clone of a private repository through `POST /workspace?action=clone` on the port-4281 dev server, recorded account and no token, cancelled clone cleanup, then deletion.

### Limitations

- The plan's live check asked for the clone through the home-view row with the dashboard opening. The recorded live run used the HTTP action directly, so the dialog in a real browser and the dashboard opening after a real state message are not exercised live; they are covered by the component and selection tests above. The parent may decide whether a browser run is wanted.
- The interrupt-on-cancel path has no automated test, as the plan states.
