# Change a session's model and effort

## Goal

Allow a running session to select a configured profile with the same harness between turns. Save its model and effort for later turns and resumes. A session never changes harness.

## Native controls

- **Claude Code:** The [Agent SDK reference](https://code.claude.com/docs/en/agent-sdk/typescript#applyflagsettings) documents `applyFlagSettings` with `model` and `effortLevel`; a model change during a turn can affect its next model call. The installed Claude Code 2.1.318 accepted a stream-JSON `control_request` with `subtype: apply_flag_settings` and `settings: {model, effortLevel}`. Match the response by request ID. This is an in-process flag override, not a settings-file write.
- **Codex:** [App-server](https://developers.openai.com/codex/app-server#turns) accepts `model` and `effort` on `turn/start` and makes them defaults for subsequent turns. `turn/steer` continues the current turn.
- **OpenCode:** [ACP](https://opencode.ai/v2/docs/cli/acp#config-options) uses `session/set_config_option` for `model` and `effort`; model selection changes the available effort options. The plugin already implements these calls on launch.
- **Pi:** [RPC](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc-commands.md#model) provides `set_model` and `set_thinking_level`. Its [session implementation](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/src/core/agent-session.ts) clamps unsupported thinking levels, so check the available levels before selecting one rather than reporting an unselected effort as selected.

All four have live controls; no process relaunch or harness-specific disabled selector is needed. A failed native change must not be represented as a successful selection.

## Decisions

The smallest complete path extends the existing profile selector, session fields and ordered plugin commands. Add a `configure` command carrying model and effort; it replies only after the plugin accepts the change. Existing `sessions.model` and `sessions.effort` already persist the selected values, and launch uses them on resume. Save those fields in the same transaction that applies a successful command reply and deletes the command. No saved-state format changes, so no migration is needed. Profile names are configuration labels, not saved identities today; continue displaying the saved harness/model/effort if a profile is renamed or removed.

Changing a profile is available only when the session is idle and ready. During a turn the selector is disabled; the current turn and messages steering it retain their existing settings. The user can select after the turn ends. Enforce this at the runtime and plugin as well as the UI, since a turn can start between rendering and the request. Serialize a selection with input delivery until its reply, so no new turn starts between the native change and the saved update. A stale selection is refused visibly. This avoids changing a model partway through a turn and avoids introducing a deferred-change queue.

Each plugin applies or validates both settings before replying. Codex keeps the requested values in the plugin and sends them on the next `turn/start`; it does not change an active turn. For OpenCode and Pi, whose native controls are separate, validate available model and effort where available, and if the second operation fails, restore the previous settings before allowing another input; report the failure. If restoration fails, end the plugin so the existing resume path restores the last saved settings. The change to the plugin protocol reaches existing active sessions only after those sessions restart; an older plugin's unknown-command error must leave the saved selection unchanged.

## Implementation plan

1. In `internal/harness/protocol.go` and `plugins/internal/plugin/serve.go`, add the configure command and the agent callback, preserving command order and existing reply semantics. Specify that it is accepted only between turns.
2. In `internal/session`, add a profile selection operation: resolve the profile from current configuration, reject a different harness and a session that is not ready and idle, record the command using the existing command path, and block input delivery while it is outstanding. On success, persist the command's model and effort in `commands.go` with its reply; on failure leave them unchanged and surface the error. Make the plugin's idle check authoritative for races and resume delivery after the reply. The existing `store` schema and launch fields remain unchanged.
3. In `plugins/claude`, translate configure to the single `apply_flag_settings` control request, await its response and propagate failures. In `plugins/codex`, retain the selected values and include them on each new `turn/start`. In `plugins/opencode`, reuse its model/effort option selection and retain current options across changes. In `plugins/pi`, use RPC model and thinking controls, checking the returned model and supported thinking levels. Preserve each plugin's existing steering and queued-input behavior.
4. In `internal/ui/actions.go`, add a session profile action. In `internal/ui/web/agent/profile-select.jsx` and the API client, fetch profiles for an existing session, offer only those with its harness, display the saved values, and disable the control while working, unready, or a selection is in flight. Surface refusal through the existing error path. Keep the existing new-session behavior.
5. Update `docs/usage.md` and `docs/harness-support.md` where they describe the selector and protocol, including the between-turn restriction and restart requirement for previously running plugins.

## Test plan

- Through session and UI public actions, select a same-harness profile, observe the new label and subsequent input's settings, restart/resume and observe the saved values; reject another harness, unknown profile, unready session, and selection racing a turn. Verify a selection pending a reply prevents a new input from overtaking it, and an error leaves the prior selection.
- Through each plugin's `serve` entry point with its native stand-in, verify the documented control messages, confirmation and errors, and that input during a turn still steers or queues as before. Test OpenCode and Pi second-setting failure and restoration, including failure to restore. Verify Codex puts both values on the following `turn/start`, and Claude waits for the matching control response. Test replay of an unanswered configure command across app reattachment and the old-plugin refusal.
- Browser tests exercise the same-harness choices, disabled working state, selection and visible failure. Run `make check`; give the user the development server URL for their visual check.
