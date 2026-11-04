# Harness differences

Profiles select a native coding tool through the `harness` field. Install and
authenticate that tool before launching a session. Hypercube configures each
launch without rewriting your global or project settings or changing `HOME`.

## Requirements

| Harness | Executable | Model and effort |
| --- | --- | --- |
| Claude Code (`claude`) | `claude`; works with a Claude Team login | Claude's model and effort values. |
| OpenCode (`opencode`) | OpenCode 2.x | A `provider/model` from `opencode models`; a level offered for that model, or `default` if none. |
| Codex (`codex`) | `codex` | Codex's model and reasoning effort values. |
| Pi (`pi`) | Pi 1.4 or later | A full `provider/id` from `pi --list-models`; effort is its thinking level. |

OpenCode rejects an unsupported model or effort at launch. Use full Pi model
IDs: Pi may substitute a different model for a partial ID, and Hypercube fails
the launch if the selected model differs from the requested one.

## Messages and model changes

A follow-up during work does not have identical timing across harnesses.
Neither does changing the profile selection:

| Harness | Follow-up during a turn | Model or effort change |
| --- | --- | --- |
| Claude Code | Read before its next step. | May affect its next model call. |
| OpenCode | Queued for the next turn, with other waiting messages. | Native controls applied when selected. |
| Codex | Steers the current turn before its next step. | Used at the next turn start; steering keeps the current settings. |
| Pi | Read after the current step's tool calls, before the next model request. | Native controls applied when selected. |

Pi can adjust a requested thinking level to one the model supports. The profile
selector shows the result. A partially failed change can leave the model
changed and the effort unchanged; the error remains visible.

Interrupt ends the current turn and leaves its output in chat. OpenCode, Codex,
and Pi also give up pending questions or dialogs when interrupted.

## Permissions

**Claude Code** runs in auto permission mode. Tool requests that reach Hypercube
offer Allow and Deny. Typing a response without choosing Allow denies the tool
and gives Claude the text. Saved permission choices use Claude's tool names.

**OpenCode** allows permissions it asks about once, as with `opencode --auto`.
A permission denied by OpenCode's own configuration stays denied.

**Codex** runs in a workspace-write sandbox with network access and an automatic
approval reviewer. Git directories are writable, allowing child sessions to
commit. Hypercube's MCP tools need no approval. Requests the reviewer does not
settle appear in chat, with Allow, Deny, and session permission where supported.
If Codex offers cancellation without decline, choosing Deny cancels the turn.

Codex can save command and file-change approvals. Network-host, write-root, and
permission requests have no persistent Hypercube grant. MCP tool approvals have
no Always allow option because Codex does not identify the tool in that request.

**Pi** extension dialogs appear as choices, Yes/No, or a text field according
to the dialog. A timed-out dialog closes; extension errors appear in chat.

Claude Code and OpenCode send answers to a group of questions once all are
answered. Codex does the same for blocking questions; its async questions send
each answer as a message, and new input closes unanswered async questions.

## Usage reports

Context use is the latest request's token count. Other reports vary:

| Harness | Cost | Subscription usage |
| --- | --- | --- |
| Claude Code | Reported with the turn result. | Five-hour and seven-day windows, after turns. |
| OpenCode | Reported for the main conversation. | Not reported. |
| Codex | Not reported. | Account limit windows, after turns. |
| Pi | Session total, including before a resume. | Not reported. |

## Conversation storage

Sessions resume their saved native conversation. A failed resume stays failed
rather than silently opening a new conversation. Pi starts saving with the
first message, so a Pi session with no saved conversation cannot resume.

Hypercube keeps its Codex conversations under its own state directory, outside
the normal Codex Recents list. It links known configuration, credentials,
instructions, skills, and plugins from the inherited `CODEX_HOME` or `~/.codex`.
An older conversation's first resume copies its native rollout into that
storage while keeping its conversation ID.

## Plugin protocol

Each `hypercube-harness-<name>` executable runs beside `hypercube`. The app
starts it with `serve` and exchanges commands and events as JSON Lines. The
plugin owns the native protocol; the core and browser interface do not name
individual harnesses.

The [protocol types](../internal/harness/protocol.go) define commands, replies,
transcript items, and usage fields. Native mappings live in the
[Claude Code](../plugins/claude), [OpenCode](../plugins/opencode),
[Codex](../plugins/codex), and [Pi](../plugins/pi) plugins. Restart a session to
load a newly installed plugin.
