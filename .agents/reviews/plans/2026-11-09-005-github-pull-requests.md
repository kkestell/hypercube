# Plan review: 2026-11-09-005-github-pull-requests

## Round 1

- Task ID: `2026-11-09-005-github-pull-requests`
- Mode: `plan`
- Author: reviewer `plan-reviewer`, Hypercube child of `1771c79b`, Claude Code,
  native conversation `7989b8d0-1436-4b4f-8ad3-10b45e003508`, model
  `claude-opus-5-5`. Independent of the planner (`1e20740e-b018-49eb-9ae2-27fe8948db63`).
- Plan (repository-relative): `.agents/plans/2026-11-09-005-github-pull-requests.md`,
  blob `6a2eeba9528c729abb5dbe9d4a42e04117b4d844`
- Guidance blobs: `.agents/designs/2026-11-09-001-github-integration.md`
  `bd05647e353db6d672ce69b41fe047ee89f2f730`;
  `.agents/plans/2026-11-09-004-github-workspace-header.md`
  `41c9484ae293309b1ec984f6f7a3d97d82a67441` (prerequisite components)
- Source HEAD: `b8025c7d20e951f87fec8a4ca30a721e78ebc39a`

### Verdict

Work size: right-sized. Detail: appropriate, with one unspecified state.
Not ready until finding 1 is settled. Action: `resolve-decisions`.

### Findings

1. **Empty state when the first query failed (minor, required).** The empty
   states are: no account; `updated` 0 and no error → `Checking GitHub.`; no
   pull requests → `No open pull requests.` With `updated` 0 **and** an
   `error` (first query failed, or a token read failure once 003 finding 2
   lands), the list falls through to `No open pull requests.`, which is false.
   Correction: add, before the last case, `updated` 0 with an error →
   `Could not load pull requests.` (the header already shows the reason), and
   a dashboard test for it.

2. **Fork pull requests (advisory, no change required).** **Address review**
   and **Fix checks** tell the agent to push to "its branch {branch}"; for a
   pull request with `fork` set, that branch is in the contributor's fork and
   the agent can push only when maintainers may edit it. The agent can work
   that out with `gh`; hiding those two actions for forks is the simpler
   alternative if the parent prefers.

Independently checked and sound: `requestMerge`'s `input` post and
`chooseSession` (`sessions/session-actions.ts:48,111`), `openUnsent`
(`sessions/selection.ts:128`), `typed` and `unsentKey` (`agent/drafts.ts:14,69`),
`statusOf`'s four-argument use in the dashboard, and `CommandDialog`'s props
all exist as the plan uses them; action rules and instruction texts are fixed;
picker ordering and the `li` assertion scoping are specified.

### Normalization

Active target 600–1,500 words (experimental default; no curated reference set).
Measured 828 words, no fenced code. Inside the band. One list, one picker
dialog, pure instruction functions, and two small actions with browser tests
is a normal browser assignment. No split or trim warranted.

### Handoff

Add the empty state in finding 1; the parent can verify it without a new
independent round. Finding 2 needs only the parent's acknowledgment.

## Coverage and shared interfaces (all four units)

Request coverage: (1) gh accounts, absent without gh → 002; (2) account
detection, repository from the tracked remote with `origin` fallback → 002,
saved per-workspace choice → 003, dropdown → 004; (3) `GH_TOKEN` in agents and
panel shells → 002; (4) no new MCP tools → no unit adds one; (5) background
fetch → 003; (6) header → 004, pull requests list and row actions → 005,
session row state → 004 with matching in 003; (7) one-minute polling → 002's
`Run` and 003's pass. Nothing is missing.

Interfaces agree: 002's `Watcher` (`Env`, `Statuses`, `Changed`, `Refresh`) and
`githubtest` stand-ins are what 003 consumes; 003's `Handler(db, rt, gh)`,
`workspaces[].github`, `sessions[].pullRequest`, and `github-account` action
are what 004 consumes; 004's `github/` types, `pullRequestFacts`, and
`PullRequestIcon` are what 005 consumes. The one cross-unit gap is the token
failure state (003 finding 2, 005 finding 1).
