# Tool runs pass over user messages

## Goal

A run of consecutive tool calls closes at the next transcript item that is not
a tool call. A user message, or a message delivered from another session,
arrives while the agent keeps working, so today it closes the run early and
splits one stretch of work into two runs. User items must leave runs alone.

When done:

- A user item neither closes nor splits a run. It draws in its place, and calls
  that arrive after it join the run above it.
- A run still closes at the next other item that is not a tool call (text,
  reasoning, an error, or an attention request), with its duration ending at
  that item.
- An open run has no spinner and shows two lines: a chevron and "Working…",
  then, indented, the latest call's title and shortened input.
- A closed run shows one line: a chevron and "Worked for 24s".

## Related code

- `internal/ui/web/transcript/transcript-tree.js` — `transcriptRows` groups
  calls into runs and computes `closed` and `duration`; `status` items are
  already skipped without splitting runs.
- `internal/ui/web/transcript/tool-items.jsx`, `tool-items.css` — `ToolRun`
  draws the run summary.
- `internal/ui/web/transcript/transcript-tree.test.js`,
  `transcript.test.jsx` — tests for runs and their rendering.

## Decisions

- **A user item is drawn but does not touch the open run.** In `rowsOf`, a
  `user` item adds its row and leaves `run` as it is. The run row stays where
  its first call was, so later calls grow the run above the user message. This
  covers the user's own messages and messages delivered from other sessions,
  which are both `user` items.
- **Spinners stay on individual calls.** Only the run summary loses its
  spinner; expanded calls keep theirs.

## Test plan

- `transcript-tree.test.js`:
  - Calls, a user item, then calls form one open run followed by the user row;
    the run lists every call.
  - The same with a message from another session (`user` with `from`).
  - Text after that closes the run, with the duration ending at the text.
  - A user item after a closed run leaves it closed; calls after the user item
    start a new run.
- `transcript.test.jsx`: an open run shows no spinner, "Working…", and the
  latest call's title on a second line; a closed run shows only its duration.

## Implementation plan

1. `internal/ui/web/transcript/transcript-tree.js`: in `rowsOf`, push a `user`
   item's row without closing or clearing `run`. Remove `running` from run rows
   and update the file's opening comment.
2. `internal/ui/web/transcript/tool-items.jsx`, `tool-items.css`: `ToolRun`
   renders "Working…" with the latest call's `Title` on an indented second line
   while open, and the duration alone when closed, with no spinner. Update the
   file's opening comment.
3. Update the tests above.

## Documentation updates

- `docs/usage.md`: the run shows "Working…" and the latest call until the next
  item that is not a tool call or a message to the agent, then "Worked for 32s".
