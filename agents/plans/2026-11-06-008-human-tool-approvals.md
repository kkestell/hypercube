# Human tool approvals across harnesses

Status: proposed plan and research findings. No implementation is authorized by
this document alone. Research performed on 2026-11-06; live agent approval
probes remain outstanding.

## Goal and user requirements

Configure each harness to perform its ordinary permitted operations automatically
and send operations needing a human decision to Hypercube. Stop forcing Claude's
auto classifier and Codex's automatic approval reviewer. A legitimate action
should reach the user instead of being rejected by a classifier the user cannot
switch off in Hypercube.

The user trusts these local agents, watches their work, and accepts differences
between harnesses. Reading workspace files should not require approval. Routine
workspace edits should normally proceed. The goal is a simple common interface
for the permission requests a harness emits, rather than interception of every
tool call or identical enforcement across harnesses.

Keep harness-specific policy and native response formats in the executable
plugins. The core must not name a harness. Use only per-launch arguments,
environment, and configuration overrides. Do not write permission rules into
the user's global settings, repository settings, credentials, or HOME.

This work does not include a Hypercube classifier, shell parser, static command
safety engine, new sandbox, deny-rule database, or MCP proxy. Figma and the
reported connector-policy problem are not acceptance criteria.

## Proposed baseline

| Harness | Launch/policy change | Automatic work | Requests reaching Hypercube |
| --- | --- | --- | --- |
| Claude Code | `acceptEdits`, existing stdio permission callback, explicit allow for Hypercube MCP tools | Routine reads, in-scope edits, and commands Claude recognizes as permitted | Remaining shell/tool requests and exceptions to ordinary file permissions |
| Codex | Existing `on-request` and workspace-write sandbox; reviewer `user` | Reads, edits, and commands permitted by the configured sandbox and rules | Escalations and other native approval requests |
| OpenCode v2 | Explicit shell `ask` rule; replace plugin auto-approval with pending requests | Reads and edits under existing rules | Shell calls, plus whatever other rules require approval |
| Pi | Existing extension adds a Bash permission handler | Reads and edits | Bash calls gated by the extension |

These are intentionally different filters. For example, a Codex shell command
can run inside its sandbox while OpenCode or Pi asks about an equivalent shell
call. The UI must not claim that all three enforce the same command policy.

## Evidence and its limits

The research worker inspected repository code, primary documentation, installed
packages, and OpenCode source at its installed tag. It reported these versions:

- Claude Code 2.1.319.
- Codex 0.166.0.
- OpenCode v2.2.0, rather than the older 1.x configuration model.
- Pi 1.4.0.

The worker checked OpenCode's injected configuration with
`opencode debug agents --standalone`, using isolated temporary configuration.
It reported that the proposed rules appeared in the effective agent policy and
that temporary files were deleted. It did not run live agent approval probes.
The parent independently inspected the current launch, permission, grant, and
dialog code. Treat documented behavior and source findings as implementation
evidence, not proof of a completed end-to-end integration.

### Claude Code findings

`plugins/claude/main.go` currently launches with `--permission-mode auto` and
`--permission-prompt-tool stdio`. Requests that the classifier rejects need not
reach Hypercube. Switching to `acceptEdits` removes that classifier from the
ordinary permission path while retaining native permission checks.

The proposed launch arguments are:

```text
--permission-mode acceptEdits
--permission-prompt-tool stdio
--allowedTools mcp__hypercube__*
```

The allow pattern is passed as a literal argument, not expanded by a shell.
Hypercube's own tools should keep working without a permission dialog for every
coordination call. Existing higher-priority native rules still apply.

Documentation describes `acceptEdits` as allowing reads, in-scope file changes,
and recognized filesystem commands. It also permits recognized read-only shell
commands. Other shell calls, external paths, protected-file changes, web tools,
and MCP tools can require approval. Exact command exemptions belong to Claude;
do not reproduce its list in Hypercube or promise that every read always runs.

