# Always allow a tool

## Goal

A tool approval request offers, beside its usual choices, **Always allow in
this session** and **Always allow in all sessions**. The choice saves a grant
for that tool and allows the request. A later approval request for a granted
tool is allowed by Hypercube before it is published: no attention flag, card,
notification, or transcript entry for the request, while the tool's own item
stays in the transcript. Grants are per harness. A session grant belongs to
one Hypercube session and survives that session's restarts; an all-sessions
grant covers every session of that harness in every workspace. Questions,
forms, and other attention requests behave as before.

Plugins name the tool an approval asks about; the host owns grants and
answers. Sessions launched before the change need a restart to load the
updated plugins; until then their approvals offer no Always allow choices.

## Native request identities

- **Claude Code.** A `can_use_tool` control request names the tool in
  `tool_name` (`Bash`, `Edit`, `mcp__server__tool`, ...). Every request except
  `AskUserQuestion` is an approval; the grant key is `tool_name`.
- **Codex.** The request method identifies the action: command execution
  (`item/commandExecution/requestApproval`), file changes
  (`item/fileChange/requestApproval`). The grant keys are `commandExecution`
  and `fileChange`. A command
  request with `networkApprovalContext` asks about a host, not a tool, and has
  no key. A file request with `grantRoot` asks about writes under a directory,
  and an `item/permissions/requestApproval` request asks for arbitrary
  permissions; neither identifies a tool and neither gets a grant key.
  An MCP tool approval arrives as `mcpServer/elicitation/request`, whose
  params carry `serverName`, `threadId`, `turnId`, and a request with `message`,
  an empty form, and `_meta` holding `codex_approval_kind: "mcp_tool_call"`,
  `tool_title`, `tool_description`, and `tool_params`. Codex 0.164
  (`codex-rs/core/src/mcp_tool_call.rs`,
  `build_mcp_tool_approval_elicitation_meta`) never sends the tool's name, and
  the app-server drops the elicitation's own ID, which embeds the call ID
  (`codex-rs/app-server/src/bespoke_event_handling.rs`). `tool_title` is display
  text. Codex MCP tool approvals therefore have no key and offer no Always
  allow. Per-launch `mcp_servers.<server>.tools.<tool>.approval_mode` would
  enforce a grant natively, but a grant cannot be created from a request that
  does not name its tool.
- **OpenCode** allows every permission request already.
- **Pi** has no tool approvals; its attention items are extension dialogs.

## Related code

- `internal/harness/protocol.go` — `Item`; gains `Grant`.
- `plugins/claude/main.go` — `control` builds permission items.
- `plugins/codex/requests.go` — `commandRequest`, `fileRequest`.
- `internal/session/event.go` — `eventLocked` opens, publishes, and saves items.
- `internal/session/grants.go` (new) — save grants and record automatic answers.
- `internal/session/input.go` — existing `Answer` records a durable `CmdAnswer`.
- `internal/session/commands.go` — `record`, `send`.
- `internal/store/store.go` — schema and session queries.
- `internal/ui/actions.go`, `internal/ui/web/api.js`,
  `internal/ui/web/transcript/attention-form.jsx` — answering in the browser.
- `internal/harness/harnesstest/harnesstest.go` — the plugin stand-in.

## Decisions

- **Host-side grants.** Matching happens in `eventLocked`, which already sees
  every item before publication, and the answer is an ordinary durable
  `CmdAnswer`. Plugins hold no grant state, the protocol gains one item field
  and no command, and a new grant takes effect at once in every running session.
- **`Item.Grant`.** An approval attention item sets `Grant` to the harness's own
  name for the tool it asks to use. The first of its choices allows the request
  once, and Hypercube answers a granted request with it. Keys are compared
  exactly and never derived from prompt text.
- **Storage.** A `grants` table: `session TEXT REFERENCES sessions(id) ON DELETE
  CASCADE` (NULL for all sessions), `harness TEXT NOT NULL`, `tool TEXT NOT
  NULL`. A request is granted when a row matches its tool and the session's
  harness, with the session's ID or NULL. `CREATE TABLE IF NOT EXISTS` in
  `schema` upgrades existing databases.
