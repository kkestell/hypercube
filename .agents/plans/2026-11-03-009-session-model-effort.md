# Change a session's model and effort

## Goal

Allow a running session to select a configured profile with the same harness while ready. Save the resulting model and effort for later turns and resumes. A session never changes harness.

## Native controls

- **Claude Code:** The [Agent SDK reference](https://code.claude.com/docs/en/agent-sdk/typescript#applyflagsettings) documents `applyFlagSettings` with `model` and `effortLevel`; a model change during a turn can affect its next model call. The installed Claude Code 2.1.318 accepted a stream-JSON `control_request` with `subtype: apply_flag_settings` and `settings: {model, effortLevel}`. Match the response by request ID. This is an in-process flag override, not a settings-file write.
- **Codex:** [App-server](https://developers.openai.com/codex/app-server#turns) accepts `model` and `effort` on `turn/start` and makes them defaults for subsequent turns. `turn/steer` continues the current turn.
- **OpenCode:** [ACP](https://opencode.ai/v2/docs/cli/acp#config-options) uses `session/set_config_option` for `model` and `effort`; model selection changes the available effort options. The plugin already implements these calls on launch.
- **Pi:** [RPC](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc-commands.md#model) provides `set_model` and `set_thinking_level`. Its [session implementation](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/src/core/agent-session.ts) clamps unsupported thinking levels, so read back the resulting thinking level and report the level Pi selected.

All four have live controls; no process relaunch or harness-specific disabled selector is needed.

## Decisions

Extend the existing profile selector, session fields and ordered plugin commands. Add a synchronous `configure` command carrying model and effort. Its reply optionally carries the resulting pair, even when an error reports a partial change; absence of the pair means nothing was accepted. Apply a reported pair to `sessions.model` and `sessions.effort` in the same transaction that applies the reply and deletes the command. Existing session fields and launch arguments already preserve these values across restarts, so no saved-state migration is needed. Profile names are configuration labels, not saved identities; continue displaying the saved harness/model/effort if a profile is renamed or removed.

The selector is available whenever the session is ready, including while it is working. Changes take effect at each harness's native timing: Claude may use the new model on its next model call, Codex uses the new values on the next `turn/start`, and OpenCode and Pi apply their native controls when called. The server validates readiness and the harness match. Errors remain visible, including an error after a partial change, and the displayed values reflect the pair reported by the plugin.

Commands already run in order: the store reads them by ID, `session/commands.go` serializes sends, and `plugin/serve.go` handles and replies to each before reading the next. No additional input barrier is needed. Codex's configure callback retains values for the next `turn/start`; if that later request is refused, the existing input error path reports it. The existing command replay and unknown-command error handling cover reattachment and old plugins. Active sessions need to restart to load the plugin protocol changes.

## Implementation plan

1. In `internal/harness/protocol.go` and `plugins/internal/plugin/serve.go`, add the configure command, a synchronous agent callback, and an optional resulting model/effort pair on the reply. Include the pair even alongside an error when a native operation partially changed the selection.
2. In `internal/session`, add a profile selection operation: resolve the configured profile, reject a different harness or a session that is not ready, and record the command using the existing command path. In `commands.go`, save a reported pair with the reply and surface any error; when the reply has no pair, leave the stored values unchanged. Keep the existing store schema, launch fields, command replay, and error handling.
3. In `plugins/claude`, send the `apply_flag_settings` control request, await its matching response, and update `c.model` for the usage-window calculation in `events.go` when the model changes. In `plugins/codex`, retain the requested values and include them on each subsequent `turn/start`, without extra validation. In `plugins/opencode`, reuse its model/effort option selection and report the resulting pair even if the effort selection fails after the model changes. In `plugins/pi`, use RPC model and thinking controls, read back the resulting thinking level, report the actual pair after partial changes, and update the cached context window used by `events.go` when the model changes. Preserve native steering and queued-input behavior.
4. In `internal/ui/actions.go`, add a session profile action. In `internal/ui/web/agent/profile-select.jsx` and the API client, fetch profiles for an existing session, offer only those with its harness, display the saved values, and enable selection whenever ready. Surface errors through the existing error path. Keep the existing new-session behavior.
5. Update `docs/usage.md` and `docs/harness-support.md` where they describe the selector and protocol, including native timing and the restart requirement for previously running plugins.

## Test plan

- Through session and UI public actions, select a same-harness profile while idle and while working, observe the resulting label and settings, restart/resume and observe the saved pair. Reject another harness, unknown profile, and an unready session. Verify errors remain visible and a partial-change reply saves its reported pair; a reply with no pair leaves the previous selection.
- Through each plugin's `serve` entry point with its native stand-in, verify native controls, successful and partial-change replies, model-derived usage-window updates, and that input during a turn still steers or queues as before. Verify Codex sends both values on the next `turn/start` and surfaces its refusal, Claude waits for its matching control response, and Pi reports the actual selected effort. Include one focused test of an unanswered configure command across app reattachment.
- Browser tests exercise same-harness choices, selection while working, readiness and visible failure. Run `make check`; give the user the development server URL for their visual check.
