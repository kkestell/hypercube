# Profiles without roles, and messaging between any running sessions

## Goal

Two changes, implemented in sequence by two children.

**Part A, profiles.** A profile no longer has a role. Every configured profile
can start a top-level session from the workspace's `+` picker, and every
profile can be passed to `create_session` for a child. The picker shows each
profile on two lines: harness · model · effort, then the description cut to
one line. The `opus` and `sol` lead profiles leave the example config and the
user's config. Saved sessions keep their own harness, model, and effort, so
they are unaffected.

**Part B, messaging.** Any running session's agent can message any other
running session's agent: its children, its parent, siblings, other families,
and sessions in other workspaces of the same app. A parent still addresses its
own children by name; every session is addressable by ID. A message to a
stopped or ended session is refused instead of queued. `list_sessions` lists
the running sessions the caller can message, with each one's workspace and
parent. Messages are queued and delivered on the existing path.

## Related code

- `internal/harness/config.go` — `Profile.Role`, `RoleLead`, `RoleGeneral`,
  `Leads`, `ChildProfiles`, `Lead`, `ChildProfile`, and the lead/child counts in
  `LoadConfig`.
- `internal/session/sessions.go:46` — `Create` resolves the lead profile.
- `internal/session/launch.go:193` — `startArgs` passes child profiles to the
  startup instructions.
- `internal/ui/ui.go:30` — `GET /leads` and its `leads` handler.
- `internal/ui/actions.go:85` — `new` action reads the `lead` query value.
- `internal/ui/web/api.js:23` — `fetchLeads`.
- `internal/ui/web/sessions/selection.js:89` — `openUnsent` stores `lead`.
- `internal/ui/web/sessions/session-actions.js:39` — posts `lead`.
- `internal/ui/web/sidebar/sidebar-menus.js` — `showLeads` builds context-menu
  items from descriptions.
- `internal/ui/web/sidebar/sidebar.jsx:75,105` — opens the picker and renders
  the open menu.
- `internal/ui/web/shared/context-menu.jsx` — positioning and dismissal
  (Escape, outside press, window blur) around `vscode-context-menu`, which
  takes one string label per item.
- `internal/ui/web/agent/composer.jsx:57` — joins harness, model, effort with
  `' · '`; the picker uses the same format.
- `internal/store/store.go:306-332` — `Running` and `FindSession`, the current
  messaging scope.
- `internal/session/messages.go` — `SendMessage` queues and calls `Deliver`.
- `internal/mcp/mcp.go` — tool descriptions, `listSessions`, `sendMessage`,
  `createSession`, and the profile enum.
- `internal/report/report.go` — `Format` (message attribution), `Peers`,
  `Instructions`, `writeTeamwork`.
- `docs/configuration.md`, `docs/usage.md:19-21,38`, `README.md:9`,
  `scripts/dev:90-140`.

## Decisions

### Part A

- `Config` keeps `Profiles []Profile`; the role helpers become one lookup,
  `func (c Config) Profile(name string) (Profile, error)`, exact name only,
  error `no profile named %q`. Callers that listed profiles use `cfg.Profiles`.
  `Create` gets no default for an empty name; the browser always sends one.
- `LoadConfig` checks names, uniqueness, descriptions, and the spec as now, and
  requires at least one profile (`at least one profile is required`).
- A leftover `"role"` key in an existing config is an unknown JSON field, which
  `json.Unmarshal` ignores, so old configs keep loading. No migration: sessions
  store their own harness, model, and effort, and the unsent session lives only
  in browser memory.
- One name for the concept: the route becomes `GET /profiles`, the handler
  `profiles`, the browser function `fetchProfiles`, the unsent field and
  `new` query value `profile`, and the sidebar function `showProfiles`.
- The picker is a small dedicated component, `ProfilePicker`, because
  `vscode-context-menu` takes single-string labels. `ContextMenu` keeps its
  look and behavior. The positioning and dismissal code moves from
  `context-menu.jsx` into a shared hook, `usePopup(ref, x, y, onClose)` in
  `shared/popup.js`, used by both, so the two popups close the same way.
- `useSidebarMenu` keeps one open popup. The picker's state is
  `{x, y, profiles, workspace}`; context menus stay `{x, y, items}`. The
  sidebar renders `ProfilePicker` when `menu.profiles` is set, else
  `ContextMenu`. The request counter that cancels a pending fetch is unchanged.
- Each picker row is a `<button>`: first line `harness · model · effort`,
  second line the description in the description color, one line with an
  ellipsis; the button's `title` holds the full description. The first row
  takes focus on open; ArrowUp and ArrowDown move focus between rows; Enter and
  click choose; Escape, an outside press, and blur close. Styling uses the same
  VS Code menu colors and minimum width as `.context-menu`, with a maximum
  width so long descriptions truncate.
