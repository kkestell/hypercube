# Multiple question answers and combined verification

## Plan

`agents/plans/2026-11-06-004-multiple-question-answers.md`

## Summary

Implemented #3 through the browser, HTTP action, recorded plugin command, and
native adapters. Claude multiSelect questions, OpenCode array questions, and
Codex enumerated array forms retain all selected labels; single-choice dialogs
and approvals keep their controls. Selected labels share the existing draft
owner across transcript and notification forms. Saved singular commands still
replay through the dispatcher.

#13's existing framing fix was committed with the pre-existing security work
in `509bac8`, then verified again at the HTTP boundary. Three subagents completed
the other sixteen issues; their plans and logs use the `issue-session-fixes`,
`issue-ui-fixes`, and `issue-plugin-fixes` slugs.

## Decisions

- Native Claude answer formatting was checked against Anthropic's
  [AskUserQuestion demo](https://github.com/anthropics/claude-agent-sdk-demos/tree/main/ask-user-question-previews).
- Added the upstream shadcn Checkbox source, with its generated license notice,
  instead of custom checkbox styling.
- Shared protocol validation checks choices before recording external answers
  and before native adapters consume plugin commands.

## Checks run

- Focused Claude/OpenCode and HTTP race regressions passed, including all labels,
  punctuation, invalid selections, retry, and the framing header.
- Three mounted form tests passed: explicit submission and retry, shared draft
  choices, and read-only ended sessions.
- `make check` passed, including all 253 UI tests, 27 desktop tests, and the full
  Go race suite. Independent reviews reported no confirmed findings; see
  `agents/reviews/2026-11-06-001-issue-fixes.md` for scope and limits.

## Manual verification

`HYPERCUBE_DEV_DIR=<temporary directory> make dev` served an isolated checkout
on port 4281. The main page, JavaScript, stylesheet, config, and workspace action
responded successfully, and the page returned `X-Frame-Options: SAMEORIGIN`.
Opened the page in the user's browser. Rendering and visual judgment were left
to the user. No native agent was started and the installed service was untouched.
