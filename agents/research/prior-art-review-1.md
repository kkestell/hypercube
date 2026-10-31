# Design review against prior art, first pass

The report of a Hypercube worker (GPT 6.1 Sol, high effort), 2026-10-30, as
sent. It reviews `agents/agent-harness-plugins.md` as it stood then. [V] means
verified from current upstream docs or source, [I] means the reviewer's
inference or recommendation, not an experiment.

Verdict: KEEP executable adapters for native terminal harnesses, but CHANGE
the protocol before implementing it. The strongest problems are delivery
guarantees the hooks cannot provide, invisible missing idle-wake support,
uncorrelated attention events, and approval choices flattened below ACP. ACP
plus an owned TUI is a real competing architecture, not something ruled out
by "no terminal."

## 1. Unit of integration: KEEP executables; CHANGE the rationale and alternatives

- [V] Herdr's manifests override screen-detection rules only for already
  recognized agents; a new screen-detected harness still requires a binary
  update. But agents can independently report state AND resume argv without
  a Herdr change. Thus the blanket "No other orchestrator … loads integrations
  it does not ship" is not sustainable:
  https://herdr.dev/docs/agents/#detection-manifests and
  https://herdr.dev/docs/add-herdr-support/. Herdr workflow plugins are
  manifests PLUS arbitrary executable commands, not a demonstration that data
  alone replaces behavior: https://herdr.dev/docs/plugins/.
- [V] ACP registry data declares distribution/command/args/env; runtime
  behavior lives in the agent/adapter executable:
  https://github.com/agentclientprotocol/registry/blob/main/agent.schema.json.
  Claude's adapter wraps Agent SDK; current codex-acp wraps App Server; pi-acp
  spawns `pi --mode rpc`: https://github.com/zed-industries/claude-agent-acp,
  https://github.com/agentclientprotocol/codex-acp,
  https://github.com/svkozak/pi-acp. pi-acp explicitly accepts but does NOT
  wire session MCP servers, so ACP adapter availability does not prove
  Hypercube compatibility.
- [V] Missed prior art: Toad already provides a terminal UI, concurrent
  agents, resume, shell integration, and an agent catalog via ACP:
  https://github.com/batrachianai/toad. acpx offers persistent multi-agent ACP
  sessions, custom adapter commands, permissions, queues and embedding:
  https://github.com/openclaw/acpx.
- [I] Replace the universal novelty claim with the narrower native-TUI
  requirement. Say "ACP does not preserve the harness's native terminal
  interface," NOT "ACP gives no terminal." ACP plus one Hypercube terminal
  frontend could eliminate most per-harness Hypercube code, at the cost of
  owning prompt editor, conversation rendering, slash commands, approvals,
  auth and adapter gaps. That is a product tradeoff, not an impossibility.
  Keep executable translators because hook response translation and
  extension code are real behavior; do not invent a manifest interpreter.
  Built-ins remain simpler if third-party independently shipped support is
  not actually a goal.

## 2. Events: CHANGE lifecycle and correlation; KEEP a small surface

- [V] Herdr reports `idle|working|blocked`, separates session identity from
  state, and recommends monotonic `seq` to reject delayed reports, including
  across restarts: https://herdr.dev/docs/add-herdr-support/ and
  https://herdr.dev/docs/socket-api/#agent-state-reporting. ACP has
  ID-addressed tool lifecycle updates; turn completion reasons include
  `end_turn|max_tokens|max_turn_requests|refusal|cancelled`, with
  cancellation explicitly not an error:
  https://agentclientprotocol.com/protocol/v1/prompt-turn and
  https://agentclientprotocol.com/protocol/tool-calls.
- [V] Hooks have NOT uniformly converged on full lifecycle coverage: Herdr
  deliberately uses Claude/Codex/Devin/Droid hooks only for identity and
  retains screen state detection because interruption/permission
  cancellation coverage is incomplete: https://herdr.dev/docs/integrations/.
  Pi distinguishes low-level `agent_end`, final actionable
  `agent_before_settle`, and final `agent_settled`; aborted settled events
  have `aborted`; extension UI has `ui_prompt_start/end`:
  https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md.
- [I] Concrete changes:
  - (a) Give `start` a conversation ID and update it immediately on
    clear/resume/fork, not only at the next prompt; otherwise quitting after
    switching sessions resumes the old one.
  - (b) Specify a launch generation and ordering rule for state-changing
    reports, with a source sequence when concurrent/background reporting is
    allowed; don't globally discard a delayed context-bearing hook solely
    because it is old.
  - (c) `ask`/`answered` need a request ID (or an explicit aggregate
    blocked-state contract). One question/subagent answer must not clear
    another pending question. The status-line-redraw-means-answered shortcut
    is especially weak.
  - (d) Clarify that `stop` means final ready-for-next-input, not low-level
    model-run end, and preserve the stop reason where available. `tool`
    should mean an injection boundary, not promise a complete tool execution
    transcript. Keep `tool-failed`/`denied` explicitly diagnostic, not
    lifecycle state. No need to import ACP's plans, thought chunks, or diffs
    just to maintain a pane and queue.