- The teamwork text in `report.writeTeamwork` changes its heading from "Child
  profiles. Every child session is general; ..." to "Profiles. Choose the
  profile whose description fits the task, and pass its name to
  create_session as profile:" and lists `cfg.Profiles`.

### Part B

- `FindSession(caller, to)`: a top-level caller's own child by name first (as
  now); otherwise `t.Session(to)` by ID, in any family and workspace. A child
  caller resolves by ID only, so it reaches siblings by ID. The doc comment
  states the new rule.
- `SendMessage` refuses, in this order: blank text, unknown recipient, self,
  an ended child (existing message), and a recipient for which `tx.Running`
  is false: `session %q is stopped; only running sessions receive messages`.
  It then queues and calls `Deliver` exactly as now. Its signature drops the
  `running` result: `SendMessage(from, to, text string) (store.Session, error)`.
  `mcp.sendMessage` replies `Message queued for %s.` with no stopped note.
- A recipient that stops after the message is queued keeps it in its queue
  until its next start, as stopped sessions already do. Running sessions
  waiting for the user after a start (`AwaitingUser`) also hold it, as now.
- New store query `func (t *Tx) RunningSessions() ([]Session, error)`: every
  session that is not ended and whose family is running (top-level `running`,
  or a child whose parent is running), in creation order. It is the single
  definition used by `list_sessions`; it agrees with `tx.Running`.
