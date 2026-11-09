# Plan review: 2026-11-09-003-github-status

## Round 1

- Task ID: `2026-11-09-003-github-status`
- Mode: `plan`
- Author: reviewer `plan-reviewer`, Hypercube child of `1771c79b`, Claude Code,
  native conversation `7989b8d0-1436-4b4f-8ad3-10b45e003508`, model
  `claude-opus-5-5`. Independent of the planner (`1e20740e-b018-49eb-9ae2-27fe8948db63`).
- Plan (repository-relative): `.agents/plans/2026-11-09-003-github-status.md`,
  blob `33d265b4dfe4b7abdb2471bfd77976ef89b242be`
- Guidance blobs: `.agents/designs/2026-11-09-001-github-integration.md`
  `bd05647e353db6d672ce69b41fe047ee89f2f730`;
  `.agents/plans/2026-11-09-002-github-accounts.md`
  `665bfe3e1eb2079209f553481eb073501290aa8f` (prerequisite it extends)
- Source HEAD: `b8025c7d20e951f87fec8a4ca30a721e78ebc39a`

### Verdict

Work size: right-sized. Detail: appropriate, with one incorrect settled
decision. Not ready until finding 1 is corrected. Action: `resolve-decisions`.

### Findings

1. **Checkout sessions match pull requests by a stale branch (required).**
   The plan's Current system says `Branch` is "empty for sessions sharing the
   registered checkout", and the matching rule uses `s.Branch` when set.
   But `internal/session/lifecycle.go:58-96` sets `s.Branch` for every
   top-level checkout session (`s.Parent == "" && s.Base == ""`) to the
   checkout's branch **at each launch**. So a checkout session that creates a
   feature branch and opens its pull request — the common flow — keeps matching
   the branch it launched on (often `main`), and its row shows no pull request
   until it relaunches. The snapshot test "checkout session matched by status
   branch" would pass only because the fixture leaves `Branch` empty, which
   real checkout sessions never do.
   Correction: decide by directory first. A session whose `Dir` is the
   workspace path or inside it but not under `<path>/.hypercube/` uses the
   status's `Branch`; any other session uses `s.Branch` (empty means none).
   Fix the Current system bullet, and give the checkout-session fixture a
   stale `Branch` that differs from the status branch.

2. **A token read failure is invisible (minor, required).** 002 logs a token
   error and leaves the account set without a token; 003 then skips the fetch
   and query, so `Error` is empty and `Updated` stays 0. The header shows
   nothing wrong and 005's list shows `Checking GitHub.` indefinitely.
   Correction: when the workspace has an account but no token, set `Error` to
   the token error (`could not read the token for LOGIN: ...`).

3. **Background fetch over SSH (advisory, no change required).** `GIT_TERMINAL_PROMPT=0`
   does not stop `ssh` from asking for a key passphrase on the backend's
   terminal (`make dev`), or a confirm-per-use SSH agent from prompting every
   minute per workspace. Keys in an ordinary agent and HTTPS remotes are
   unaffected, and this checkout's remote is HTTPS. Accept the behavior unless
   the parent wants HTTPS-only fetching.

Independently checked and sound: `--no-write-fetch-head` rationale;
`HEAD...@{upstream}` left/right order gives ahead/behind; keeping the last good
pull requests while `Repo` is unchanged; the `Changed()` select case fits the
existing coalesce loop (`internal/ui/socket.go:50-98`) and the zero `Watcher`
never fires; `github_accounts` cascades because the store opens with
`foreign_keys(1)` (`internal/store/store.go:177`); the account action validates
against `Choices` and refreshes.

### Normalization

Active target 600–1,500 words (experimental default; no curated reference set).
Measured 1,112 words, 12 fenced. Inside the band. One pipeline extension
(watcher pass → snapshot → socket) with two Git helpers, a query, a matching
rule, one action, and four test layers matches the normal-assignment
references. No split or trim warranted.

### Handoff

Correct findings 1 and 2 in the plan; the parent can verify both without a new
independent round. Finding 3 needs only the parent's acknowledgment.

## Execution decision

- Author: parent coordinator (Hypercube session 1771c79b, Claude Code, native conversation 2fb9801a-4985-41f3-bd41-371c1ae39e8e, claude-opus-5-5)
- Revision checked: 63165bad85f0b12c1fd122d2c4d70545141755e5
- Final plan blob: `.agents/plans/2026-11-09-003-github-status.md` 6a839caf26b790cc2707c6398aed2e6529358e27
- Guidance blobs: `.agents/designs/2026-11-09-001-github-integration.md` 1be166e4fffb44898b53d6c18ad9f4fb89bd4152
- Dispositions: Finding 1 resolved: the session branch now comes from `s.Dir`. In the checkout outside `.hypercube/`, it is the status branch and `s.Branch` is ignored. Inside `.hypercube/`, it is `s.Branch`. Anywhere else, none. The Current system bullet is corrected, and the fixture has a stale saved Branch. Finding 2 resolved: a token read failure sets Error, a pending choice leaves Error empty, and a watcher test covers both. Finding 3 is advisory and needs no change.
- Decision: proceed
