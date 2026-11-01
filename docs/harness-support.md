# Harness support

Each harness is supported by a plugin: an executable named
`hypercube-harness-<name>` installed beside `hypercube`. The `harness` value in
the [configuration](configuration.md) is that name. `make install` installs
the plugins for Claude Code (`claude`), OpenCode (`opencode`), Codex
(`codex`), and Pi (`pi`). The plugins configure each launch without rewriting
user or project settings or changing HOME, and the harnesses keep their own
conversations, credentials, and delegation. Neither Hypercube's core nor the
browser view names a harness; native protocols live in the plugins.

## Plugin protocol

Hypercube runs `<plugin> serve` for each agent and keeps it running. The two
exchange JSON Lines on the plugin's standard input and output, and the plugin
writes diagnostics to standard error. The plugin ends when its input closes.
The types are in [protocol.go](../internal/harness/protocol.go).

- **Commands.** Each carries an ID, and the plugin replies with the same ID: a
  success or an error. `start` gives the plugin the agent's launch information:
  the hypercube executable path, an environment for Hypercube processes the
  harness starts, the agent's ID, role, model, effort, saved conversation,
  working directory, runtime instructions, and the state directory. The
  plugin replies once the harness is ready for input, and the harness loads
  `hypercube mcp` as an MCP server with that environment. `input` carries text
  for the harness; its reply means the harness accepted the text, including into
  its own queue, and an error means it did not. `interrupt` stops the current
  turn. `answer` replies to an attention request. `shutdown` ends the plugin.
- **Events.** The plugin reports the native conversation ID to resume later,
  the main agent's work state (`working` or `idle`), transcript items, and
  usage: `context`, the tokens of the latest request, `window`, the model's
  context window, and `cost`, in US dollars.
- **Items.** An item has an ID and a kind: text, reasoning, tool, error, or
  attention. A later event with the same item ID replaces the item, or extends
  its text when it sets `append`. A tool item carries the tool's name, input,
  output, and whether it failed. A native subagent's items name the item that
  started it as their parent.
- **Attention requests.** An attention item is a prompt with optional choices
  and optional free text. The browser answers with the item's ID, and the plugin
  maps the answer to the native reply. An answer to a finished request is an
  error.

A malformed record, or a reply to an unknown ID, ends the connection. The agent
then follows the relaunch rule in [Architecture](architecture.md#ownership).

The plugins are in [plugins](../plugins).

## Messages and native subagents

Hypercube sends each queued message as an `input` command, one at a time, and
removes it when the plugin replies with success. Messages go out whether the
agent is idle or working; the plugin steers or queues them with the native
harness. A failed worker turn preserves its queued messages and sends its
session's lead a bounded error summary. A lead's own failures stay in its
chat.

Native subagents are separate from the Hypercube agent's main conversation.
Their items appear nested in the main agent's transcript, and they never take
the parent's messages or change its saved conversation or work state.

## Claude Code

The plugin keeps one `claude -p` process running per agent, with stream-JSON
input and output, and works on a Claude Team login. The process runs in auto
permission mode, with the model and effort from the agent's profile and the
`hypercube mcp` server in its MCP configuration. The runtime instructions are
appended to the system prompt on every launch, and a saved conversation resumes
with `--resume`. A conversation Claude Code cannot resume fails the launch.

- **Text.** Assistant text and reasoning stream into the transcript as Claude
  writes them. Native subagents' tool calls appear nested under the call that
  started them.
- **Permissions and questions.** A tool permission request becomes an attention
  request with the choices Allow and Deny; typed text without a choice denies the
  tool and tells Claude the text. An `AskUserQuestion` call becomes one attention
  request per question, with the options as choices and free text for another
  answer; Claude receives the answers once every question has one.
- **Messages and interrupts.** Messages sent during a turn go to Claude at once,
  and it reads them before its next step. Interrupt stops the turn the way Esc
  does, and the stopped turn stays in the transcript.
- **State and usage.** Claude reports when it starts working and when it is
  idle. Context is the tokens of the latest request; the context window and cost
  come from each turn's result.

## OpenCode

OpenCode 2.x is required. The plugin keeps one `opencode acp` process running
per agent and speaks the Agent Client Protocol to it. The process runs in the
agent's working directory with the `hypercube mcp` server as an MCP server of
its session. The runtime instructions are loaded as an OpenCode instructions
file on every launch, and a saved conversation resumes with `session/resume`.
A conversation OpenCode cannot resume fails the launch. A permission that
OpenCode asks about is allowed once, as with `opencode --auto`; a permission
that OpenCode's configuration denies stays denied.

The model must be a `provider/model` ID from `opencode models`, and the effort
one of the levels OpenCode offers for that model (`default` for a model without
levels); a launch with any other value fails and names it.

- **Text and tools.** Text and reasoning stream into the transcript as OpenCode
  writes them. A subagent's text and tool calls appear nested under the call
  that started it.
- **Questions.** OpenCode's question tool becomes one attention request per
  question, with the options as choices and free text for another answer;
  OpenCode receives the answers once every question has one.
- **Messages and interrupts.** OpenCode runs one prompt at a time, so a message
  sent during a turn waits in the plugin and starts the next turn when the
  current one ends, together with any other waiting messages. Interrupt cancels
  the turn and any open questions, and the stopped turn stays in the transcript.
- **State and usage.** The agent is working from the start of a turn until its
  last prompt has finished. Context and cost come from OpenCode's usage updates
  for the main conversation, and the context window from the model.

## Codex

The plugin runs the Codex CLI with Approve for me: an automatic reviewer
answers approval requests inside a workspace-write sandbox with network
access. The agent's Git directories are writable, so a worker can commit in
its worktree.

## Pi

Pi 1.4 or later is required. A conversation resumes by its Pi session ID. Pi
has no approvals. The model is `provider/id`, and effort is Pi's thinking
level.