The current plugin returns a native `behavior: deny` response with a message.
The worker found that this produces a tool error and allows the model to
continue; setting `interrupt` would have different behavior. Preserve denial
feedback. The plugin does not currently request interruption on ordinary denial.

Native permission suggestions can target `.claude/settings.local.json`.
Returning those suggestions unchanged could write project settings, contrary to
the repository's requirements. Continue answering requests without persisting
native suggested rules. A possible session-only suggestion rewrite was not
verified and is not needed for this plan.

The current approval text may contain only a tool name and the model's
description. Include the actual command or target path when constructing the
approval item so the user can assess the operation without finding another row.
Use the real input already received by the plugin; do not infer a command from
descriptive prose.

### Codex findings

`plugins/codex/main.go` already starts or resumes threads with `on-request`, a
workspace-write sandbox, network enabled, and Git directories added as writable
roots. It overrides the reviewer to `auto_review`.

Change only the reviewer in the launch/resume parameters:

```json
{
  "approvalPolicy": "on-request",
  "approvalsReviewer": "user",
  "sandbox": "workspace-write"
}
```

Keep the existing sandbox configuration, network setting, Git handling, and
explicit approval of Hypercube's own MCP tools. This is a reviewer change, not
a reason to redesign sandbox policy. Operations inside the configured boundary
continue automatically. Network use does not automatically prompt because this
plugin already enables network access.

Native decisions include `accept`, `acceptForSession`, `decline`, and `cancel`,
subject to the request's offered choices. The current plugin maps these to
Allow, Allow for this session, and Deny. A decline lets the agent continue; a
cancel interrupts the turn. Preserve that distinction and the current fallback
when the request does not offer decline. Do not claim Deny always continues.

The precise cache key and lifetime of `acceptForSession` were not established.
It must not be described as equivalent to Hypercube's saved tool-wide grant.
Do not add native execution-policy or network-policy amendments that persist
rules outside Hypercube.

The installed schema reported legacy values that differ from current public
documentation. Avoid basing the design on old `untrusted` or `on-failure`
approval policies. This plan uses `on-request`, already used by the plugin.

### OpenCode findings

`plugins/opencode/events.go` currently answers `session/request_permission`
immediately through `permission()` in `requests.go`, selecting `allow_once`.
Stopping that automatic response is necessary but insufficient: v2's base
policy allows most operations and therefore may emit no request.

Merge this rule into the existing `OPENCODE_CONFIG_CONTENT` object alongside
the injected instructions:

```json
{
  "permissions": [
    {"action": "shell", "resource": "*", "effect": "ask"}
  ]
}
```

V2 uses `permissions`, `shell`, and ordered rules. Do not substitute the v1
`permission`/`bash` configuration. The last matching rule wins. The worker
confirmed the injected shell rule against the installed binary; interactions
with agent-specific overrides still need a live check.

A separate `external_directory` ask rule is available. It could cause frequent
requests in worktrees and shared Git directories. Leave that addition out of
the initial baseline; decide it after measuring normal worktree behavior.
Likewise, do not silently expand this shell policy to every web or MCP tool.

ACP offers allow-once, allow-always, and reject-once option identifiers. Retain
the offered native identifiers in the plugin's pending request and respond with
the selected identifier. Do not invent stable option IDs or translate by label.

The worker traced rejection through OpenCode v2.2.0 source: rejection without a
message ends the turn and rejects the session's other pending permission
requests. ACP does not provide the feedback channel needed for the native
continue-with-correction path. Mark permission items as not accepting free
text. Verify the observed stop reason and closing of other requests live.

Native `always` saves the proposed patterns project-wide in OpenCode's database.
It is neither session-only nor managed by Hypercube's saved-permissions screen.
The baseline should select only allow-once or reject-once.

### Pi findings

`plugins/pi/pi.js` currently only registers Hypercube's MCP server. Pi has no
native permission mode or sandbox to switch on.

