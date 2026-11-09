# Title sessions from their first prompt

Builds on `2026-10-31-001-web-ui-only.md`.

## Goal

A new session has no title and shows as "New Session". The first prompt the
user sends its lead becomes the session's title: the prompt's first line, cut
to 40 characters. A title the user sets is never replaced, and later prompts
leave the title alone.

## Related code

- `cmd/hypercube/main.go` — `openWorkspace` gives a new lead the first unused
  `Session N` title; `retitle` changes a title and renames the window.
- `internal/store/store.go` — `Agent.Title`; `State.Title` returns a session's
  title or its ID.
- `internal/harness/harness.go` — `Launch` names a new window
  `cmp.Or(a.Title, a.Team)`.
- `internal/report/report.go` — `Run` handles the `prompt` report from every
  plugin; `input` is the report's body.
- `internal/wait/wait.go` — `Wait` marks the agent busy before the harness
  starts a turn for delivered messages.
- `internal/tmux/tmux.go` — `FindWindow` and `RenameWindow`.
- `plugins/claude/main.go`, `plugins/codex/main.go` — `hook` builds the
  `prompt` report body from `UserPromptSubmit`; `hookInput` holds the hook's
  input.
- `plugins/opencode/opencode.js` — the `prompt` hook; `event.prompt.text` is
  the prompt before Hypercube appends context.
- `plugins/pi/pi.js` — the `before_agent_start` handler; a user event's
  `event.prompt` is the prompt text.
- `cmd/hypercube/integration_test.go` — `TestHarnessPlugin` is the stand-in
  harness; `TestTmuxNativeLifecycle` checks window names.

## Decisions

- **Untitled:** an empty `Agent.Title` means the session is untitled.
  `store.Untitled = "New Session"` is what an untitled session shows: its
  window name and `State.Title`. The `Session N` numbering goes away.
- **Which prompt titles the session:** the first `prompt` report for an
  untitled lead that arrives while the lead is idle. A prompt the user types
  starts a turn from idle. A turn started by delivered messages arrives after
  `Wait` has already marked the agent busy, so wake-ups never title a session,
  whatever the harness does with their text.
- **Plugins send the text of every prompt** as `text` in the `prompt` report
  body. Hypercube decides whether it becomes the title.
- **Title text:** the first line that is not blank, with runs of whitespace
  collapsed to one space. Longer than 40 characters, it is cut to 39 and ends
  with `…`. A prompt with no visible text leaves the session untitled.
- **Window rename:** `Run` renames the window inside the same state update
  that saves the title, so a title the user sets at the same moment through
  `retitle` ends as both the saved title and the window name. The rename is
  best effort: a failure leaves the window showing "New Session" and does not
  fail the harness's hook.

## Test plan

- `internal/report`: a `prompt` report with text from an idle, untitled lead
  sets its title to the cut first line. A second prompt does not change it. A
  prompt from a busy lead, from a worker, from a lead with a title, or with
  only whitespace sets none. Titles cover a long line, a multi-line prompt, and
  non-ASCII text cut on a character boundary.
- `TestTmuxNativeLifecycle`: new sessions' windows are named "New Session".
  Typing a line into a new lead's pane renames its window and title to that
  line. A wake-up delivered to an untitled lead leaves it untitled. A session
  renamed through `window rename` keeps its title after the next prompt.
- Live: in each harness, the first prompt titles the session in the sidebar,
  and a lead woken by another lead's message stays "New Session".

## Implementation plan

1. `internal/store/store.go`: add `Untitled`; `State.Title` returns it for a
   lead without a title.
2. `internal/harness/harness.go`: `Launch` names the window
   `cmp.Or(a.Title, store.Untitled)`.
3. `cmd/hypercube/main.go`: `openWorkspace` adds a lead with no title; remove
   the `Session N` loop.
4. `internal/report/report.go`: add `Text string \`json:"text"\`` to `input`.
   In the `prompt` case, before marking the agent busy, title an idle, untitled
   lead with `title(in.Text)` and rename its window. Add `title`.
5. `plugins/claude/main.go`, `plugins/codex/main.go`: add `Prompt` to
   `hookInput` and send it as `text` with `UserPromptSubmit`.
6. `plugins/opencode/opencode.js`: send `event.prompt.text`, read before
   context is appended, as `text`.
7. `plugins/pi/pi.js`: send `event.prompt` as `text`.
8. `cmd/hypercube/integration_test.go`: `TestHarnessPlugin` sends the typed
   line as `text` for typed input and no text for wake-ups; apply the test plan.

## Documentation updates

- `docs/usage.md`: a new session shows as "New Session" until its first prompt
  titles it; double-clicking a session still sets its title, after which
  prompts leave it alone.
