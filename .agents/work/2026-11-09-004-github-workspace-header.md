# Work log: GitHub workspace header

- Task ID: `2026-11-09-004-github-workspace-header`
- Plan (repository-relative): `.agents/plans/2026-11-09-004-github-workspace-header.md`, blob `41738c246985820fde77063b7e01846492ab702a`
- Guidance: `.agents/designs/2026-11-09-001-github-integration.md`, blob `1be166e4fffb44898b53d6c18ad9f4fb89bd4152`
- Plan review: `.agents/reviews/plans/2026-11-09-004-github-workspace-header.md` (accepted)
- Author: impl-header, Claude Code, native conversation `81a3d88d-7eb7-40c7-84c3-9135cbf43834`, `claude-sonnet-5-5`
- Status: implemented
- Code base: `444646518183624627d89d802a4f9819a1db4aab`
- Code head: `d18dd2ce0f2c981596d8ce91844a7e0d80d077c0`
- Implementation commits: `d18dd2ce0f2c981596d8ce91844a7e0d80d077c0`

## Changes

- `internal/ui/web/github/`: `types.ts` (`GitHubStatus`, `PullRequest`), `wording.ts` (`trackingText`, `pullRequestFacts`, `pullRequestLook`), `pull-request-icon.tsx`, `github-header.tsx` (header, account `Select`, exported `chooseAccount`), with tests.
- `sessions/types.ts`: `Workspace.github`, `Session.pullRequest`. `sessions/workspace-dashboard.tsx` renders the header under the name. `shared/icon.tsx` registers `git-pull-request` and `git-pull-request-draft`. `sidebar/session-item.tsx` shows the pull request icon before the branch icon and treats it as an icon for the title's edge.
- `docs/interface.md`: header description and pull request icon table.

## Deviations

- The account chooser is not driven through the Radix `Select` in happy-dom; the test calls `chooseAccount` directly and asserts the request, as the plan's advisory allows. The test asserts the chooser's trigger and placeholder render.
- The dashboard heading's bottom margin is `mb-2` when the header shows (layout spacing was left to the implementer).

## Verification (target: Code head, macOS)

- `bun test web/github` in `internal/ui`: pass.
- `bun test` in `internal/ui`: 415 pass, 0 fail.
- `bunx tsc -p .` in `internal/ui`: no errors.
- `make check`: exit 0 (output in `/tmp/check004.out`), run on the code as committed.

## Provenance

| Role | Coordinator | Harness | Native conversation | Model |
| --- | --- | --- | --- | --- |
| implementer | impl-header | Claude Code | `81a3d88d-7eb7-40c7-84c3-9135cbf43834` | `claude-sonnet-5-5` |

## Limitations

No screenshots or live browser check; the user judges the look.

## Repair 1 (work review round 1)

- Code base (unchanged): `444646518183624627d89d802a4f9819a1db4aab`
- Code head: `634cca583e13c8a59af17eb6a84e5d8d77d676bf`
- Fix commit: `634cca583e13c8a59af17eb6a84e5d8d77d676bf`
- Finding 1: the error line now ends the error with a period before "Showing data from"; the test asserts the full sentence.
- Finding 2: `docs/interface.md` now says the chooser shows when several signed-in accounts can push, or none can push and several are signed in, matching `watcher.go` (candidates are the pushers, else all signed-in accounts; a chooser when more than one).
- Verification (target: new Code head): `bun test web/github` 10 pass; `bunx tsc -p .` no errors; `make check` exit 0 (`/tmp/check004b.out`).
