# Create a session with its first prompt

## Plan

`agents/plans/2026-11-01-001-create-session-on-first-prompt.md`

## Summary

`Runtime.Create` takes the first prompt, titles the session from it, starts the
lead, and sends the prompt. `POST session?action=new` takes the same JSON body
as `agent?action=input`. Choosing a lead in the browser opens an unsent lead
pane held in page memory; sending its first prompt creates the session, and
selecting another session or lead discards it. The plan's goal is met.

## Departures from the plan

- `report.Prompt` keeps no agent argument: it only releases queued messages.
- `agentAction` reads the body only for `answer`; `input` bodies go through the
  shared `readInput` helper, which `sessionAction` also uses.
- Tests that count a lead's inputs or transcript items go through the fixture's
  `inputs` and `transcript` helpers, which leave out the creation prompt and its
  reply for leads made by `fixture.session()`. `internal/ui` tests that
  counted transcript items were adjusted by hand.
- `TestAgentThatKeepsEndingStaysEnded` ignores `Create`'s error: the stand-in
  plugin can exit before the prompt arrives, and the session remains.

## Decisions

- The unsent pane is a synthetic lead (`id` "new") rendered through `card`.
  While the create request runs it shows as busy with Send disabled.
- A late response for an unsent session the user has left is dropped; the
  created session appears in the sidebar from the next state update.

## Checks run

- `make check` — passed (ran `bun install --frozen-lockfile` in `internal/ui`
  first; the worktree had no `node_modules`).
- Parsed the page's module script with `bun build --no-bundle`. The page was
  not run in a browser.

## Follow-up work

- The page's unsent-session flow has no automated test.