- `list_sessions` lists `RunningSessions` except the caller. Each entry:
  `to` (the name for the caller's own children, the ID for everyone else),
  `title`, `harness`, `activity`, `workspace` (the workspace path), `parent`
  (the parent's ID, empty for a top-level session), `directory`, `branch`.
  The `ended` field goes, since ended sessions are never listed.
- `send_message` has one description for every caller: "Send a message to any
  running Hypercube session by its ID, or to one of your child sessions by its
  name. list_sessions lists the sessions you can message." A child's version
  appends "Your parent is %s." `list_sessions`' description lists the new
  fields.
- `report.Format` attribution: the sender's address is its name when it is the
  recipient's own child, else its ID (as now). When the sender is neither the
  recipient's parent nor its own child, the first line also identifies it:
  `Message from %s (titled %q, in workspace %s[, a child session of %s]):`.
  The reply line stays `... use the hypercube send_message tool to <address>.`
  The fallback for a sender with no session row (`user`) is unchanged.
- Startup instructions (`report.Instructions`):
  - Child: "Your parent created you: use send_message to %s to report to it.
    You can also message any running Hypercube session by its ID;
    list_sessions lists them." replaces "...is the only session you can
    message...". `reportBack` is unchanged.
  - Top-level: "Children can message only you" and "You can also message the
    other top-level sessions in this workspace by their IDs, and list_sessions
    lists the current ones" become "You can message any running Hypercube
    session, in any workspace, by its ID; list_sessions lists them with their
    workspace and parent."
  - The "Other top-level sessions: ..." line goes, and `report.Peers` with it:
    the list was the workspace's messaging scope, and a list of every running
    session at startup would be stale. The children list and the shared
    checkout paragraph stay.

### Ownership and order

- Part A owns `internal/harness`, the session and UI call sites, the browser
  picker, and the profile docs, plus the profile lines in `internal/mcp` and
  `internal/report` (the enum, `createSession`'s lookup, `writeTeamwork`, the
  test configs). Part B starts from the merged Part A and owns everything else
  in `store`, `session/messages.go`, `mcp`, and `report`. Sequencing this way
  gives each file one owner at a time.
- Part A needs no architecture review and can start now. Part B removes the
  family and workspace boundary on messaging; it should get the independent
  review the hypercube skill requires while Part A runs.

## Test plan

### Part A

- `internal/harness/harness_test.go`: a config of only profiles loads;
  `Profile` finds each by exact name and refuses an empty or unknown name; a
  config whose profiles still have `"role"` loads; `{"profiles":[]}` and `{}`
  are refused; the existing missing-field, duplicate, and broken-JSON cases
  stay. Drop the role, lead, and child-order cases.
- `internal/session` and `internal/ui` tests: `Create` with a configured
  profile name; with an unknown name it fails. Fixture configs lose `role`.
- `internal/mcp/mcp_test.go`: the `create_session` profile enum lists every
  profile, including the one that was a lead.
- `internal/ui/ui_test.go` or the integration test: `GET /profiles` returns
  every profile in file order; `cmd/hypercube/integration_test.go:542` polls
  `/profiles`.
- `internal/ui/web`: `api.test.js` for `fetchProfiles`; `sidebar.test.jsx`
  checks the picker shows each profile's `harness · model · effort` and
  description, that choosing one opens the unsent session with `profile`
  set, that Escape and an outside press close it, and that a pending picker
  is still cancelled; `session-actions.test.js` posts `profile`. Existing
  context-menu tests pass unchanged.

### Part B

- `internal/store/store_test.go` `FindSession`: a parent finds its child by
  name, any session by ID in any workspace and family (including another
  family's child and a session in workspace `v`); a child finds its parent,
  siblings, and other families by ID, and nothing by a sibling's name.
  `RunningSessions` returns running top-level sessions and the non-ended
  children of running families only.
- `internal/session/messages_test.go`: the scope table becomes: allowed to
  every running session by ID and to own children by name, from parents and
  children alike, across workspaces; refused to self, to missing, to ended
  children, and to stopped sessions, with nothing queued for a refused send.
- `internal/mcp/mcp_test.go`: `list_sessions` from a parent and a child shows
  running sessions in another family and workspace with `workspace` and
  `parent`, own children by name, and no stopped or ended sessions; a child
  can message a running peer; a message to a stopped peer fails with
  "stopped".
- `internal/report/report_test.go`: `Format` keeps the short form between
  parent and child and adds title, workspace, and parent for others, with a
  reply address that `FindSession` resolves for the recipient; the
  instructions mention messaging any running session and no longer list
  peers.
- `cmd/hypercube/integration_test.go:380-395`: replace the "exposed another
  session's child" check with one that a running session in another family
  is listed and reachable.

## Implementation plan

### Part A (first child)

1. `internal/harness/config.go`: delete the role constants, `Role`, and the
   four helpers; add `Profile`; update `LoadConfig` and the type comments.
   Update `harness_test.go`.
2. `internal/session/sessions.go` (`cfg.Profile`), `launch.go`
   (`cfg.Profiles`), `internal/mcp/mcp.go` (enum from `cfg.Profiles`,
   `cfg.Profile` in `createSession`, the `newServer` comment),
   `internal/report/report.go` (`writeTeamwork` heading and comment). Remove
   `role` from every test config: `internal/session/fixture_test.go`,
   `internal/mcp/mcp_test.go`, `internal/report/report_test.go`,
   `cmd/hypercube/integration_test.go`.
3. `internal/ui/ui.go`: `GET /profiles`, handler `profiles` listing
   `cfg.Profiles`. `internal/ui/actions.go`: read `profile`.
4. Browser: `api.js` `fetchProfiles`; `selection.js` and
   `session-actions.js` use `profile`; new `shared/popup.js` (`usePopup`),
   used by `shared/context-menu.jsx`; new `sidebar/profile-picker.jsx` and
   `sidebar/profile-picker.css`; `sidebar-menus.js` `showProfiles`;
   `sidebar.jsx` renders the picker. Update the tests listed above.
5. `scripts/dev`: drop `claude-lead` and `codex-lead` and every `role` line,
   leaving the four profiles.
6. Docs (below). Run `make check`.

### Part B (second child, after Part A is merged and the review is resolved)

1. `internal/store/store.go`: new `FindSession` rule and `RunningSessions`.
   Update `store_test.go`.
2. `internal/session/messages.go`: the stopped check and the new signature.
   Update `messages_test.go`.
3. `internal/mcp/mcp.go`: tool descriptions, `listSessions` over
   `RunningSessions` with the new entry fields, `sendMessage` reply. Update
   `mcp_test.go`.
4. `internal/report/report.go`: `Format` attribution, `Instructions` text,
   delete `Peers`. Update `report_test.go`.
5. `cmd/hypercube/integration_test.go` messaging checks. Docs (below). Run
   `make check`.

## Documentation updates

- Part A, `docs/configuration.md`: delete the `opus` and `sol` profiles and
  every `"role"` line from the example. Replace the paragraphs after it with:
  every field shown is required; provide at least one profile, with unique
  names; the workspace's `+` picker lists every profile with its harness,
  model, effort, and description, and top-level sessions choose among the same
  profiles for their children; each description says which kind of work the
  profile suits and when to use it. Keep the sentences on saved settings and
  rereading. The parent applies the same edit to
  `~/.config/hypercube/config.json` so it matches the example.
- Part A, `README.md:9`: "profiles and worktree setup". `docs/usage.md:38`:
  "to choose a profile for a new session".
- Part B, `docs/usage.md:19-21`: a parent addresses its children by name; any
  session addresses any running session, in any workspace, by ID; a message to
  a stopped or ended session is refused; `list_sessions` lists the running
  sessions an agent can message, with their workspace and parent. Keep the
  shared-checkout sentence.

## Restart note

After `make install-service`, the new profile picker, profile lookup, message
policy, and attribution apply at once. Active agents keep the startup
instructions and the tool descriptions they started with until their family
restarts.
