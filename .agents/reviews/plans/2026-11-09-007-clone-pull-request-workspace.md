# Plan review: clone a pull request's repository

## Round 1

- Task ID: 2026-11-09-007-clone-pull-request-workspace
- Mode: plan
- Author: check-clone-plan, Hypercube child of 04ef1b14; OpenCode, GPT-6.1 Terra; native conversation `ses_e3e04cd84ffePJaSjBSSkPv5s5`; effort unavailable.
- Plan (repository-relative): `.agents/plans/2026-11-09-007-clone-pull-request-workspace.md`; blob `ddd580e461071a7fb0613181e8c3aff7c012b430`.
- Reviewed checkout HEAD: `8ee8659e5366cf00df5821a4f3dedc6d2029b17e`; plan's source HEAD: `bf8e583130589a550ccc76c16f4baaedb5f1b089`.
- Guidance blobs: none; no linked shared designs.
- Verdict: **changes required**. Work size: right-sized. Detail: anemic on authentication; otherwise appropriate. Primary action: `resolve-decisions`.

### Finding 1: the chosen account is not guaranteed to authenticate the clone

The authentication decision (lines 45–59) sets `GH_TOKEN` and local `credential.githubAccount` but depends on the user's existing Git credential helpers. `GH_TOKEN` is consumed by the gh credential helper, not Git itself; gh login does not establish that Git is configured to invoke that helper. Other configured helpers may return credentials before gh is consulted and need not honor `credential.githubAccount`. Thus a signed-in account can fail to clone a private repository, or clone using another account, contrary to the accepted account requirement. The live checks use two particular helper configurations and do not cover either case. The proposed local-bare-repository test (lines 157–168) rewrites HTTPS to a local path, so it cannot verify authentication.

Decide a per-invocation Git credential configuration that actually supplies the selected account's token regardless of existing helpers, without persisting the token or changing global configuration, while retaining the repository-local account setting. Add a test at a suitable credential boundary proving the selected token is used with no existing helper and with a competing helper; verify no token is persisted. Preserve useful Git errors without exposing the token.

### Assessment and evidence

The assignment covers one connected clone-to-workspace flow: Git execution and cancellation, account/repository validation and workspace registration, dialog/progress and desktop picker, selection after asynchronous state arrival, and integration/browser checks. Its cross-layer coordination resembles the normal lifecycle reference assignment, not the independent subsystems in the splitting reference; a separate test-only unit would not be proportionate here. Scope is bounded. The plan identifies base-repository handling, account setting, folder suggestion, errors, and the selection race; current `internal/github/watcher.go`, `internal/github/gh.go`, `internal/workspace/remote.go`, `internal/ui/actions.go`, `internal/session/sessions.go`, and `internal/ui/web/sessions/selection.ts` were inspected. The authentication flaw is a decision defect, not evidence for splitting the assignment.

Normalization: measured **1,459 words**, within the experimental default **600–1,500** band; zero curated references were supplied. Length is advisory and does not establish readiness. No implementation tests or builds were run for this plan review. The live authentication evidence in the plan was not independently repeated; the helper-dependence follows from the described command and credential model.

### Handoff

Resolve finding 1 and make the verification exercise real credential selection rather than only a local URL rewrite. Parent should record the corrected plan blob and execution decision before implementation. No code or plan was edited, and no commit was made.

## Execution decision

- Author: parent coordinator, Hypercube session `04ef1b14`, Claude Code, native conversation `90b40c9b-b89d-4bd1-af98-13494c3d11de`, model `claude-opus-5-5`
- Round 1 is set aside. The user ruled that it went beyond the scope of the plan check, so finding 1 is not acted on and the plan's original authentication decision stands.
- Parent correction, independent of round 1: the live check now clones an untracked private repository, because `kkestell/hypercube-pr-lab` is already a workspace and shows no Clone button.
- Final plan blob: `b94ca01899c97ccf1d9371142f7600c9a9bc6bd8`. Guidance blobs: none.
- Decision: `proceed`