## 3. Delivery and the waiter: KEEP hook replies and pull wait as one implementation; CHANGE guarantees

- [V] Claude documents asyncRewake wakeups, BUT timeout is still enforced on
  asyncRewake (unlike ordinary async hooks); command hook default timeout is
  600 seconds. It also limits additionalContext/plain stdout strings to
  10,000 characters and replaces oversized output with a file path and a
  2,000-character preview without instructing the model to read it. Stop
  blocks are overridden after eight consecutive continuations, reset by tool
  calls: https://code.claude.com/docs/en/hooks.md (Hook handler fields, JSON
  output, Stop, Run hooks in the background).
- [I] These are ordinary-use delivery failures: a lead idle more than 10
  minutes can lose its only waiter unless launch explicitly handles that
  timeout; a research report batch can silently become an unread file after
  being dequeued; a Stop reply can be ignored at the loop cap. Add these to
  experiments and Claude implementation requirements, and do not call the
  stdout-write boundary "delivered." At minimum document it as handed to the
  plugin, expose/report injection failure, and define batching by the
  channel's size budget. If reliable delivery is required, add message IDs
  plus acknowledgement after native acceptance; writing stdout is not
  acceptance. Do not claim exactly-once model receipt.
- [V] Pi's `sendUserMessage` explicitly rejects a message if streaming
  without `deliverAs`; it offers `steer` or `followUp` (same Pi extensions
  URL). [I] The native call must choose a safe queue mode even after `wait`
  marked the agent idle: the user can submit between wait completion and
  injection. Do not delete the text and then treat that normal race as
  successful delivery.
- [V] Codex officially supports a native TUI attached to App Server
  (`codex --remote`); App Server exposes `turn/start`, `turn/steer`,
  `thread/inject_items`, status and approvals:
  https://developers.openai.com/codex/app-server/. This disproves the broad
  claim that all server modes mean no native terminal (multi-client
  same-thread ownership and draft behavior still need an experiment).
  agent-deck uses OpenCode session API queueing and refuses unconfirmed
  delivery without falling back to keys:
  https://github.com/asheshgoplani/agent-deck. Herdr prompt is input-based
  but rejects already blocked agents and can atomically submit and wait:
  https://herdr.dev/docs/socket-api/. cmux's generic
  `surface.send_text`/`send_key` are terminal input, not confirmed agent
  delivery: https://www.cmux.dev/docs/api. acpx uses a single live session
  owner plus local IPC and acknowledges queue admission separately from
  completion: https://github.com/openclaw/acpx/blob/main/docs/sessions.md.
- [I] Retain "never type into drafts or dialogs," but let a plugin use an
  attached server or control API instead of requiring hooks and wait as the
  only control path. Record submission, acceptance, and completed turn
  separately. Add the Codex remote-TUI experiment rather than dismissing
  server modes categorically.

## 4. Approvals: CHANGE fixed decisions to plugin-supplied choices; KEEP core-owned UI and the blocking process

- [V] ACP supplies `optionId`/`name`/`kind` (`allow_once|allow_always|reject_once|reject_always`),
  returns the selected `optionId` or cancelled, and assigns
  remembered-decision scope to the agent, not the client:
  https://agentclientprotocol.com/protocol/tool-calls. Vibe Kanban has
  request IDs, create-then-wait, cancellation tokens, approval timeout and a
  separate question path:
  https://github.com/BloopAI/vibe-kanban/blob/main/crates/executors/src/approvals.rs.
  Its ACP translation maps Approved to an AllowOnce option and errors when
  none exists, direct evidence of the limitation of flattening choices:
  https://github.com/BloopAI/vibe-kanban/blob/main/crates/executors/src/executors/acp/client.rs.
- [I] `approve` input has `options: [{id, label, kind}]`; the reply is
  `{outcome: "selected", option: id}` or cancelled, with defer as a separate
  host fallback only when a native fallback exists. Keep a denial message as
  an optional separate feature. Never synthesize "allow once" if the harness
  only offered a scoped or persistent alternative. Keep summary and detail
  authored by the trusted plugin, not model output; show commands and paths
  without hiding them. Killing `approve` withdraws the host UI but is NOT
  literally equivalent to ACP's required cancelled response: require plugin
  cleanup to cancel or unblock its native request, and forbid a late
  decision after cancellation. Generic choice rendering also avoids
  designing a second incompatible UI for questions.

## 5. Capabilities: ADD a small declaration

- [V] ACP requires checking load/resume capabilities before attempting them:
  https://agentclientprotocol.com/protocol/session-setup. Vibe Kanban
  declares capabilities and exposes per-executor support:
  https://github.com/BloopAI/vibe-kanban/blob/main/crates/executors/src/executors/mod.rs.
  Herdr distinguishes authoritative state reporting from screen fallback and
  exposes warnings and explain: https://herdr.dev/docs/agents/.
