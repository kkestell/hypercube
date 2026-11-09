# Harness differences

A harness is the native coding tool behind a session. Install and authenticate
it before starting a session. Hypercube uses per-launch configuration: it does
not rewrite your global or project settings or change `HOME`.

## Requirements

| Harness | Required executable | Model and effort selection |
| --- | --- | --- |
| Claude Code | `claude`; works with a Claude Team login | A model and effort listed by its initialization. |
| OpenCode | `opencode`, version 2.x | A `provider/model` and one of its offered variants. |
| Pi | `pi`, version 1.4 or later | A full `provider/id` and an offered thinking level. |

Hypercube discovers installed harnesses at startup. The selector lists models
configured in **Settings › Agents › Models**. The table above gives the form
of each harness's model ID and effort. Invalid selections are rejected at start
or model change, before the first prompt.

A model with efforts needs one; a model without efforts takes none. OpenCode and
Pi report the chosen effort back to the selector. Listing models does not
create a conversation; OpenCode reads its native model catalog directly.

## Messages and model changes

| Harness | Follow-up during work | Model or effort change |
| --- | --- | --- |
| Claude Code | Read before its next step. | May affect its next model call. |
| OpenCode | Read at the next step boundary, after running tools finish. | Native controls applied when selected. |
| Pi | Read after the current step's tool calls, before the next model request. | Native controls applied when selected. |

A partially failed change can leave the model changed but the effort unchanged.
The selector shows accepted settings; the error stays visible in chat.

Interrupting leaves output in the conversation. OpenCode and Pi also give up
pending questions or dialogs when interrupted.

## Permissions

Agents are local processes using your credentials. Separate worktrees do not
restrict filesystem access. Native permission and sandbox behavior still
depends on the harness:

### Claude Code

- Runs in auto permission mode.
- Tool requests that reach Hypercube offer **Allow** and **Deny**.
- Typing a response without choosing Allow denies the tool and supplies the
  text to Claude.
- Saved permission choices use Claude's tool names.

### OpenCode

- Allows permissions it asks about once, as with `opencode --auto`.
- A permission denied by OpenCode's own configuration stays denied.

### Pi

- Extension dialogs appear as choices, Yes/No, or a text field.
- A timed-out dialog closes; extension errors appear in chat.

Saved choices can be reviewed and removed in **Settings**.

## Questions

| Harness | Answer delivery |
| --- | --- |
| Claude Code | A group of questions is submitted after all are answered. |
| OpenCode | A group is submitted after all are answered. Withdrawn questions close. |
| Pi | Answers follow the extension dialog's controls. |

Hypercube handles its own **New session** / **Do it here** questions directly,
without the harness.

## Usage reports

Context use is an estimate from the latest model request or step. The tokens
counted depend on the harness:

| Harness | Context use | Cost | Subscription usage |
| --- | --- | --- | --- |
| Claude Code | Latest input, including cached tokens. | Reported with the turn result. | Five-hour and seven-day windows, after turns. |
| OpenCode | Latest step's input, output, and cached tokens. | Main conversation. | Not reported. |
| Pi | Latest request's total tokens, including output. | Session total, including before a resume. | Not reported. |

See [usage indicators](interface.md#usage-indicators) for how readings appear.

## Conversation storage

Each session resumes its saved native conversation. A failed resume stays
failed instead of silently creating a new conversation. Pi starts saving
with the first message; a Pi session with no saved conversation cannot resume.

Forking also uses native history and fails explicitly if it cannot copy that
history. See [fork context and directory rules](introduction.md#fresh-and-forked-context)
and [storage locations and native conversations](storage.md#native-conversations).

## Native subagent display

Harness-created subagents are not Hypercube child sessions. OpenCode shows
the parent tool's result, not live subagent transcripts, because its protocol
does not identify the launching tool. Other harnesses retain child transcripts
where their protocols supply that ID.
