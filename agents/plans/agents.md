# Agents: phased implementation of `docs/agents.md`

Builds the design in `docs/agents.md` in small phases. Each phase ends with something the user can reload and play with in the dev server. Speed over polish until phase 10: no new tests unless a bug report calls for one, happy paths first, edge cases listed under "Deferred" in each phase and picked up when hardening.

Two things are not deferred, because they are cheap now and expensive to retrofit:

- **Invariant 1 (security).** Every new endpoint that changes state gets the loopback and origin checks from the start (`AGENTS.md`).
- **Process groups.** Agent processes are spawned in their own process group and killed as a group, so a stopped agent doesn't leave its children running. Full ownership rules (reparented children, PID reuse, `setsid`) wait for phase 10.

Each phase updates `docs/architecture.md` for the routes, protocol and model it changes, and runs `bun test` and `bunx tsc --noEmit` before it's called done (existing tests may be rewritten when the model changes under them).

## Cross-cutting decisions

- **ACP client.** `@agentclientprotocol/sdk` (v1 protocol), server side only. Check its current client API at install time; the research says the fluent `client()` API replaced `ClientSideConnection`. If it fights Bun, hand-roll NDJSON JSON-RPC over the pipes; the surface we use is small.
- **Advertised client capabilities.** None (`fs` and `terminal` off), per `agents/research/acp/protocol.md` §5.
- **Transcripts: the server reduces, the browser replaces.** The server folds ACP `session/update`s into a list of transcript items per agent (`user`, `agent`, `thought`, `tool`, `plan`, `permission`, `notice`), keyed so streamed chunks and tool-call updates replace an existing item. It sends the browser item upserts, not raw ACP. The same items are what phase 6 stores.
- **Transcripts don't go in the model push.** The model (workspaces, sessions, agents with status) is still pushed whole. Transcripts get their own messages: a snapshot per agent on connect, then `item` upserts to every socket. Simplest thing that works at the expected scale; per-panel subscription can come later.
- **Environment seam.** A tiny `Environment` interface from phase 1 (`setup`, `spawn(cmd, cwd, env) → pipes + kill`, `run(cmd)`, `teardown`) with only a local implementation, so Docker in phase 9 is a second implementation rather than a refactor.
- **Runtime state on `globalThis`.** Agent processes and ACP connections live next to the model on `globalThis`, so `bun --hot` reloads keep them, as the model does today.

## Phase 1: GitHub workspaces and session clones

The model switches from "a workspace is a local directory" to "a workspace is a GitHub repo, a session is a clone of it".

- `addWorkspace` takes `owner/name` or a GitHub URL, normalizes it to `owner/name`, and checks it with `gh repo view`. Name is `owner/name`. The add-workspace UI asks for a repo instead of a path.
- `createSession` adds the session immediately with `status: "cloning"`, then runs `gh repo clone owner/name ~/.local/share/hypercube/sessions/<session-id>` in the background. It ends `ready` or `failed` with the error.
- `deleteSession` (new request) removes the clone and the session. No unpushed-work check yet (phase 6).
- Session list shows the status (spinner, error) and a delete action.

Deferred: deleting while cloning, clone cancellation, non-GitHub remotes, duplicate-repo checks beyond exact `owner/name`.

**You can test:** add a real repo by name or URL, get an error for a bad one, create sessions and watch them go from cloning to ready, find the clone on disk, delete a session and see its directory go away.

## Phase 2: Talk to the lead

The first real agent: the lead runs `opencode acp` in the clone and the agent panel becomes a chat.

- When a session becomes `ready`, spawn the lead (`opencode acp`, cwd = clone, own process group), `initialize`, `session/new` with `cwd`. Record the ACP session id on the agent.
- Agent gets `status`: `starting`, `idle`, `busy` (prompt in flight), `waiting` (permission request open), `exited` (with exit code or error).
- New requests: `prompt {agentId, text}`, `cancel {agentId}`, `answerPermission {agentId, requestId, optionId}`.
- **Prompt queue.** One `session/prompt` at a time per agent. A prompt sent while busy is queued and sent when the turn ends. The UI shows queued prompts.
- Agent panel: scrolling transcript (user messages, agent text as plain text, thoughts collapsed, one line per tool call with kind, title and status), a composer, a Stop button, permission requests rendered as buttons for their options, a status badge on the tab.
- Deleting a session kills its lead's process group. Server shutdown kills every agent.

Deferred: restarting an exited agent (phase 6), markdown and diffs (phase 3), crash during a turn, cancel races, stderr capture beyond a log line, very long transcripts.

**You can test:** create a session and chat with the lead in its clone. Ask it to read or change files and watch tool calls stream in, stop a turn mid-way, answer a permission prompt, send several prompts while it's busy and see them queue and run in order. Check with `ps` that deleting the session kills `opencode`. Reload the page mid-turn and see the transcript come back and keep streaming.

