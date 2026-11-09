# Work log: GitHub pull requests panel

- Task ID: `2026-11-09-005-github-pull-requests`
- Plan (repository-relative): `.agents/plans/2026-11-09-005-github-pull-requests.md`, blob `735790eef7d8c8123f3e1b190d5096ad4abab8e7`
- Guidance: `.agents/designs/2026-11-09-001-github-integration.md`, blob `1be166e4fffb44898b53d6c18ad9f4fb89bd4152`
- Plan review: `.agents/reviews/plans/2026-11-09-005-github-pull-requests.md` (finding 1 resolved in the plan)
- Extra item from the parent: the header's error line adds "." only when the error does not already end with one.
- Author: impl-prs, Claude Code, native conversation `1fe21930-643c-4558-9309-1420ebd06b1c`, `claude-sonnet-5-5`
- Status: complete
- Code base: `0ab05a68a43a709ce17089250cecb277875360a0`
- Code head: `2efde1dbef924b82c8397af8b09df3dacfc7bbc8`
- Implementation commits: `2efde1dbef924b82c8397af8b09df3dacfc7bbc8`

## Changes

- `internal/ui/web/github/instructions.ts`: `actionsFor`, `instruction`, `actionLabels`, `sendInstruction`, `startInstruction`.
- `github/pull-requests.tsx`: `PullRequests` list with empty states, rows, actions, overflow link. `github/send-dialog.tsx`: the session picker.
- `sessions/workspace-dashboard.tsx` renders `PullRequests` after the header and labels the Recent Sessions list; `app/app.test.tsx` scopes its `li` assertion to that list.
- `github/github-header.tsx`: `sentence()` adds a period only when missing; test added.
- Tests: `github/instructions.test.ts`, `github/pull-requests.test.tsx`, `github/github-header.test.tsx`.
- `docs/interface.md` `## GitHub`: the Pull Requests list and actions.

## Deviations

None. Layout details (row spacing, author and facts as spans in one wrapped line) were left to the implementer.

## Verification (target: Code head, macOS)

- `bun test web/github web/app` in `internal/ui`: 65 pass, 0 fail.
- `bun test` in `internal/ui`: 426 pass, 0 fail.
- `bunx tsc -p .` in `internal/ui`: no errors.
- `make check`: exit 0 (output in `/tmp/check005.out`), run on the code as committed.

## Provenance

| Role | Coordinator | Harness | Native conversation | Model |
| --- | --- | --- | --- | --- |
| implementer | impl-prs | Claude Code | `1fe21930-643c-4558-9309-1420ebd06b1c` | `claude-sonnet-5-5` |

## Limitations

No screenshots or live browser check; the user judges the look.

## Completion

- Accepted work review: `.agents/reviews/work/2026-11-09-005-github-pull-requests.md` (round 1 accept)
- Final reviewed Code head: `2efde1dbef924b82c8397af8b09df3dacfc7bbc8`
- Implementation commits: `2efde1dbef924b82c8397af8b09df3dacfc7bbc8`
- Changes after the reviewed head on `hypercube/session-1771c79b` are workflow records under `.agents/` only.

| Role | Coordinator | Harness | Native conversation | Model |
| --- | --- | --- | --- | --- |
| parent | Hypercube session 1771c79b | Claude Code | `2fb9801a-4985-41f3-bd41-371c1ae39e8e` | `claude-opus-5-5` |
| planner | planner | Claude Code | `1e20740e-b018-49eb-9ae2-27fe8948db63` | `claude-opus-5-5` |
| plan and work reviewer | plan-reviewer | Claude Code | `7989b8d0-1436-4b4f-8ad3-10b45e003508` | `claude-opus-5-5` |
