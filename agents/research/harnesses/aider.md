# Aider

Researched 2026-10-29. **Not installed** on this machine (`which aider` found nothing [verified]). I did not install it, per the brief. Every fact below is therefore [docs], meaning read from the linked page or from source at `main` on GitHub (read via raw URLs, not run). The source may be ahead of the PyPI release.

Tags:
- **[verified]**: run here. Nothing about Aider was run.
- **[docs]**: from the linked page or source. Pages were read through a summarizing fetch, so names are as extracted.
- **unknown**: not established.

Source links, shortened: `args.py`, `main.py`, `io.py`, `coders/base_coder.py` are at `https://raw.githubusercontent.com/Aider-AI/aider/main/aider/`.

## 1. Overview

- What: "AI pair programming in your terminal" [docs](https://pypi.org/pypi/aider-chat/json).
- Vendor: GitHub org `Aider-AI` [docs](https://github.com/Aider-AI/aider).
- License: Apache-2.0 [docs](https://github.com/Aider-AI/aider). The PyPI classifiers say Apache Software License [docs](https://pypi.org/pypi/aider-chat/json).
- Version: 0.86.2 is the latest on PyPI as of 2026-10-29 [docs](https://pypi.org/pypi/aider-chat/json). Release date: unknown (the fetch cut off the release list).
- Install: the GitHub page says `python -m pip install aider-install`, then `aider-install` [docs](https://github.com/Aider-AI/aider). The PyPI package is `aider-chat`, which requires Python `>=3.10,<3.13` [docs](https://pypi.org/pypi/aider-chat/json).
- Adoption: 49.8k GitHub stars at fetch time [docs](https://github.com/Aider-AI/aider).

## 2. Interactive mode in a PTY

- It is a line-oriented REPL, not a full-screen app. Input is a `prompt_toolkit` `PromptSession` with prompt `> ` (plus `multi` in multiline mode), inside a `while True` loop [docs: `io.py`]. The `reserve_space_for_menu=4` argument only reserves room for the completion menu.
- Output uses `rich`. Assistant replies render as Markdown [docs: `io.py`].
- Colors: `--pretty/--no-pretty` (default on) and env `AIDER_PRETTY` [docs](https://aider.chat/docs/config/options.html). `NO_COLOR` set to a non-empty value turns pretty off. Dumb terminals force it off [docs: `io.py`].
- `--dark-mode` and `--light-mode` pick colors for the terminal background [docs](https://aider.chat/docs/config/options.html).
- Streaming: `--stream/--no-stream`, default on [docs: `args.py`].
- Resize: no SIGWINCH handler and no terminal-size query. The console is created as `Console()` with no size [docs: `io.py`]. prompt_toolkit may handle resize internally; unknown.
- Behavior under xterm.js: unknown, untested. The points to test are prompt_toolkit's terminal queries, bracketed paste, and rich colors.
- Input history: `.aider.input.history` (`--input-history-file`), read and written by prompt_toolkit's `FileHistory` [docs: `io.py`, options page].

## 3. Headless and server modes

- One-shot: `aider --message/--msg/-m "<text>"` sends one message, processes the reply, and exits. The docs say it "disables chat mode" [docs](https://aider.chat/docs/config/options.html). Exit: `main()` returns nothing, which becomes `sys.exit(None)`, so the exit code is 0. `--message-file` returns 1 when the file is missing or unreadable. Other exceptions from `coder.run` are not caught in that branch [docs: `main.py`].
- `--message-file/-f FILE`: same, with the message read from a file [docs](https://aider.chat/docs/config/options.html).
- `--commit`: commit pending changes with a generated message, then exit [docs: `args.py`].
- `--lint` and `--test`: fix or test the files, then exit [docs: `args.py`].
- Machine-readable output: no flag emits JSON. Usage appears as a text line (§11) [docs: `args.py`].
- Server mode: none. `--gui/--browser` starts a local Streamlit app, not an API [docs: `main.py`].
- ACP: no mention in the docs, the FAQ, or source flags. Unknown beyond that.
- MCP: no native client. Source has no MCP code [docs: `args.py`, `main.py`]. The docs page `https://aider.chat/docs/config/mcp.html` returned HTTP 404 [docs]. Unofficial MCP wrappers exist, for example `disler/aider-mcp-server` and `sengokudaikon/aider-mcp` [docs: search results, not verified].
- Python API: `from aider.coders import Coder`, `Coder.create(main_model=..., fnames=...)`, `coder.run("...")`. The docs say the API "is not officially supported or documented" and "could change in future releases" [docs](https://aider.chat/docs/scripting.html). The docs show both `main_model=` and `model=` in different examples.

## 4. Conversation identity

- No session or conversation ID. The request path creates none [docs: `base_coder.py`]. The only hashes are per request, per response, and git commit SHAs.
- The conversation is a Markdown file, the chat history file. Default `.aider.chat.history.md`, computed from the git root when a repo is found, else the bare name in the cwd [docs: `args.py`]. Set it with `--chat-history-file` or env `AIDER_CHAT_HISTORY_FILE` [docs](https://aider.chat/docs/config/options.html).
- Writes: append mode (`open("a")`). Aider writes `# aider chat started at <time>` on each start, and logs user input, assistant output, tool messages, and confirm/prompt answers [docs: `io.py`].
- Resume: `--restore-chat-history` reads the file at startup and splits it into messages with `split_chat_history_markdown`. If the history is too large for the model's `max_chat_history_tokens`, it summarizes in a background thread. No cap is applied at read time [docs: `base_coder.py`].
- So a per-agent path works as the caller-chosen identity: `--chat-history-file <dir>/<agent>.md --restore-chat-history`. Not run.
- Several instances in one directory: they share the default chat file unless each sets its own path. Appends from two processes would interleave. The `io.py` excerpt shows no lock; unknown.
- Auto-commits are on by default [docs: options; `args.py`]. Two agents committing into one repo can collide in the git index. That is inferred, not tested.

## 5. Hooks and events

- None found. No hook, event, or callback API appears in the docs, the FAQ, or the flags in `args.py` [docs].
- The only outbound callback is the notification command (§10).
- `--load FILE` (env `AIDER_LOAD`) runs `/commands` from a file at launch only. It runs after startup notices and before the message branch [docs: options; `main.py`].

## 6. Plugins and extensions

- None documented. The extension points are config files (`.aider.conf.yml`, §8), `.env`, `--read` for read-only files, and `--load` for a command file [docs].
- The Python API is explicitly unsupported (§3).

## 7. MCP

- No native MCP client (§3). Source and docs show none [docs].
- Unofficial wrappers exist (§3).

## 8. Instructions and customization

- System prompt: no flag overrides it. `--commit-prompt` only affects commit messages. `--show-prompts` prints the system prompts and exits, for debugging [docs: `args.py`].
- Read-only context: `--read FILE`, repeatable [docs](https://aider.chat/docs/config/options.html).
- Per-process config: every option maps to an env var `AIDER_<OPTION>` through configargparse's `auto_env_var_prefix="AIDER_"` [docs: `args.py`]. The config file is YAML, given by path with `--config FILE` or `-c` [docs: `args.py`]. Stdin and URLs are not accepted [docs: `args.py`].
- Config search, when `--config` is not given: cwd, then git root, then home. The list is reversed, so the cwd file is last and wins [docs: `main.py`]. Whether `--config` skips that search: unknown. `args.py` does not settle it.
- `.env`: loaded automatically from the git root by default. `--env-file` changes it [docs](https://aider.chat/docs/config/options.html).
- Models: `--model`, `--editor-model`, `--weak-model` (used for commit messages and history summaries), `--architect` (architect edit format) [docs: options; `args.py`]. `--api-key provider=<key>` and `--set-env` set keys per process [docs: `args.py`].
- Agents, subagents, custom commands: none beyond built-in `/` commands [docs].
- Permission modes: none. Confirmations are the only gate. `--yes-always` answers yes [docs]. In `io.py`, when `yes` is set, confirmations marked `explicit_yes_required` still answer no, so `--yes-always` does not approve everything [docs: `io.py`].
- Git behavior: `--auto-commits`, `--dirty-commits`, `--gitignore`, `--dry-run`, `--stream` are all `BooleanOptionalAction`, so each has a `--no-` form [docs: `args.py`].

## 9. Getting messages into a running session

- No API, socket, or hook. The options are:
  - Typing into the PTY prompt: unknown, untested.
  - `--watch-files` (env `AIDER_WATCH_FILES`, default off): aider scans the repo for one-line comments that start or end with `AI`, `AI!` or `AI?`, using `#`, `//` or `--`. `AI!` asks for a change and `AI?` asks a question. Edits made in any editor count [docs](https://aider.chat/docs/usage/watch.html). Costs: the comments go into the user's source files, and the LLM is asked to remove them. Only one-line comments are read.
  - `--load` runs only at launch (§5).
- A relaunch per message: `--message` with `--restore-chat-history` and a per-agent chat file gives one process per turn and keeps history across turns. This is a design option from source; not run.
- There is no hook that can hold the turn or feed text back at turn end. The Stop-block pattern in `harness-support.md` has no equivalent here.

## 10. Status detection from outside

- The one signal is the notification. With `--notifications` or `--notifications-command`, aider rings the bell or runs the command "when the LLM has finished generating a response and is waiting for your input" [docs](https://aider.chat/docs/usage/notifications.html). In source, the bell rings at the start of `get_input`, `confirm_ask` and `prompt_ask`, but only if an LLM call started since the last ring (`llm_started()` sets the flag) [docs: `io.py`].
- The command runs through the shell with `shell=True` and `capture_output=True`. It gets no arguments and no environment variables [docs: `io.py`]. The agent's identity must be written into the command string for each process.
- Defaults when `--notifications` is set and no command is given: `terminal-notifier` or `osascript` on macOS, `notify-send` or `zenity` on Linux, PowerShell on Windows. Otherwise a BEL character [docs: `io.py`; docs page].
- Waiting for input and waiting for confirmation use the same path, so a notification cannot tell them apart [docs: `io.py`].
- Busy: no signal. Unknown.
- Permission: `confirm_ask` is the only permission-like state, and it shares the input-wait signal. Under `--yes-always`, most confirmations never prompt [docs: `io.py`].
- The prompt is `> `, so PTY output could be parsed. That is fragile and untested.

## 11. Usage and cost

- After each message, aider prints `Tokens: <n> sent, <n> received. Cost: $<x> message, $<y> session.` [docs: `base_coder.py`]. Cache write and cache hit fragments appear only when nonzero. If the model's metadata lacks `input_cost_per_token`, the cost part is dropped [docs: `base_coder.py`].
- The session total is kept in memory by the process [docs: `base_coder.py`]. Whether it survives a restart: unknown.
- `--llm-history-file FILE` (env `AIDER_LLM_HISTORY_FILE`) appends each message sent to the LLM and each response, with a header line [docs: `io.py`, `base_coder.py`]. The logged content is prompts and replies, not usage numbers.
- `--analytics-log FILE` logs analytics events [docs](https://aider.chat/docs/config/options.html). Contents: unknown.
- No JSON or machine-readable usage output [docs: `args.py`]. Parsing the text line is the only route.

## 12. Gotchas

- Analytics: `main.py` gates the opt-in prompt on `args.analytics is not False` and calls `analytics.enable()` only if allowed. If analytics is unset, an interactive prompt "Allow collection of anonymous analytics to help improve aider?" can appear [docs: `main.py`]. Pass `--no-analytics` per process (the flag exists via `BooleanOptionalAction` [docs: `args.py`]). The options page says the default is "random" [docs](https://aider.chat/docs/config/options.html).
- Git writes by default: `--auto-commits` and `--dirty-commits` are on, and `--gitignore` is on, which adds `.aider*` to the repo's `.gitignore` [docs: options; `args.py`]. For Hypercube, set these per process.
- Files written into the repo root by default: `.aider.chat.history.md`, `.aider.input.history`, `.aiderignore` [docs: `args.py`].
- `.aider.conf.yml` and `.env` are loaded from the cwd, the git root and home [docs: `main.py`, options]. User config can change an agent's behavior.
- `--yes-always` does not approve everything (§8).
- Docs disagree: the scripting page uses `--yes`; the options page and `args.py` use `--yes-always` [docs]. Use `--yes-always`.
- Nothing here was run. The PyPI package requires Python `>=3.10,<3.13` [docs].

## 13. Fit for Hypercube

- Launch: per-agent YAML via `--config`, or `AIDER_*` env vars. Set `--chat-history-file <per-agent path>`, `--restore-chat-history`, `--no-analytics`, `--yes-always`, `--no-gitignore`, `--notifications-command '<hypercube command with agent id>'`. All flags exist in source or docs; none was run.
- Conversation: the per-agent chat file is the resume key. A turn can run as a `--message` relaunch, so a long-lived process is not required.
- Status: idle only, via the notification command. Busy and permission waits cannot be separated. This is a gap.
- Message delivery: PTY typing, or `--watch-files` comments that edit user files. No hook can hold a turn or feed text at turn end, so the Stop-block design in `harness-support.md` does not map. This is a gap.
- Missing: session IDs, hooks, MCP, JSON output, and a machine-readable usage report. The plugin would have to parse the text line.
