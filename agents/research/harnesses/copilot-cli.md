# GitHub Copilot CLI

## 1. Overview
[docs](https://github.com/github/copilot-cli) GitHub's terminal coding agent has a public documentation/issue repository with about 11.3k stars; CLI implementation source availability and license unknown (repository license must not be assumed to cover the binary). Install with `brew install copilot-cli` or `npm install -g @github/copilot`. [verified] Installed `GitHub Copilot CLI 1.0.83`; latest release unknown.

## 2. Interactive mode in a PTY
[verified] `copilot` is interactive; `-i PROMPT` starts a turn; `--mouse=on|off` applies in alternate-screen mode, `--screen-reader` enables accessibility. Full-screen default, xterm.js compatibility and resize behavior unknown.

## 3. Non-interactive/headless and server modes
[verified] `-p PROMPT` runs non-interactively; `--output-format json` gives JSONL, `--stream on|off` controls streaming. `--acp` starts a native Agent Client Protocol server. [docs](https://github.com/github/copilot-sdk) Copilot SDK provides programmatic agent integration; whether an SDK can attach to a separately running TUI is unknown.

## 4. Conversation identity
[verified] `--session-id UUID` sets a new session ID or resumes an existing session/task; `--resume=ID` accepts ID, prefix or name; `--continue` picks most recent. `--name` sets a human name. [docs](https://docs.github.com/en/copilot/how-tos/copilot-cli/use-hooks) `$COPILOT_HOME` defaults to `~/.copilot` for state/config. Exact conversation storage paths, observing chosen IDs and simultaneous access to the same session unknown.

## 5. Hooks and events
[docs](https://docs.github.com/en/copilot/how-tos/copilot-cli/use-hooks) JSON files under `.github/hooks/` or `$COPILOT_HOME/hooks/` configure `sessionStart`, `sessionEnd`, `userPromptSubmitted`, `preToolUse`, `postToolUse`, `errorOccurred`, and `agentStop`; changes load on startup. Commands use `bash`/`powershell`, optional `cwd`, `env`, `timeoutSec`. Permission-specific event and stop-block/context-injection capabilities unknown; see [hook reference](https://docs.github.com/en/copilot/reference/hooks-reference). [verified] `COPILOT_HOME` overrides state and config per process, but isolating it also changes authentication/state; `--plugin-dir` loads a local plugin per process, whether it injects hooks is unknown.

## 6. Plugins/extensions
[verified] `--plugin-dir DIRECTORY` loads local plugins per process, repeatable; `--extension-sdk-path DIRECTORY` overrides SDK used by extension subprocesses. Extension API capabilities and hook packaging unknown.

## 7. MCP
[verified] `--additional-mcp-config JSON` or `@FILE` augments `~/.copilot/mcp-config.json` per process; repeatable. `--disable-builtin-mcps` and `--disable-mcp-server NAME` control servers. [docs](https://github.com/github/copilot-cli) GitHub MCP server is included. Supported transports unknown.

## 8. Instructions and customization
[verified] `COPILOT_CUSTOM_INSTRUCTIONS_DIRS` adds instruction search directories; `--no-custom-instructions` disables AGENTS.md and related files; `--agent NAME` chooses custom agent, `--model MODEL`, `--effort LEVEL` and provider environment `COPILOT_PROVIDER_BASE_URL`, `COPILOT_PROVIDER_TYPE`, `COPILOT_PROVIDER_API_KEY` support customization. `--allow-tool`, `--deny-tool`, `--allow-all-tools`, `--allow-all-paths`, `--allow-all-urls` control permissions. Explicit system-prompt override and custom command loading unknown.

## 9. Getting messages INTO a running interactive session from outside
[verified] `--acp` exposes an ACP server instead of starting ordinary TUI; `--remote` enables GitHub remote control. [docs](https://github.com/github/copilot-sdk) SDK can send prompts to sessions it manages. Whether ACP, SDK or remote control can attach to the exact locally running TUI, inject while busy, or wake idle without disturbing the input editor is unknown. PTY typing is a fallback, with editor interference risk.

## 10. Status detection from outside
[docs](https://docs.github.com/en/copilot/how-tos/copilot-cli/use-hooks) `userPromptSubmitted`, `agentStop`, tool hooks and session hooks provide partial lifecycle signals. Permission-wait and ask-user notifications for an existing TUI unknown; ACP/SDK event mapping needs validation.

## 11. Usage and cost reporting
[verified] `--usage-output-file FILE` writes final usage JSON; `--max-ai-credits` caps session AI credits. `COPILOT_OTEL_FILE_EXPORTER_PATH` enables JSONL OpenTelemetry traces and metrics with token counts and cost attributes; disabled by default. [docs](https://github.com/github/copilot-cli) Copilot premium requests are quota billed; exact price and interpretation of metrics depend on plan.

## 12. Gotchas for running many instances side by side
[verified] `COPILOT_HOME` controls shared configuration/state; `--log-dir` sets logs; `--no-auto-update` disables auto-download. `COPILOT_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN` provide auth in that precedence order. Parallel session locks, state conflicts, and rate limits unknown. Avoid `--allow-all` unless intentionally granting tools, paths and URLs.

## 13. Fit for Hypercube
- [verified] Launch `copilot --session-id UUID -i PROMPT`, resume with saved UUID; add Hypercube MCP with `--additional-mcp-config`.
- [verified] Native `--acp` and per-process `--plugin-dir` are promising integration surfaces, but TUI attachment is unverified.
- [docs](https://docs.github.com/en/copilot/how-tos/copilot-cli/use-hooks) Hook events offer partial status; per-process isolation and permission/idle wake semantics remain gaps.
- [verified] JSON usage output and opt-in OpenTelemetry supply usage, while messages to an independent interactive TUI remain unresolved.