The installed extension API exposes `tool_call`; an asynchronous handler can
ask with `ctx.ui.confirm` and return `{block: true, reason}` on refusal. The
existing RPC adapter in `plugins/pi/requests.go` already turns confirm dialogs
into attention items and sends `confirmed` responses.

Add a handler for the Bash tool only. Show the actual command, await the
confirmation, and block unless the answer is affirmative. Leave reads and
workspace edits automatic. A refusal returns a tool error and the model can
continue. Do not create a second request mechanism or intercept Hypercube's
own MCP coordination calls.

Use the existing interrupt/dialog cleanup path. The default timeout and precise
cancellation result need a live probe; do not add a fixed timeout to guess at
rendering or user response time. Other installed Pi extensions may affect tool
execution, so this small gate must not be represented as universal enforcement.

## Shared design and saved permissions

The current architecture already contains the needed pieces:

- Plugins emit `KindAttention` items and retain the native pending request.
- Session answer commands carry selected choices and optional text.
- Transcript and notification cards share `AttentionForm` and its drafts.
- The session runtime stores and applies Hypercube tool-wide grants.

Reuse those paths. A once/deny approval does not need a new protocol item type,
permission service, policy interface, or harness-specific branch in the core.
Questions remain questions and keep their offered choices and multi-select
behavior. Pi's existing confirm can initially retain its Yes/No presentation.

For OpenCode, extend its pending request handling to represent either a form or
a permission request, with only the fields those cases need. Permission items
offer Allow once and Deny; the plugin maps them to native option IDs. Ensure
answer validation, cancellation, withdrawal, and shutdown cover both cases.
Replace the unconditional `permission()` response rather than leaving parallel
automatic and interactive permission paths.

Saved permissions are an independent product decision, not a prerequisite for
removing classifiers. Today `Grant` identifies a whole tool/category, and
`Runtime.AlwaysAllow` records the first choice for matching requests. Grants are
scoped to one Hypercube session or all sessions of the same harness, survive
restart, and can also answer currently open matching requests.

Examples of their breadth:

- Claude `Bash`: all Bash approval requests, not one command prefix.
- Claude `Edit`/`Write`: exceptions that normally require approval too, including
  some protected or external targets.
- Codex `commandExecution`: subsequent matching command approval requests,
  including requests to leave the normal sandbox boundary.

A Codex command grant does not literally disable the sandbox or authorize every
possible operation. It automatically accepts requests tagged with that category.
The earlier worker wording that it "removes the sandbox" was too broad.

For the first implementation, preserve existing grant records, keys, application,
settings removal, and existing grant controls. Do not delete or reinterpret
saved state, remove Codex's native session choice, or add new OpenCode/Pi
tool-wide grants as an incidental part of the launch changes. An existing saved
grant can suppress a prompt during validation; use isolated state without grants.

The original UI problem remains a planned follow-up. The user requested one
radio group and then asked for the preliminary UI changes to be reverted while
the permission model was researched. Before implementing the follow-up, settle:

1. Whether the common choices should be Allow once, Deny, Allow this tool in
   this session, and Allow this tool in all sessions, omitting native remembered
   choices from new prompts while retaining decoding for old pending requests.
2. Whether broad Hypercube grants remain available for all currently supported
   tool categories, and whether new OpenCode/Pi approvals should offer them.
3. Whether exact command/pattern grants are needed. They are outside this plan;
   do not implement shell parsing or native persistence to simulate them.

If the proposed common choices are adopted, render all as radio options with one
Submit action, keep the real scope visible, use existing shared drafts, and
dispatch either `answerRequest` or `alwaysAllow` only on submission. Do not
replace real request handling with a permanent preview path. Reuse the scripted
harness for deterministic visual review.

## Implementation sequence and touched files

1. **Confirm the native boundaries with isolated live probes.** Use the
   installed versions and launch overrides above, clean state, and harmless
   reads/edits/commands in a temporary repository. Establish a representative
   request/allow/deny cycle for each harness. Record any deviations in this plan
   before changing the implementation around an assumption.
