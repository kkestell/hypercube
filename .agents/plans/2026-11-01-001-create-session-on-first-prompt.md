# Create a session with its first prompt

## Goal

Choosing a lead under a workspace's New Session button creates the session at
once: its database rows, its sidebar row, and its lead's harness process. A
session the user leaves without prompting is stopped later by the idle
timeout, and a Codex lead that never received a prompt has no saved
conversation to resume, so the session cannot start again.

After this change, choosing a lead opens an unsent new session in the browser
only. It has a lead pane with an empty transcript and a message field, and
nothing exists on the server. Sending the first prompt creates the session,
starts its lead, and delivers the prompt in one request. Selecting another
session, or opening another new session, discards the unsent one, as VS Code
discards an untitled chat that was never sent.

## Related code

- `internal/ui/static/index.html` — `pickLead` posts `session?action=new` and
  selects the returned ID; `changed`, `attach`, and `render` show the selected
  session's agents; `card` builds an agent's pane and message field; `submit`
  posts `agent?action=input`.
- `internal/ui/ui.go` — `sessionAction` handles `new` by calling
  `Runtime.Create`; `agentAction` parses the input body (`text`, `images`)
  under the `maxInput` limit; `leads` lists the lead profiles.
- `internal/session/session.go` — `Runtime.Create` adds the session and its
  lead, then `start(id, true)` launches the lead and waits for it to be ready.
- `internal/session/runtime.go` — `Runtime.Input` checks the prompt, sends it,
  titles the session through `report.Prompt`, and writes the user item.
- `internal/report/report.go` — `Prompt` releases queued messages and titles an
  untitled session from the lead's first prompt; `title` makes the title.
- `internal/store/store.go` — `Untitled` ("New Session") is shown for a session
  with no title, in `Snapshot`; `report.go` uses it for the session list given
  to agents.
- `cmd/hypercube/integration_test.go`, `internal/ui/ui_test.go`,
  `internal/session/runtime_test.go` — create sessions through the
  `new` action or `Runtime.Create`.

Nothing else creates sessions: the MCP tools add workers to existing
sessions, and the command line has no session commands.

## Decisions

- **The browser holds the unsent session.** It keeps the workspace ID and lead
  profile name in page memory. There is no server-side draft, no new state, and
  no new endpoint; the existing `new` action gains the first prompt.
- **One request creates, starts, and prompts.** `POST
  session?action=new&workspace=…&lead=…` takes the same JSON body as
  `agent?action=input` (`{"text", "images"}`) and returns the session ID after
  the lead has the prompt. Today's `new` already waits for the lead to start, so
  the wait is unchanged.
- **The prompt is checked before anything is created.** An empty prompt or bad
  images is refused with no session made. If the lead then fails to start, the
  request is refused as today, and the session stays listed with the failure in
  its lead's transcript.
- **The session is titled when it is created.** Every session now starts with a
  prompt, so `Create` sets the title from it (an image-only prompt is titled
  "Image"), and the sidebar row shows the title from its first appearance.
  Titling on the lead's first prompt in `report.Prompt`, and the `Untitled`
  fallback, are removed, since no session lacks a title any more.
- **The unsent session has no sidebar row.** It appears only in the main area,
  with nothing selected in the sidebar, as VS Code's new chat does not appear
  in its history until sent. The page title shows the workspace name. The
  bottom shell panel and the right panel's Workers tab are empty, since there
  is no session yet.
- **The lead pane shows the chosen profile.** `GET /leads` also returns each
  profile's harness, model, and effort, so the unsent lead's pane shows the
  same configuration line a running lead shows.

## Implementation plan

1. `internal/report/report.go`: rename `title` to exported `Title`, returning
   "Image" when the prompt has no text line. Reduce `Prompt` to releasing
   queued messages (`s.AwaitingUser = false`) and drop its agent argument if no
   longer used.
2. `internal/store/store.go`: delete `Untitled`; `Snapshot` shows `s.Title`.
   `internal/report/report.go`: the session list uses `s.Title`.
