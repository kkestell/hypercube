# Profiles without roles, and messaging between running top-level sessions

## Goal

Two changes, implemented in sequence by two children.

**Part A, profiles.** A profile no longer has a role. Every configured profile
can start a top-level session from the workspace's `+` picker, and every
profile can be passed to `create_session` for a child. The picker shows each
profile on two lines: harness · model · effort, then the description cut to
one line. The `opus` and `sol` lead profiles leave the example config and the
user's config. Saved sessions keep their own harness, model, and effort, so
they are unaffected.

**Part B, messaging.** A child can message only its own parent. A top-level
session can message its own children by name and any other running top-level
session by ID, across all workspaces in the same app. Other families' children
remain out of scope. Stopped and ended recipients are refused. `list_sessions`
lists only running addressable recipients, with workspace and parent identity.
Existing queue and delivery behavior remains.

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

- Preserve the existing family rules in `FindSession(caller, to)`: a child
  resolves only its parent by ID; a top-level caller resolves its own child by
  name, otherwise another top-level session by ID. Remove only the workspace
  condition from the latter lookup. Do not expose foreign children.
- `SendMessage` refuses blank text, unknown/out-of-scope recipient, self,
  ended child, and stopped recipient (`tx.Running`). Queue accepted messages
  and call `Deliver` on the existing path. Remove the now-redundant `running`
  result and the MCP reply about delivering later to a stopped session.
- A recipient that stops after acceptance keeps its queued message as today.
  Running sessions awaiting the user also retain existing delivery behavior.
- Use existing `Sessions("")`, `Children`, and `Running` queries to list
  recipients. No global all-session query is needed. A child lists its running
  parent only. A top-level session lists its running non-ended children and
  other running top-level sessions across workspaces. Exclude self.
- Entries keep `to`, `title`, `harness`, `activity`, `directory`, `branch`,
  add `workspace` (path) and `parent` (ID), and drop the redundant `ended` field.
  Own children use their names; top-level peers and a child's parent use IDs.
- Tool descriptions and startup instructions describe the exact scope above.
  Keep child parent-only wording. Remove the stale startup list of other
  top-level sessions and `report.Peers` if no longer used; direct agents to
  `list_sessions` instead. Keep the shared-checkout warning explicitly scoped
  to the same workspace.
- `report.Format` keeps reply addressing and parent/child message attribution.
  A peer message header identifies the sender's title and workspace, with its
  session ID as the reply address. No extra parent clause is needed.

### Ownership and order

- Part A owns `internal/harness`, the session and UI call sites, the browser
  picker, and the profile docs, plus the profile lines in `internal/mcp` and
  `internal/report` (the enum, `createSession`'s lookup, `writeTeamwork`, the
  test configs). Part B starts from the merged Part A and owns everything else
  in `store`, `session/messages.go`, `mcp`, and `report`. Sequencing this way
  gives each file one owner at a time.
- Part A needs no architecture review. Part B removes only the workspace
  boundary between top-level sessions; independently review the revised scope
  before implementation.

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

- Store scope tests: parents find own children by name and peer top-level
  sessions by ID across workspaces; children find only their own parent.
  Refuse sibling, foreign child, and foreign parent access from children,
  and foreign child access from top-level sessions.
- Runtime tests: cross-workspace parent messaging succeeds; stopped, ended,
  missing, self, and out-of-scope recipients fail without adding a message.
  Preserve delivery behavior for accepted messages.
- MCP tests: a parent's list includes running peers across workspaces and its
  own active children only, with workspace/parent identity. A child's list
  contains only its running parent. Stopped/ended sessions are absent.
- Report tests: parent-child attribution stays unchanged; peers carry title,
  workspace, and resolvable ID reply addresses. Instructions preserve child
  restrictions and describe global running top-level peer access.
- Integration tests: reach a running top-level session across workspaces,
  continue rejecting foreign child access, and reject stopped recipients.

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

1. `internal/store/store.go`: remove the workspace filter only from the
   top-level peer lookup. Update `store_test.go`.
2. `internal/session/messages.go`: the stopped check and the new signature.
   Update `messages_test.go` and the signature callers in
   `session/feed_test.go` and `session/children_test.go`.
3. `internal/mcp/mcp.go`: tool descriptions, `listSessions` using existing
   queries and caller scope, new entry fields, `sendMessage` reply. Update
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
- Part B, `docs/usage.md` and `docs/architecture.md`: children message only
  their parent; top-level sessions message their own children and any running
  top-level session across workspaces. Stopped and ended targets are refused.
  `list_sessions` shows only running addressable recipients with workspace and
  parent identity. Remove the obsolete claim that workspace restrictions
  prevent cross-session operations. Keep the shared-checkout warning scoped
  to sessions in the same workspace.

## Restart note

After `make install-service`, the new profile picker, profile lookup, message
policy, and attribution apply at once. Active agents keep the startup
instructions and the tool descriptions they started with until their family
restarts.