2. **Change Claude's launch and approval content.** Update
   `plugins/claude/main.go` and `requests.go`. Keep the existing stdio callback,
   denial feedback, and native response ownership. Update relevant boundary
   assertions in `plugins/claude/main_test.go`.
3. **Change Codex's reviewer.** Update `plugins/codex/main.go`, correct the
   automatic-reviewer comment in `requests.go`, and update launch/resume
   assertions in `main_test.go`. Preserve native request mapping and saved grants.
4. **Implement OpenCode's interactive permission path.** Update
   `plugins/opencode/main.go`, `events.go`, and `requests.go`, splitting a file
   only if forms and approvals become difficult to follow. Keep request ownership
   in the existing agent. Replace auto-approval, inject the shell rule, extract
   enough native tool-call information to show the actual command, and add
   focused protocol tests in `main_test.go` or a behavior-named test file.
5. **Gate Pi Bash through the existing extension/dialog path.** Update
   `plugins/pi/pi.js`; change Go dialog handling only where a verified behavior
   requires it. Add a suitable behavioral check for the extension-to-dialog
   boundary; do not merely assert JavaScript source text.
6. **Update accurate documentation.** Revise Permissions in
   `docs/harness-support.md`, the Pi extension comment, and any developer
   reference made inaccurate. Update `docs/usage.md` only for behavior users
   need to understand, especially OpenCode rejection ending a turn. Do not change
   user configuration examples or profiles: this baseline is plugin launch policy.
7. **Review the integrated implementation.** One independent review is required
   for this cross-plugin change. Review whole touched files, state ownership,
   actual native replies, and the absence of redundant paths. Resolve verified
   findings and complete validation before calling the work done.
8. **Handle the UI/grant follow-up separately once its choices are settled.**
   Likely files are `plugins/codex/requests.go`,
   `internal/ui/web/transcript/attention-form.tsx`, its existing tests,
   `agent/drafts.ts` if necessary, and `docs/usage.md`. Do not expand this step
   into a new persistent permission engine.

No database migration is expected for the baseline: it preserves stored grants,
transcript items, and command formats. If implementation changes saved semantics
or fields, revisit that conclusion explicitly and include migration/replay work.
Already saved or open native requests must remain answerable after restart.

## Validation and acceptance

Choose checks by observable consequence. Existing harness stand-ins are useful
for request/response transport, while live probes establish native mode behavior
that a stand-in cannot prove. Avoid duplicating the same transport assertion at
every layer.

### Focused protocol and lifecycle checks

- Claude launch includes `acceptEdits`, stdio callback, and the Hypercube allow
  pattern; an approval stays open until answered; Allow and Deny produce the
  right native response and denial feedback survives.
- Codex start and resume use reviewer `user`; request decisions still map to
  accept/decline/cancel correctly. No new persistent rule amendment is sent.
- OpenCode does not answer a permission before the user acts. Allow and Deny
  use the exact offered option IDs. Permission cancellation, withdrawal,
  interruption, and shutdown close requests without leaking a waiter or sending
  duplicate native responses. Native form questions retain their behavior.
- Pi Bash waits for the dialog result, affirmative answers proceed, and refusal
  or interruption cannot accidentally execute the gated command. Reads and edits
  do not acquire new dialogs.
- Existing saved grants still work and can be removed in Settings. No test
  silently starts with a grant that bypasses the path it intends to exercise.

### Live smoke matrix

Run in isolated state with harmless temporary files and no saved grants:

