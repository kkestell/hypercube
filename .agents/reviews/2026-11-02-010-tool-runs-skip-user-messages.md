# Tool runs skip user messages

## Scope and coverage

Reviewed `git diff f619a6a` in the `tool-work` worktree: `transcript-tree.js`, `tool-items.jsx`, `tool-items.css`, `transcript-tree.test.js`, `transcript.test.jsx`, and `docs/usage.md`, with their callers (`transcript-item.jsx`, `transcript.jsx`, `transcripts.js`). Generated bundles in `internal/ui/dist` were checked only for being regenerated. After the user's visual feedback, the parent moved the chevron to the end of each disclosure heading and set line height 1.5 on the tool summary. I reviewed that delta in the original checkout (`git diff` against the index): `tool-items.jsx`, `tool-items.css`, `transcript.test.jsx`, and all of `message-item.jsx` and `message-item.css`, with `reasoning-item.jsx` and `.css` for consistency. It has no findings. A later change moved `line-height: 1.5` to `.message-item-agent` and `.tool-items-disclosure`, so summaries, previews and expanded bodies inherit one value. I traced that inheritance in `message-item.css`, `tool-items.css`, `reasoning-item.css`, `markdown.css` and `base.css`, and it has no findings either. Expanded tool input and output now inherit 1.5 as well, where the page default was `normal`. That is a visual effect for the user to judge. The last change styled agent text as a left-aligned bubble with CSS only (`transcript-item.css`). I inspected that CSS and the `TextItem` caller in `transcript-item.jsx`: both the top-level list and the nested call children are column flex containers, so `align-self` applies at both levels, and `markdown.css` keeps code blocks scrolling inside the bubble. It has no findings. The review was against the plan and the user's clarification: user items neither close nor split an open run, other non-tool rows close it as before, the run summary has no spinner, and individual calls keep theirs.

Gaps:

- No rendering or screenshot checks; the user owns visual review, including how the indented second line looks.
- `make check` was running in the author's session and was not rerun here.
- By request, nothing was fixed or staged. F1 is left unchanged under the clarified scope.

## Summary

| ID | Severity | Category    | Disposition    | Title                                                          |
| -- | -------- | ----------- | -------------- | -------------------------------------------------------------- |
| F1 | Medium   | Correctness | Unchanged under clarified scope | A run left open at turn end absorbs the next turn's tool calls |

## F1. A run left open at turn end absorbs the next turn's tool calls

Medium · Correctness · Unchanged under clarified scope (deferred) · `internal/ui/web/transcript/transcript-tree.js:86`

A run closes only at a text, reasoning, error, or attention row. When a turn ends with tool calls and no closing row, such as an interrupted turn, the run stays open. Before this change the next user message closed it. Now the user message passes over it, so the next turn's calls join the old run above that message, and the run shows "Working…" while the agent is idle. When the next turn finally produces text, "Worked for Ns" counts the idle time between turns.

This follows from the specified behavior. It is a limitation of the clarified scope, not a defect against it: the user told the planner "keep it how it is, but ignore user messages" after turn-end and backend alternatives were discussed.

### Evidence

Ran `transcriptRows` on an interrupted call at t=1s, a user message at t=4h, a call at t=4h+1s, and text at t=4h+5s. The result was one run with 2 calls, then the user row, then the text row, with duration `Worked for 14404s`.

### Disposition

Unchanged under the clarified scope, and deferred. The parent session assessed it as a valid limitation and presented it to the user. Closing runs on a turn-end signal from the backend is out of scope for this change.

## Checks run

- `bun test web/transcript` in `internal/ui`: 43 pass, 0 fail, on the first version. After the feedback delta, `bun test` over all of `internal/ui`: 173 pass, 0 fail.
- A fresh `bun build.js` into a temporary directory matches `internal/ui/dist` after the feedback delta.
- Searched every other use of the chevron and summary classes. The tool chevron's rotation selector uses child combinators, so nested call chevrons do not rotate with the run.
- Traced every consumer of `transcriptRows` and run rows (`transcript-item.jsx`, `transcript.jsx`, `transcripts.js` and its `ancestorKeys` expansion for attention requests). None reads `running`, and the user-row change does not affect them.
- Confirmed `Spinner` is still used by `ToolCall`, and that `docs/usage.md` is the only document describing runs.
- The one-off reproduction script for F1 was run outside the repository and deleted.

## Verdict

The implementation matches the plan and the clarified behavior. The chevron and line-height feedback is consistent with the existing Thinking disclosure, which already ends in its chevron. Tests cover the user and from-session cases, closing by each other row kind, and completed groups staying closed. One medium finding, F1, is a limitation of the clarified scope and is left unchanged and deferred.
