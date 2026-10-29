# ACP: what the protocol gives Hypercube

Research for replacing "run each harness's TUI in a PTY" with Hypercube as an ACP client. Scope is the protocol only; which harnesses speak ACP is covered elsewhere.

Researched 2026-10-29 from agentclientprotocol.com. Conventions:

- **[spec](url)** is a fact stated on that spec or RFD page.
- **[unclear]** means the spec is silent or contradictory on it.
- "Implication" lines are my reading for Hypercube, not spec.
- Unless a section says otherwise, "spec" means **protocol v1**, the stable line. v2 is a Draft published 2026-07-20 ([v2 announcement](https://agentclientprotocol.com/announcements/acp-v2-draft.md)). It is called out where it changes the answer.
- Limits: pages were read through a fetch tool that summarizes, and I read the first 100,000 of 158,860 characters of the [v1 schema page](https://agentclientprotocol.com/protocol/v1/schema.md). Method and capability lists come from the prose pages and the schema's first part. Before building on a specific field, check the JSON schema in the repo release.

## 1. Transport and lifecycle

- **Transport.** The protocol "currently defines one transport mechanism": stdio. [spec](https://agentclientprotocol.com/protocol/v1/transports.md)
  - The client launches the agent as a subprocess. The agent reads stdin and writes stdout. [spec](https://agentclientprotocol.com/protocol/v1/transports.md)
  - Messages are UTF-8 JSON-RPC 2.0, newline-delimited, with no embedded newlines. [spec](https://agentclientprotocol.com/protocol/v1/transports.md)
  - stdout carries only ACP messages. stderr is free-form logging that the client may capture, forward or ignore. [spec](https://agentclientprotocol.com/protocol/v1/transports.md)
  - Shutdown is the client closing stdin and terminating the subprocess. This is only in the transport page's diagram; no shutdown method exists. [spec](https://agentclientprotocol.com/protocol/v1/transports.md)
- **Other transports.** Custom transports are allowed if they keep the JSON-RPC format and lifecycle. Streamable HTTP and WebSocket are not in the spec. They are an Active RFD targeting v1 as an additive feature, with a reference implementation in Goose in progress. [spec](https://agentclientprotocol.com/rfds/streamable-http-websocket-transport.md)
- **Who spawns whom.** The client spawns the agent ("typically run as subprocesses of the Client"). The client owns the environment, user interaction and resource access. [spec](https://agentclientprotocol.com/protocol/v1/overview.md)
  - Implication: for Docker sessions Hypercube has to start the agent inside the container and pipe stdio to it, for example `docker exec -i`. The protocol says nothing about this.
- **Handshake.**
  1. The client sends `initialize` with `protocolVersion` (a single integer, major version only), `clientCapabilities` and `clientInfo`. The client MUST send the latest version it supports. If the agent supports that version it echoes it, otherwise it replies with its latest, and the client should close the connection if it can't speak that. [spec](https://agentclientprotocol.com/protocol/v1/initialization.md)
  2. The agent replies with `agentCapabilities`, `agentInfo` and `authMethods`. All capabilities are optional, and an omitted one is unsupported. [spec](https://agentclientprotocol.com/protocol/v1/initialization.md)
  3. The client calls `session/new` or `session/load`. [spec](https://agentclientprotocol.com/protocol/v1/overview.md)
- **Baseline agent support.** Every agent MUST support `session/new`, `session/prompt`, `session/cancel` and `session/update`, and `text` and `resource_link` content in prompts. [spec](https://agentclientprotocol.com/protocol/v1/initialization.md)
- **Auth.** `authMethods` in the initialize response lists the options. [spec](https://agentclientprotocol.com/protocol/v1/authentication.md)
  - `agent` type (the default): the client sends `authenticate {methodId}` and the agent runs its own login.
  - `terminal` type: only offered if the client advertised `auth.terminal`. The client runs the same agent program interactively with extra `args` and `env`, waits for exit status 0, then reconnects and re-initializes.
  - The spec defines no environment-variable method type and does not say where credentials are stored or how they are passed afterwards.
  - Implication: Hypercube's plan to inject API keys as container environment variables sits outside ACP. An agent that finds its credentials already present would be expected to advertise no auth methods or accept `authenticate` trivially, but that is agent behavior. [unclear]
  - `logout` is optional, gated on `agentCapabilities.auth.logout`. What happens to running sessions after logout is undefined. [spec](https://agentclientprotocol.com/protocol/v1/authentication.md)

## 2. Sessions

- **Who picks the id.** The agent does. `session/new` returns a unique `sessionId`, and the client uses it for prompt, cancel, load, resume and close. [spec](https://agentclientprotocol.com/protocol/v1/session-setup.md)
- **`session/new` params.** `cwd` (absolute, and it "MUST be used for the session regardless of where the Agent subprocess was spawned"), `mcpServers`, and `additionalDirectories` if the agent advertises it. The effective root set is `[cwd, ...additionalDirectories]`. [spec](https://agentclientprotocol.com/protocol/v1/session-setup.md)
- **`mcpServers`: can every agent get Hypercube's MCP server this way? Yes, with limits.**
  - **stdio** is the baseline every agent must support. Fields are `name`, `command` (absolute path), `args`, and `env` as an array of `{name, value}`. [spec](https://agentclientprotocol.com/protocol/v1/session-setup.md)
  - **http** (`url`, `headers`) and **sse** need the agent to advertise `mcpCapabilities.http` or `.sse`. SSE is deprecated in MCP. [spec](https://agentclientprotocol.com/protocol/v1/session-setup.md)
  - Agents "SHOULD connect to all MCP servers specified by the Client", and clients "MAY use this ability to provide tools directly to the underlying language model". That is SHOULD, not MUST. [spec](https://agentclientprotocol.com/protocol/v1/session-setup.md)
  - What happens when an MCP server fails to connect is not specified. [unclear]
  - `env` and `headers` values are treated as credentials. Agents should not echo them into updates, errors, `_meta` or logs. [spec](https://agentclientprotocol.com/protocol/v1/session-setup.md)
  - Implication: per-agent identity is easy. Hypercube passes a different `mcpServers` entry per session. With http, put an agent token in `headers`. With stdio, put `HYPERCUBE_AGENT_ID` in `env`, but that needs a small shim binary at an absolute path inside the container. http avoids the shim and needs the capability. Whether an agent actually exposes the tools to the model is only checkable per harness.
  - **MCP-over-ACP** (the client serves MCP tools over the ACP connection itself, `"type": "acp"`) is a draft RFD targeting MCP 2026-07-28. Not usable now. [spec](https://agentclientprotocol.com/rfds/mcp-over-acp.md)
- **Resuming and listing.**

  | Method | Status | What it does |
  |---|---|---|
  | `session/load` | stable, needs `loadSession` | Replays the whole conversation as `session/update` notifications, then returns `{}`. Takes `sessionId`, `cwd`, `mcpServers`. [spec](https://agentclientprotocol.com/protocol/v1/session-setup.md) |
  | `session/resume` | stable since 2026-04-22, needs `sessionCapabilities.resume` | Same params, no replay. Restores context, reconnects MCP servers. [spec](https://agentclientprotocol.com/protocol/v1/session-setup.md), [announcement](https://agentclientprotocol.com/announcements/session-resume-stabilized.md) |
  | `session/list` | stable, needs `sessionCapabilities.list` | Optional `cwd` filter and cursor pagination. Returns `sessionId`, `cwd`, optional `title` and `updatedAt`. [spec](https://agentclientprotocol.com/protocol/v1/session-list.md) |
  | `session/close` | stable, needs `sessionCapabilities.close` | Cancels ongoing work like `session/cancel`, frees resources, idempotent. [spec](https://agentclientprotocol.com/protocol/v1/session-setup.md) |
  | `session/delete` | stable, needs `sessionCapabilities.delete` | Removes the session from future `list` results. Soft or hard delete is up to the agent. Deleting an active session is implementation-defined. [spec](https://agentclientprotocol.com/protocol/v1/session-delete.md) |
  | `session/fork` | RFD proposal only | Creates a new session from an existing one; no capability name is settled beyond `session.fork`. [spec](https://agentclientprotocol.com/rfds/session-fork.md) |

  - On `load` and `resume` the client must resend the full `additionalDirectories`, because omitting it "does not restore stored roots implicitly". Treat `cwd` and `mcpServers` the same way, since they are request params. [spec](https://agentclientprotocol.com/protocol/v1/session-setup.md)
- **Do sessions survive an agent process restart?** The spec does not say. It says only that loading "enables persistence across restarts and sharing sessions between different Client instances", and the security page says it does not state whether sessions persist after the process exits. Where and whether an agent stores sessions is its own business. [unclear] ([session-setup](https://agentclientprotocol.com/protocol/v1/session-setup.md))
- **Several sessions in one process.** Not stated. The spec says sessions give "multiple independent interactions with the same Agent", each with its own state, but never says one connection can hold several at once. [unclear] ([session-setup](https://agentclientprotocol.com/protocol/v1/session-setup.md))
- v2 draft: `session/load` is removed in favor of `session/resume` with `replayFrom`, and `list`, `resume` and `close` become required when `session` is advertised. [spec](https://agentclientprotocol.com/protocol/v2/migration.md)

## 3. Prompt turns

- **Turn.** The client sends `session/prompt {sessionId, prompt: ContentBlock[]}`. The agent streams `session/update` notifications and answers the original request with `{stopReason}` when no tool calls remain. [spec](https://agentclientprotocol.com/protocol/v1/prompt-turn.md)
- **Update kinds** (`sessionUpdate` discriminator) [spec](https://agentclientprotocol.com/protocol/v1/prompt-turn.md):

  | Kind | Meaning |
  |---|---|
  | `user_message_chunk`, `agent_message_chunk`, `agent_thought_chunk` | Streamed text. The optional `messageId` groups chunks into one message. |
  | `tool_call`, `tool_call_update` | Tool calls. Fields below. |
  | `plan` | The agent's plan. |
  | `available_commands_update` | Slash commands changed. |
  | `current_mode_update`, `config_option_update` | Mode or config changed. |
  | `session_info_update` | Title or last-updated changed. |
  | `usage_update` | Context usage and optional cost. |
  | `notice` | Live user-facing notice. Needs client `session.notices`. |
  | `compaction_update`, `compaction_summary_chunk` | Context compaction. Need client `session.compaction`. |

  - Updates are not limited to active turns. Agents can send them after session creation and while replaying history. [spec](https://agentclientprotocol.com/protocol/v1/prompt-turn.md)
- **Tool calls.** [spec](https://agentclientprotocol.com/protocol/v1/tool-calls.md)
  - Fields: `toolCallId`, `title`, optional `name`, `kind`, `status`, `content`, `locations`, `rawInput`, `rawOutput`.
  - `kind` is one of `read`, `edit`, `delete`, `move`, `search`, `execute`, `think`, `fetch`, `switch_mode`, `other`.
  - `status` is one of `pending`, `in_progress`, `completed`, `failed`.
  - `content` is `content` (text, image or resource), `diff` (`path`, `oldText`, `newText`), or `terminal` (`terminalId`).
  - `name` is an optional programmatic tool name, stabilized 2026-09-17, informational only. [spec](https://agentclientprotocol.com/updates.md)
- **Plans.** `plan` carries the agent's execution plan. [spec](https://agentclientprotocol.com/protocol/v1/prompt-turn.md) The entry shape is on the [agent-plan page](https://agentclientprotocol.com/protocol/v1/agent-plan.md), which I did not read.
- **Stop reasons.** `end_turn`, `max_tokens`, `max_turn_requests`, `refusal`, `cancelled`. [spec](https://agentclientprotocol.com/protocol/v1/prompt-turn.md)
- **Cancel.** `session/cancel` is a notification. [spec](https://agentclientprotocol.com/protocol/v1/prompt-turn.md)
  - The client MUST answer every pending `session/request_permission` with the `cancelled` outcome, and SHOULD mark unfinished tool calls `cancelled` locally.
  - The agent SHOULD stop model and tool work promptly, MAY send final updates, and MUST then answer the prompt with `cancelled`.
  - `$/cancel_request` (stabilized 2026-06-29) cancels any request by id. It is optional to implement, and the receiver answers with a normal result or error `-32800`. [spec](https://agentclientprotocol.com/protocol/v1/cancellation.md)
- **Can a prompt be sent while a turn is running? In v1, unclear; in the v2 draft, yes.**
  - The v1 prompt-turn page only says the client may send another prompt after a turn completes. It says nothing on queueing, steering or rejecting. [unclear] ([prompt-turn](https://agentclientprotocol.com/protocol/v1/prompt-turn.md))
  - v2 draft: `session/prompt` resolves when the user message is *inserted* (it returns a `messageId`), not when the turn ends. Clients may send prompts while the session is `running` or `requires_action`, and agents "MUST NOT reject a prompt only because foreground work is active". Whether a held prompt steers the current turn or starts the next one is up to the agent, and there is no capability to advertise which. [spec](https://agentclientprotocol.com/protocol/v2/prompt-lifecycle.md)
  - The v2 prompt RFD says it "does not specify queueing, steering, or whether agents insert new prompts while busy". [spec](https://agentclientprotocol.com/rfds/v2/prompt.md)

## 4. Permissions

- **Flow.** The agent sends the client `session/request_permission {sessionId, toolCall, options}`, and the client answers `{outcome: {outcome: "selected", optionId}}` or `{outcome: {outcome: "cancelled"}}`. [spec](https://agentclientprotocol.com/protocol/v1/tool-calls.md)
- **Options.** Each has `optionId`, `name` and `kind`: `allow_once`, `allow_always`, `reject_once`, `reject_always`. `kind` is a UI hint only. The agent owns remembered decisions and their scope. [spec](https://agentclientprotocol.com/protocol/v1/tool-calls.md)
- **Optional for the agent.** The agent "MAY request permission" before a tool call, and the spec does not say when it must. Clients may auto-allow or auto-reject per user settings. [spec](https://agentclientprotocol.com/protocol/v1/tool-calls.md)
- **Advisory.** Permission requests are advisory, and the agent must honor the decision. ACP does not sandbox the agent, which runs with its process's permissions. [spec](https://agentclientprotocol.com/protocol/v1/security.md)
- **Elicitation.** A separate stable mechanism (`elicitation/create`, stabilized 2026-07-22) lets the agent ask the user for structured input, form or URL. It is gated on the client's `elicitation` capability. [spec](https://agentclientprotocol.com/protocol/v1/elicitation.md)
- v2 draft: `session/request_permission` requires `title` and moves the tool call under `subject`. [spec](https://agentclientprotocol.com/protocol/v2/migration.md)
- Implication: a worker with nobody watching needs a Hypercube-side policy (auto-allow, or route to the lead or the user). Whether an agent asks at all depends on its mode (section 6).

## 5. Client-provided capabilities (fs/*, terminal/*)

- **Optional.** Both are gated on `clientCapabilities`: `fs.readTextFile`, `fs.writeTextFile`, `terminal`. The agent MUST NOT call a method whose flag is false or missing. [spec](https://agentclientprotocol.com/protocol/v1/file-system.md), [terminals](https://agentclientprotocol.com/protocol/v1/terminals.md)
- **If the client doesn't implement them.** The spec states the prohibition and gives no fallback. [unclear] Presumably the agent uses its own file and shell tools, but this is agent behavior, not protocol.
- **What they are.**
  - `fs/read_text_file` takes `sessionId`, `path`, optional `line` and `limit`. `fs/write_text_file` takes `path` and `content`. The point is access to unsaved editor buffers. [spec](https://agentclientprotocol.com/protocol/v1/file-system.md)
  - `terminal/create`, `output`, `wait_for_exit`, `kill`, `release` run commands on the client side. A terminal can be embedded in a tool call and the client shows its output live. [spec](https://agentclientprotocol.com/protocol/v1/terminals.md)
- v2 draft removes the client `fs` and `terminal` capabilities and methods entirely. Client-side tools are exposed through MCP instead. [spec](https://agentclientprotocol.com/protocol/v2/migration.md)
- Implication: Hypercube has no editor buffers, and its agents already run commands in their own environment. Advertising neither capability is both allowed and consistent with where v2 is going.

## 6. Modes, models, slash commands, config options

- **Config options** are the current way (stabilized 2026-02-04). [spec](https://agentclientprotocol.com/protocol/v1/session-config-options.md)
  - The agent returns `configOptions` at session setup. Each has `id`, `name`, `type` (`select`, or `boolean` if the client advertised `session.configOptions.boolean`), `currentValue`, options, and an optional `category`.
  - Defined categories: `mode`, `model`, `model_config`, `thought_level`. Categories are UX hints only. `model_config` stabilized 2026-06-24. [spec](https://agentclientprotocol.com/updates.md)
  - The client changes one with `session/set_config_option {sessionId, configId, value}`, at any time including mid-generation. The agent returns the full option list.
  - The agent can change options itself and sends `config_option_update` with the full state.
  - **Option ids and values are agent-defined.** There is no standard `effort` option; the nearest is `thought_level`, whose values are also agent-defined.
  - Implication: a Hypercube profile (harness, model, effort) cannot be applied by hard-coded ids. After `session/new`, read `configOptions`, find the option by category, and match a value; the mapping is per harness.
  - Whether `session/new` can accept initial config values is not in the pages I read, so assume you must call `set_config_option` after creating the session. [unclear]
- **Modes** (`modes` in the setup response, `session/set_mode`, `current_mode_update`) are older and deprecated: "Dedicated session mode methods will be removed in a future version". Agents should send both while it lasts. The agent can switch its own mode, typically through a tool that first asks permission with kind `switch_mode`. [spec](https://agentclientprotocol.com/protocol/v1/session-modes.md) v2 removes `set_mode` and `current_mode_update`. [spec](https://agentclientprotocol.com/protocol/v2/migration.md)
- **Model.** Only the `model` config category. I found no stable `session/set_model` in the v1 prose pages or the part of the schema I read. [unclear]
- **Slash commands.** `available_commands_update` lists `{name, description, input?.hint}`. Invoke one by putting `/name args` as plain text in the prompt. Only unstructured text input is supported. [spec](https://agentclientprotocol.com/protocol/v1/slash-commands.md)
- **System prompt.** There is no standard way to give an agent a role prompt. A client-provided `systemPrompt` on `session/new` (append or override) is a Draft RFD behind an unstable flag. [spec](https://agentclientprotocol.com/rfds/client-system-prompt.md)

## 7. Status: busy, idle, waiting

- **v1 has no status notification.** [unclear] I found none in the v1 pages. What the client can observe:
  - Busy: a `session/prompt` request is outstanding. Idle: its response arrived (with the stop reason).
  - Waiting for permission: the agent has sent `session/request_permission` (or `elicitation/create`) and the client hasn't answered. The client is the one holding that request, so Hypercube knows by construction.
  - Implication: all three states are derivable from the client's own bookkeeping, as long as nothing happens outside a prompt. Updates can arrive outside turns ([spec](https://agentclientprotocol.com/protocol/v1/prompt-turn.md)), so background agent activity is not covered.
- **v2 draft** adds a `state_update` notification with `running`, `idle` (optional `stopReason`, now including `error`) and `requires_action`. A new session is `idle`; `idle` means no foreground work, though background activity may continue. [spec](https://agentclientprotocol.com/protocol/v2/prompt-lifecycle.md)
- **`session/status` RFD** is a read-only query of one session without attaching to it. It returns `active` or `inactive`, plus the v2 `state`. It is an open proposal that starts behind `unstable_session_status`. [spec](https://agentclientprotocol.com/rfds/session-status.md)
- Not covered by any of these: reattaching to a turn that is running when the client reconnects. `session/status` says `active` does not mean the caller is attached, and the caller must still `load` or `resume` first. [spec](https://agentclientprotocol.com/rfds/session-status.md)

## 8. Multi-agent, multiple clients, extension points

- **Agent-to-agent messaging.** Nothing in the stable protocol. ACP is client to agent only.
- **Subagents.** A draft RFD (`unstable_subagents`) lets one agent announce child sessions on its own stream, each with a session id, so a client can watch them. The client can't prompt or steer them and must not load or resume them. It covers subagents created inside one agent, not independent agents coordinating. [spec](https://agentclientprotocol.com/rfds/subagents.md)
  - Implication: this would show Claude's internal subagents, not Hypercube workers.
- **Multiple clients on one session.**
  - Not addressed by the spec. Over stdio the topology is one client per agent process by construction. [spec](https://agentclientprotocol.com/protocol/v1/transports.md)
  - The remote transport RFD doesn't say whether two clients may attach to one session, and its text on whether sessions survive disconnect contradicts itself. [unclear] ([RFD](https://agentclientprotocol.com/rfds/streamable-http-websocket-transport.md))
  - The session-setup page mentions "sharing sessions between different Client instances" via `session/load`, as sequential handoff rather than concurrent attach. [spec](https://agentclientprotocol.com/protocol/v1/session-setup.md)
  - The security page says agents serving multiple users should scope `list`, `load` and `resume` to the owning user. [spec](https://agentclientprotocol.com/protocol/v1/security.md)
- **Proxy chains** (draft RFD): a conductor sits between client and agent, can add MCP tools and rewrite traffic. Linear chains only, with no multi-client story. [spec](https://agentclientprotocol.com/rfds/proxy-chains.md)
- **Extension points.** [spec](https://agentclientprotocol.com/protocol/v1/extensibility.md)
  - `_meta` on most types, with root keys `traceparent`, `tracestate` and `baggage` reserved. Custom fields must not be added at the root of spec types.
  - Custom methods must start with `_`. An unknown custom request gets `-32601`, and unknown custom notifications are ignored.
  - Custom capabilities go in `_meta` of the capability objects, with reverse-DNS names (`com.example/<name>`, methods `_com.example/<name>`).
  - Implication: `_meta` is the place for Hypercube-specific fields such as sender identity, but the agent only passes through what it chooses to.

## 9. Maturity

- **Versions.** v1 is the stable line. The version is one integer that bumps only on breaking changes. [spec](https://agentclientprotocol.com/protocol/v1/initialization.md)
  - v2 was published as a Draft on 2026-07-20. It does not replace v1, implementers should support both, there is no stabilization date, and it should not ship by default. Schemas are tagged `schema-v2.0.0-alpha.N`. [spec](https://agentclientprotocol.com/announcements/acp-v2-draft.md)
  - v2 rewrites turn and status handling, drops `session/load`, `set_mode` and the client `fs` and `terminal` capabilities, and restructures permissions and diffs. [spec](https://agentclientprotocol.com/protocol/v2/migration.md)
- **Stability guarantee.** Only RFDs in "Completed" carry a stability commitment. Active is "a visibility signal, not a stability commitment". [spec](https://agentclientprotocol.com/rfds/about.md)
- **Stabilized into v1 in 2026** [spec](https://agentclientprotocol.com/updates.md): config options (Feb 4), `session/list` (Feb 18), `session_info_update` and the registry (Mar 9), `session/resume` (Apr 22), `session/close` (Apr 23), `logout` (May 21), `additionalDirectories` (Jun 1), message ids, `usage_update` and `session/delete` (Jun 5), `model_config` (Jun 24), `$/cancel_request` (Jun 29), boolean config options (Jul 6), elicitation (Jul 22), tool call `name` (Sep 17).
- **Still RFD or draft** (as of 2026-10-29):

  | Item | Stage |
  |---|---|
  | v2 | Draft |
  | `session/fork` | Proposal |
  | `session/status` | Open, `unstable_session_status` |
  | Subagents | Draft, `unstable_subagents` |
  | MCP-over-ACP | Draft |
  | Streamable HTTP and WebSocket transport | Active |
  | Client system prompt | Draft |
  | Proxy chains | Proposal, not accepted |

  The v2 prompt RFD is also still active, with revisions dated 2026-10-29. [spec](https://agentclientprotocol.com/rfds/v2/prompt.md)
- **SDKs.** The Rust and TypeScript SDKs reached 1.0.0 on 2026-06-25. [spec](https://agentclientprotocol.com/updates.md)

## Gaps for Hypercube

**Process lifecycle.** ACP stops at the pipe.
- Spawning, picking the command, putting it in the container, passing env, detecting crash or exit (stdout EOF or process exit), and restarting are all Hypercube's. The only protocol-level shutdown is closing stdin and killing the process. `session/close` frees a session, not the process. [spec](https://agentclientprotocol.com/protocol/v1/transports.md)
- Killing "everything the agent owns" is also Hypercube's. ACP's `terminal/release` only covers terminals a client created for the agent. Agent-spawned subprocesses are outside it. The ownership rules in `AGENTS.md` ("When terminals return") still apply, now per agent process instead of per PTY.
- stdio ties the agent's lifetime to Hypercube's. If Hypercube restarts, the agent's stdin closes. What an agent does on stdin EOF is not specified. [unclear] Keeping agents alive across a Hypercube restart, which the design wants since "clones and containers outlive the server process", needs a supervisor Hypercube writes, such as a per-agent shim in the container that holds the pipes, or the remote-transport RFD if it lands.

**Durable sessions across Hypercube restarts.**
- The mechanism is: store `sessionId`, harness and `cwd` per agent in SQLite, respawn the agent, `initialize`, then `session/resume` (or `load`) with the full `cwd` and `mcpServers` resent. That needs the agent to have advertised `resume` or `loadSession`, and to have kept the session on disk. Whether it does is up to each agent, so the harness research has to answer it. [unclear] ([session-setup](https://agentclientprotocol.com/protocol/v1/session-setup.md))
- A turn running when Hypercube went down is lost to the client. v1 has no way to reattach to it, and `session/status` is only a draft and still requires a `resume`.
- Per-session state the agent holds in memory, such as approved `allow_always` decisions, may not survive. The agent owns them. [spec](https://agentclientprotocol.com/protocol/v1/tool-calls.md)

**The UI is ours.** We replace the TUI with a renderer for ACP updates.
- We need to render streamed markdown, thoughts, tool calls with kinds and statuses, diffs, plans, usage, permission and elicitation prompts, config selectors and a slash-command picker, from the content-block model.
- Hypercube should persist the update stream itself. `session/load` replays history only if the agent supports it and at whatever fidelity it chooses, and `resume` replays nothing.
- Tool output is only live if the tool emits `content`, or if the client runs the terminal (we don't). Otherwise the user sees what the agent reports in `rawOutput` and content.
- Anything the TUI did beyond the protocol (keybindings, its own dialogs, `/` commands the agent doesn't list) is gone unless the agent exposes it as a config option or command.

**The user typing directly to an agent.**
- It is a `session/prompt` like any other. Interrupt is `session/cancel`, then a new prompt.
- v1 doesn't define what happens if the prompt arrives mid-turn, so Hypercube has to serialize itself: queue until the previous prompt's response, or cancel first.
- Raw terminal access (shell escapes, editing the agent's input line) doesn't exist. The PTY plan gave that for free.

**Agent-to-agent messages.**
- Hypercube's MCP server (`send_message`) receives them. Delivering one to the recipient is a `session/prompt` from Hypercube.
- Prompts carry no sender field. The text has to say who it's from, or `_meta` has to carry it, and `_meta` is not necessarily shown to the model.
- It lands in the same turn queue as the user's own prompts, so queueing and ordering are on us, and v1 gives no steering, so an agent mid-turn can't be nudged. [unclear] for v1 ([prompt-turn](https://agentclientprotocol.com/protocol/v1/prompt-turn.md))

**Role and profile setup.**
- There is no standard system prompt, so the lead's "you coordinate workers" instructions go in the first prompt or in a harness-specific file such as `CLAUDE.md` or `AGENTS.md`, or wait for the client-system-prompt RFD.
- Model and effort go through `configOptions` with per-harness value mapping (section 6). There is no standard effort.
- Permission posture per profile comes from the mode or config option the agent offers, plus Hypercube's own auto-answer policy for `request_permission`.

**Credentials.** ACP defines `authenticate` and terminal login but not credential storage. The Docker design (keys injected as environment variables) is outside ACP, and each harness's ACP agent has to be checked for whether it starts without an auth step.

**Status.** In v1 Hypercube derives busy, idle and waiting from its own request bookkeeping (section 7). Work an agent does outside a prompt, such as background tasks or its own subagents, is invisible until v2's `state_update` or the subagents RFD lands.

**Version risk.** Build on v1 and treat v2 as a later migration. v2 changes the turn model and drops `session/load`, so the prompt-queueing and status code is the part most likely to be rewritten. Gate anything on `session/fork`, `session/status`, subagents, MCP-over-ACP and system prompts as unstable.