| Case | Expected observation |
| --- | --- |
| Workspace read | No new approval dialog on any harness |
| Ordinary workspace edit | No approval on Claude acceptEdits, Codex, OpenCode, or the proposed Pi gate |
| Representative approval-requiring shell operation | Claude/OpenCode/Pi request appears; Codex requests only when its sandbox/rules require it |
| Allow once | One native operation proceeds; later approval requirements are not silently persisted |
| Deny | Native denial occurs; continuation or interruption agrees with the documented harness behavior |
| Hypercube MCP coordination call | No routine approval loop introduced |
| Interrupt with a request open | Request closes and the operation does not proceed because of cleanup |
| Restart session | New launch policy applies; existing saved state and transcript remain usable |

For Codex, explicitly request a harmless escalation rather than expecting an
ordinary in-sandbox command to prompt. For Claude, use an operation outside its
known automatic exemptions. Do not force a destructive operation just to create
a prompt. For OpenCode, verify the effect on another pending request and observe
the turn's final stop reason. For Pi, observe cancellation while confirm waits.

Use focused `go test -race` runs for changed plugin behavior during development,
then `make check`. Use `make dev` for the end-to-end smoke test, reusing an
existing dev server. Clean up any extra processes or isolated probe state started
for this task; never stop the user's workspaces. Browser UI changes, if adopted,
also need TypeScript tests and the build, with visual judgment left to the user.

The baseline is complete when routine operations remain automatic, native
approval requests are visible and answerable, neither forced classifier is in
the approval path, and no plugin silently approves a request it is meant to
present. The UI/grant follow-up has separate acceptance criteria and must not be
reported as completed by the launch changes alone.

## Rollout and remaining questions

Harness configuration and extension code are fixed for the launched process.
Installing the service alone will not change active agents. Warn the user before
implementation and again when offering installation: restart affected sessions
to use the new behavior. Do not restart user sessions automatically. Commit or
install only when asked; this request is to write the plan.

Outstanding verification is deliberately bounded:

- Claude stdio permission delivery and denial continuation under `acceptEdits`.
- Codex's native session-approval scope, if we later retain or explain that choice.
- OpenCode rule precedence for selected agent profiles, rejection stop reason,
  and worktree noise before adding an external-directory rule.
- Pi confirm cancellation/default timeout and interaction with installed extensions.
- Saved-grant choices and the common radio-group presentation, before that
  follow-up is implemented.

Do not turn these unknowns into defensive layers. Probe the relevant behavior,
then implement the smallest supported path or document the actual limitation.

## Primary references

- [Claude Code permission modes](https://code.claude.com/docs/en/permission-modes)
  — modes, automatic operations, classifier behavior, and noninteractive runs.
- [Claude Code permission rules](https://code.claude.com/docs/en/permissions)
  — allow/ask/deny rules and precedence.
- [Claude Agent SDK permissions](https://code.claude.com/docs/en/agent-sdk/permissions)
  — permission callback responses and suggestions.
- [Claude Agent SDK user input](https://code.claude.com/docs/en/agent-sdk/user-input)
  — host interaction and user decisions.
- [Codex approvals and security](https://learn.chatgpt.com/docs/agent-approvals-security)
  — sandbox boundaries, approval policy, and automatic review.
- [Codex configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference)
  — reviewer and MCP approval settings.
- Installed Codex 0.166.0 app-server schema, generated using
  `codex app-server generate-json-schema` — native start/resume parameters and
  approval response choices. Record the version when regenerating it.
- [OpenCode permissions](https://opencode.ai/docs/permissions/) and
  [configuration schema](https://opencode.ai/config.json) — v2 policy and replies.
- [OpenCode v2.2.0 source](https://github.com/anomalyco/opencode/tree/v2.2.0)
  — `cli/src/acp/permission.ts`, `core/src/permission.ts`, and
  `core/src/session/runner/step.ts`, as inspected by the worker for ACP replies,
  persistence, and rejection behavior.
- [Pi extension documentation](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md)
  — `tool_call`, blocking, and extension UI. The worker also inspected installed
  Pi 1.4.0 `docs/rpc-extension-ui.md` and
  `examples/extensions/permission-gate.ts`; check installed-version behavior
  rather than assuming the moving main branch is identical.