- **Hidden auto-answers.** A granted request never enters `live.pending`, so it
  is neither published, checkpointed, mirrored, nor counted as attention.
  Plugins retain `Grant` on completion items. A completed approval with `Grant`
  and no pending entry is dropped, including after app restart. No additional
  live state is needed. Only new, unfinished requests from launches that are
  not stopping are automatically answered.
- **Granting answers what is open.** Saving a grant answers every open request
  it matches: the one clicked, and others the same tool has open, in this
  session or, for all sessions, in every running session of that harness.
- **Session scope.** "This session" is the session that asked; a child's grant
  does not cover its parent or siblings.

## Implementation plan

1. `internal/harness/protocol.go`: add `Grant string \`json:"grant,omitempty"\``
   to `Item` and document it in the item comment.
2. `internal/store/store.go`: add the `grants` table to `schema`; add
   `AddGrant(session, harness, tool string) error` (an empty session means all
   sessions; an existing match is kept) and `Granted(session, harness, tool
   string) (bool, error)`.
3. `internal/session/grants.go`: keep grant lookup and command recording in
   this file, reusing `record`. Automatic answers have no waiting caller;
   remove their entries from `l.waiters` after recording them.
4. `internal/session/event.go`: in `eventLocked`, for a new, unfinished
   attention item with `Grant`, read the session's harness and `Granted`;
   when granted, skip it and collect a `CmdAnswer` with its first choice.
   Drop a finished attention item with `Grant` and no pending entry. Record
   the collected answers in the event's save transaction (count them in the
   early-return check) and start `go r.send(l)` after it commits. Do not
   automatically answer while the launch is stopping.
5. `internal/session/grants.go`: add `AlwaysAllow(id, requestID string, all
   bool) error`. It requires an open attention item with `Grant`, saves the
   grant for the session (or all sessions of its harness), and in the same
   transaction records a `CmdAnswer` with the first choice for every open item
   the grant matches across running launches. It sends those commands and waits
   on the clicked one like `Answer`.
6. `plugins/claude/main.go`: set `Grant: req.Tool` on permission items.
7. `plugins/codex/requests.go`: set `Grant: "commandExecution"` on command
   items without `networkApprovalContext` and `Grant: "fileChange"` on file
   items without `grantRoot`. Leave permissions, questions, and MCP elicitations
   unchanged.
8. `internal/ui/actions.go`: action `allow` with `request` and `scope`
   (`session` or `all`) calls `AlwaysAllow`. `internal/ui/web/api.js`:
   `alwaysAllow(store, session, request, all)`.
9. `internal/ui/web/transcript/attention-form.jsx`: when `item.grant` is set,
   add "Always allow in this session" and "Always allow in all sessions"
   buttons after the choices, using the same sending state.
10. `internal/harness/harnesstest/harnesstest.go`: `!approve` opens an
    attention item `approval-N` with `Grant: "Tool"` and choices Allow and
    Deny, and finishes it with the answer when answered.

## Test plan

- `internal/store/store_test.go`: `Granted` matches a session grant only for
  that session and harness, an all-sessions grant for every session of its
  harness, and neither for another harness; deleting a session deletes its
  grants.
- `internal/session/input_test.go`: with the stand-in, `AlwaysAllow` for the
  session answers the request with Allow; a later `!approve` is answered
  without attention, a request in the transcript, or a published item;
  another session of the same harness still asks; after `AlwaysAllow` with all
  sessions it does not. A session grant still applies after the session
  restarts. `AlwaysAllow` on an item without `Grant`, or a finished one, fails.
  Two open requests for the same tool are both answered by one grant.
- `plugins/claude/main_test.go`: a permission item carries `tool_name` as
  `Grant`; an `AskUserQuestion` item has none.
- `plugins/codex/main_test.go`: command and file change items carry their
  keys; network approvals, grant-root approvals, permissions, elicitations,
  and questions have none.
- `internal/ui/web/transcript/transcript.test.jsx`: an item with `grant` shows
  both Always allow buttons and sends the chosen scope; an item without it
  shows neither.

## Documentation updates

- `docs/harness-support.md`: the attention request bullet gains `grant`; the
  Claude Code and Codex approval bullets name their grant keys, and the Codex
  bullet says MCP tool approvals offer no Always allow because Codex does not
  name the tool.
- `docs/usage.md`: one sentence after the approval sentence on Always allow and
  its two scopes. The file has unrelated staged edits; leave them intact.