- [I] The design says an idle lead is the critical path and a missing waiter
  leaves it unreachable. The user and the lead choose profiles, even if the
  event-handling core is branch-free. Never observing an event cannot
  distinguish unsupported from broken. `check` or `describe` should return
  plugin version, supported protocol, effective `idle_wake`, `attention`,
  `approvals`, and `interrupt` support, and warnings (including
  not-applicable versus unsupported for Pi, which has no permission system).
  Surface these in profile checks and `list_agents`, and warn when a lead is
  selected without wake. Declare only facts needed to explain real degraded
  behavior. Required features remain required.

## 6. Discovery, versioning, and failure: KEEP PATH naming; ADD diagnostics and fail-fast compatibility

- [V] `kubectl plugin list` shows resolved executable paths, non-executable
  candidates, and shadowed duplicates:
  https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/. Git
  ignores unsupported operations and unrecognized returned attributes:
  https://git-scm.com/docs/gitcredentials. Docker does NOT ignore unknown
  operations: `HandleCommand` returns `unknown action`, `Serve` exits 1, and
  failures are printed on stdout:
  https://github.com/docker/docker-credential-helpers/blob/master/credentials/credentials.go.
  kubectl passes argv and env as-is; it does not standardize JSON stdin and
  stdout or operation arguments.
- [V] go-plugin has protocol-version mismatch errors:
  https://github.com/hashicorp/go-plugin. MCP negotiates version and
  capabilities, recommends request timeouts, and on stdio shutdown closes
  stdin then escalates SIGTERM and SIGKILL:
  https://modelcontextprotocol.io/specification/2025-06-18/basic/lifecycle.
  LSP `initialize.processId` provides parent-death behavior, but LSP also
  defines `shutdown` and `exit`:
  https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/.
- [I] List plugins with resolved path and shadow warnings (especially the
  sibling-directory precedence); launch and hooks should reuse the resolved
  absolute plugin path, not rediscover a possibly different executable. Add
  a single protocol revision checked during check and launch, with an
  explicit incompatible-plugin error. Unknown optional fields may be
  ignored, but adding a required meaning cannot be made safe by ignoring it.
  Reject unknown commands, events, and invalid replies. Specify check and
  launch deadlines, pure JSON stdout, errors that name executable, operation,
  and exit status or signal, and cleanup ownership. Do not automatically
  retry launch or approval side effects.

## 7. Other: CHANGE certainty claims; ADD extension coexistence and lifecycle requirements

- [V] Current Pi docs settle some "unknown" table entries:
  `agent_settled.aborted` identifies interruptions; `ui_prompt_start/end`
  reports extension questions despite no built-in permission system;
  `sendUserMessage` explicitly starts a turn when idle. Its resources must be
  closed on `session_shutdown`, which also happens on reload, new, resume,
  and fork (Pi extensions URL above). [I] Change the Pi mapping accordingly,
  but keep an experiment for actual draft preservation.
- [V] Herdr's OpenCode integration tracks the selected root session and
  descendants, pending forms and permissions, retries, and multiple panes
  sharing a server, and moved V2 reporting into the pane-local TUI for that
  reason: https://herdr.dev/docs/integrations/#opencode. [I] Add a
  session-change cleanup rule: retire the old waiter and approvals and
  replace the conversation binding even when the process stays alive.
  "Plugins hold no state" is false: OpenCode needs first-prompt tracking and
  live waiter ownership, and Pi extension runtimes own resources. Say plugins
  do not own the durable message queue.
- [V] Claude hooks merge with user and project hooks, run matching handlers
  in parallel, and can be disabled or restricted by managed settings;
  `PermissionRequest` does not cover sandbox network prompts, which need
  `permission_prompt` notifications: https://code.claude.com/docs/en/hooks.md.
  [I] An installed supported version is not working instrumentation. Add a
  visible initialization or health failure if required integration never
  becomes available; wire the additional attention sources or advertise
  partial attention. "Nine of ten offer this lifecycle" conflates the
  existence of some hooks with all required injection, identity, and control
  behavior.
- [V] Claude Squad runs arbitrary configured programs in tmux and worktrees;
  ccmanager has command presets and a conservative manual fallback on
  approval errors: https://github.com/smtg-ai/claude-squad and
  https://github.com/kbwo/ccmanager. [I] They support keeping the native UI
  and a user fallback, not the claim that their integration mechanism or
  failure semantics match the proposed protocol.

## Bottom line

The executable boundary is proportionate for preserving native terminal
interfaces. The surrounding claims of universality, guaranteed delivery, and
unnecessary support metadata are what prior art does not justify. The
highest-priority revisions are delivery and timeout semantics (3),
option-preserving approvals (4), visible degradation (5), and session and
request identity (2).