3. `internal/session/runtime.go`: move the prompt checks at the top of `Input`
   (text or image required; `harness.CheckImages`) into a function
   `checkPrompt(text, images)` used by `Input` and `Create`. In `Input`, drop
   the `titled` computation and the read of the agent before the prompt if
   `Prompt` no longer needs it.
4. `internal/session/session.go`: change `Create` to
   `Create(workspaceID, lead, text string, images ...harness.Image)`. It calls
   `checkPrompt` first, sets `s.Title = report.Title(text)` before
   `AddSession`, keeps the lead agent's ID, calls `start(s.ID, true)`, and on
   success calls `r.Input(leadID, text, images...)`. Update its comment.
5. `internal/ui/ui.go`: in `sessionAction`, read the body for `new` with the
   `maxInput` limit and decode it as `agentAction` does (share a small helper
   for reading and decoding the input body), answering 400 for a malformed
   body; pass it to `Create`. In `leads`, add `harness`, `model`, and `effort`
   to each listed profile.
6. `internal/ui/static/index.html`:
   - Add page state `unsent = {workspace, lead}` (or null), with its profile's
     harness, model, and effort from `/leads`.
   - `pickLead`: choosing a lead sets `unsent`, sets `selected` to null,
     clears the hash, and shows the unsent lead pane instead of posting.
   - Show the unsent lead pane through the existing `card`, using a fixed key
     such as `'new'` with `role: 'lead'` and `activity: 'idle'`, so the
     composer, image attachment, and drafts work unchanged. Keep the hypercube
     logo hidden while it shows, and set the page title to the workspace name.
   - `submit`: for the unsent pane, post `session?action=new` with the
     workspace, lead, and `{text, images}` body. While the request runs, show
     the pane's busy indicator and keep Send disabled. On success, if the
     unsent session is still shown, clear `unsent` and its draft and
     `select(id)`. On refusal, restore the text and images as `submit` does now.
   - `select` (clicking a session row) and choosing another lead clear
     `unsent` and its draft and attachments. A state update that no longer
     lists the unsent session's workspace clears it too.
7. Tests and docs below.

## Test plan

- `internal/session/runtime_test.go`: `Create` with a prompt starts the lead,
  delivers the prompt (it is the lead's first input), writes the user item to
  the transcript, and titles the session from the prompt's first line; an
  image-only prompt titles it "Image". `Create` with an empty prompt or a bad
  image is refused and leaves no session or agent in the database and no
  plugin started. Update `fixture.session()` and `TestAgentThatKeepsEndingStaysEnded`
  to pass a prompt, and adjust tests that count the lead's inputs or check the
  title after a later prompt (such as
  `TestRestoredSessionHoldsMessagesUntilThePrompt`), since the creation prompt
  is now the first input and sets the title.
- `internal/report/report_test.go`: replace the titling cases in
  `TestPromptReleasesAndTitles` with cases for `Title` (first line, spacing,
  200-character cut, non-ASCII, no text gives "Image"), and keep a check that
  `Prompt` releases queued messages.
- `internal/store/store_test.go`: the `Snapshot` assertion uses a stored title
  instead of `Untitled`.
- `internal/ui/ui_test.go`: the fixture's `rt.Create` passes a prompt. `POST
  session?action=new` with a malformed body answers 400; with an empty prompt
  answers 409 and creates no session; with a prompt returns an ID whose lead
  received the prompt. `GET /leads` includes harness, model, and effort.
- `cmd/hypercube/integration_test.go`: `client.post` takes an optional body,
  and creating a session sends a prompt. The unknown-lead check stays a 409.
  Assertions on `AwaitingUser` and worker briefs keep their meaning.

## Documentation updates

- `docs/usage.md`: the paragraph on titles says a session is titled with the
  first line of the prompt that creates it, cut to 200 characters. The Browser
  view paragraph says choosing a lead profile opens a new session, and sending
  its first prompt creates it; leaving it before sending discards it. Edit only
  these lines; the file has other uncommitted changes.