## Phase 3: A proper agent panel

Make the panel pleasant enough to work in.

- Markdown for agent messages (with code blocks).
- Tool calls expand to show their content: diffs as a side-by-side or unified diff, text output, locations.
- The `plan` update rendered as a checklist pinned near the composer.
- `usage_update` shown in the panel header (context used, cost if given).
- **Config options** from `session/new` and `config_option_update` shown as dropdowns in the header (model, thought level, mode). Changing one calls `session/set_config_option`. This gives model and effort choice before profiles exist.
- Slash commands from `available_commands_update` offered as completions when the composer starts with `/`.
- `session_info_update` titles rename the session if it's still "Untitled N".

Deferred: images and resource content blocks, elicitation, syntax highlighting quality.

**You can test:** readable conversations with formatted code, inspect a diff the agent made, watch its plan tick off, switch the lead's model or effort from the header mid-session, run an OpenCode slash command, see the session pick up a title.

## Phase 4: Hypercube's MCP server and messaging between leads

Agents can talk to Hypercube and to each other.

- `POST /mcp` on the Bun server: MCP over streamable HTTP. Use the MCP TypeScript SDK if it works with `Bun.serve` request handlers; otherwise implement `initialize`, `tools/list` and `tools/call` by hand.
- **Identity.** Each agent gets a random token at spawn. It goes into `session/new`'s `mcpServers` as an `http` server with an `Authorization: Bearer` header. The endpoint maps token to agent and rejects anything else.
- **Security.** `/mcp` requires a loopback `Host`, rejects any request with an `Origin` header (agents don't send one; browsers do), and requires a valid token.
- Tools: `list_agents` (teammates and status, scoped as below) and `send_message(to, text)`.
- Scopes for now: a lead can message other leads in its workspace. (Workers come in phase 5.)
- **Delivery.** A message becomes a prompt in the recipient's queue, prefixed with who sent it (`Message from <name> (<session>):`). The transcript shows it as a message from that agent, not from the user.
- **Role instructions.** ACP has no system prompt, so the lead's first prompt is preceded by a Hypercube preamble: who it is, its session and workspace, its tools and the messaging rules.

Deferred: rate limiting or loop protection between agents, token rotation, message delivery when the recipient has exited.

**You can test:** open two sessions on the same workspace, ask one lead to ask the other what it's working on, and watch the message arrive in the second lead's panel, get answered and come back. Ask a lead to list its peers. `curl` the endpoint without a token, with someone else's token, or with an `Origin` header, and get rejected.

## Phase 5: Profiles and workers

Leads start workers and the session becomes a team.

- **Config file** `~/.config/hypercube/config.json` with `lead` (harness, model, effort) and `profiles` (name, description, harness, model, effort), shaped like the Rust Hypercube's (`~/projects/hypercube/docs/configuration.md`). Re-read on each use, as there. Harness is only `opencode` for now.
- **Applying a profile.** After `session/new`, find the config options with category `model` and `thought_level`, match the profile's values, and `set_config_option`. If a value doesn't match, the agent goes to `exited` with an error naming the profile and the values it offers. The lead uses `lead` from the config.
- MCP tools: `list_profiles`; `start_worker(name, profile, dir, message?)` (lead only; `dir` must resolve inside the session's clone); `stop_worker(id)` (lead only).
- Workers appear as new dockview tabs and work like the lead in every other way: you can prompt them, stop their turn, answer their permissions.
- Scopes: lead and its workers message each other; workers can't message outside their session. The preamble for workers says who their lead is.
- A remove action on worker tabs for the user.

Deferred: worker limit per session, the lead restarting a worker, auto-answering permissions for unattended workers (until then the user answers them in the worker's tab), harnesses other than OpenCode.

**You can test:** give the lead a task that splits, watch it start workers on different profiles in worktrees it makes, see their tabs appear, follow the lead–worker messages, prompt a worker directly, have the lead stop one. Put a bad model in a profile and see the error.

## Phase 6: Persistence, resume and safe deletion

Restarting the server no longer loses anything.

- SQLite at `~/.local/share/hypercube/hypercube.db` (`bun:sqlite`): workspaces, sessions (environment, clone path), agents (role, profile, ACP session id, token-free), transcript items. Write on change; load at startup.
- On startup, for every agent of every ready session: spawn, `initialize`, `session/resume` with the recorded id, `cwd` and a fresh `mcpServers` entry. Fall back to `session/load` and then to a new session with a notice in the transcript. Transcripts come from the database, not from replay.
- Agents that exit unexpectedly show a Restart action, which does the same resume.
- A session-level Stop and Start (stop all its agents, resume them later) so old sessions don't hold processes.
- **Safe delete.** Before deleting, run `git status --porcelain` and check for commits not on any remote branch, in the clone and every worktree in it. If there are any, show them and ask.

Deferred: automatic restart policies, schema migrations (drop and recreate is fine until hardening), transcript compaction, a turn running at shutdown (it's lost, as the design says).

**You can test:** have a conversation, stop and restart `bun dev`, reload, and find every session, tab and transcript back; send the lead a follow-up that shows it remembers. Kill an `opencode` process by hand and restart it from the UI. Stop a session and start it again. Make a commit in a session, don't push, try to delete it and get asked.

## Phase 7: Steering

Redirect an agent while it works, from the user or from another agent, with nothing but ACP v1, and deliver messages between agents while the recipient is working instead of after. This is the only way to rein in a worker that's going wrong without throwing its turn away.

ACP v1 allows one prompt at a time, and OpenCode rejects a second one mid-turn ("Session already has an active ACP prompt", verified). So a steer interrupts the turn at the next tool boundary and becomes the next prompt:

- A steer to an idle agent is an ordinary prompt.
- A steer to a busy agent: if a tool call is running, wait for it to finish (`tool_call_update` with `completed` or `failed`); otherwise (the model is thinking or writing) don't wait. Then `session/cancel`, and when the cancelled prompt returns, send the steer before anything in the queue. An open permission request is answered `cancelled` first.
- The steer prompt says it was sent mid-turn and that the agent's previous turn was interrupted after its last finished step.
- Verified with OpenCode on 2026-10-29: steered right after a 10 s tool call finished, the cancelled prompt returned at once, and the agent's next answer knew which commands had run and which hadn't. What's lost is only the model output after the last tool result. Other harnesses are unverified.
- **Composer:** Enter while the agent is busy steers by default, with a way to queue instead. The panel shows a pending steer as "waiting for <tool> to finish" with an Interrupt now action (Stop, then send) for a tool that runs long or hangs.
- **Agents:** every message between agents is delivered as a steer, not queued behind the turn. Queued messages arrive after the recipient has moved on (a lead saw workers' replies only after it had stopped them), and a lead that waits inside its turn never sees replies at all. Each delivered message says how long ago it was sent.

Deferred: several steers arriving before the turn stops (send them as one prompt), a harness that loses tool results on cancel.

**You can test:** give a worker a long multi-step task, steer it partway through from its tab, and watch it stop after the current command and follow the new instruction while remembering what it already did. Have a lead and its workers exchange messages while they work and see replies arrive mid-turn, after the current command, instead of after the turn. Start a long `sleep` and use Interrupt now.

## Phase 8: Overview and status

Make it usable at the expected scale (six workspaces, a few active sessions each, up to eight agents).

- Agent status rolled up to the session list and the overview: who's busy, who's waiting on you.
- A clear "needs you" signal for open permission requests, across workspaces.
- The side panels get content: e.g. the session's agents and their status, the queue, recent messages between agents.

Deferred: notifications outside the tab.

**You can test:** run a few sessions with workers, switch away, and see from the overview which agents are busy and which are waiting on a permission answer.

## Phase 9: Docker environments

The second environment, chosen when creating a session.

- A base image with OpenCode (and `git`, `gh`) built by Hypercube from a Dockerfile in the repo. A workspace can name its own image built `FROM` it.
- Docker environment: `setup` creates and starts a container for the session and clones inside it; `spawn` is `docker exec -i`; `teardown` removes the container. GitHub token from `gh auth token` and OpenCode's credentials passed as environment variables at container start. Nothing from the home directory is mounted.
- Create-session UI gets a Local/Docker choice.
- **Open question, verify before building:** how an agent in the container reaches `/mcp` when the server binds loopback only, and how the Host/Origin check treats it. Options include a per-session forwarder or a second listener; decide after testing what Docker on this machine actually allows.
- **Open question:** how OpenCode's ChatGPT subscription credentials get into the container as environment variables.

**You can test:** create a Docker session, chat with its lead, have it start workers, check with `docker ps` that everything runs in the container, delete the session and see the container go.

## Phase 10: Harden

Once the behavior is settled:

- Tests for the invariants: `/mcp` security, message scopes, prompt queue ordering, process-group kill on delete and shutdown, resume after restart, safe-delete detection. Bring over the ownership rules and tests from `process-management` where they apply.
- Work through the "Deferred" lists above and anything reported while testing.
- Streaming efficiency (append-only chunks instead of whole-item upserts) if long transcripts are slow.
- Bring `docs/architecture.md` and `AGENTS.md` fully up to date, including new invariants for agents.

## Not in v1

Claude Code as a harness (`docs/agents.md`: its ACP adapter can't use a Claude subscription), non-GitHub workspaces, per-workspace mirrors for large repos.
