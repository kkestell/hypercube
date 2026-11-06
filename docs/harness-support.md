---
title: Harness differences
weight: 5
---

# Harness differences

Each session runs a native coding tool, its harness. Install and authenticate
that tool before launching a session. Hypercube finds the installed harnesses
when it starts and when the selector opens, and asks a harness for its models
and efforts in the session's folder. Hypercube configures each launch without
rewriting your global or project settings or changing `HOME`.

## Requirements

| Harness | Executable | Model and effort |
| --- | --- | --- |
| Claude Code (`claude`) | `claude`; works with a Claude Team login | A model and effort its initialization lists. |
| OpenCode (`opencode`) | OpenCode 2.x | A `provider/model` and a variant offered for that model. |
| Codex (`codex`) | `codex` | A model and reasoning effort from its model list. |
| Pi (`pi`) | Pi 1.4 or later | A full `provider/id`; effort is a thinking level the model offers. |

Each harness rejects a model or effort it does not offer when the session
starts or changes model, before the first prompt. Without an effort, the
harness uses its default; Codex, OpenCode, and Pi report the effort they chose,
and the selector shows it. Listing models starts no conversation: OpenCode
opens a temporary session without tools and deletes it.

## Messages and model changes

A follow-up during work does not have identical timing across harnesses.
Neither does changing the model or effort:

| Harness | Follow-up during a turn | Model or effort change |
| --- | --- | --- |
| Claude Code | Read before its next step. | May affect its next model call. |
| OpenCode | Queued for the next turn, with other waiting messages. | Native controls applied when selected. |
| Codex | Steers the current turn before its next step. | Used at the next turn start; steering keeps the current settings. |
| Pi | Read after the current step's tool calls, before the next model request. | Native controls applied when selected. |

A partially failed change can leave the model changed and the effort
unchanged; the selector shows the accepted settings and the error remains
visible.

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
each answer as a message, and new input closes unanswered async questions. A
Codex async question leaves the agent working, so it holds up a parent's
automatic reports until the turn ends; every other question means the agent
waits for you. OpenCode closes a question it withdraws.

Hypercube answers its own **New session** / **Do it here** questions without
the harness.

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

Forked children copy a saved native conversation using the same harness.
A fork of a session in the original checkout shares its folder; a fork of a
session in its own worktree gets a new worktree. Claude Code, Codex, and Pi can
fork into another working directory. OpenCode can fork only sessions in the
original checkout, and requires a version with native fork support.
A failed fork reports an error instead of starting with fresh
context. See
[sessions and working copies](introduction.md#what-the-other-agent-knows).

Codex uses its native conversation storage, configuration, and credentials:
`CODEX_HOME` from Hypercube's launch environment, or Codex's default home.
Conversations appear in Codex's normal history. Hypercube does not select an
account itself.

Earlier versions stored Codex conversations in `codex/` under Hypercube's state
directory, including `codex/homes/` for additional accounts. Those files and
Hypercube's transcripts are preserved. Codex has no supported cross-home import,
so existing private conversations cannot resume through this integration.
Their saved chats remain readable; start a new session to use the native
integration.

OpenCode shows the parent tool's result for native subagents. Their live child
transcripts are not displayed because OpenCode does not identify the launching
tool.
Other harnesses retain child transcripts where their protocols supply that ID.
