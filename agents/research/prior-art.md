# Prior art: running several coding agents at once

Survey date: 2026-10-30. Star counts and push dates come from the GitHub API on
that date. Each fact carries a tag: **[docs](url)** for documentation, READMEs,
changelogs, and issue threads; **[source](url)** for source code or repository
metadata; **[inferred]** for conclusions drawn from those sources without a
direct statement.

Parts: 1, local terminal tools; 2, Claude Code's own multi-agent features;
3, local desktop and web apps (including vendor agent apps); 4, hosted
platforms; 5, protocols; 6, executable plugin conventions; 7, projects judged
not notable. The feature inventory is at the end.

---

# Part 1. Local terminal tools

## Herdr

### What it is

- Local terminal workspace manager and "runtime your coding agents live on";
  single Rust binary, runs inside any terminal; Linux, macOS, Windows
  **[docs](https://github.com/herdrdev/herdr)**.
- Open source, Apache-2.0 (relicensed from AGPL-3.0)
  **[docs](https://github.com/herdrdev/herdr/issues/1340)**. Active: v0.9.5 on
  2026-10-14, last push 2026-10-29, ~46.9k stars, Homebrew and mise packages
  **[source](https://github.com/herdrdev/herdr)**.

### Features

- **Structure**: sessions (separate server namespaces) contain workspaces (one
  per repo or task), which contain tabs, which contain panes; every pane is a
  real PTY **[docs](https://herdr.dev/docs/concepts/)**.
- **Creating agents**: run any agent CLI in a pane; `herdr agent start` starts a
  supported agent in an idle shell pane and returns once it is ready for input
  **[docs](https://github.com/herdrdev/herdr/blob/main/skills/herdr/SKILL.md)**.
- **Worktrees**: "New worktree" on a Git workspace row creates a checkout under
  `~/.herdr/worktrees/<repo>/<branch-slug>` and opens it as a grouped child
  workspace; "Delete worktree checkout" runs `git worktree remove`, asks again
  before forcing; branches are never deleted
  **[docs](https://herdr.dev/docs/configuration/)**.
- **Watching**: every agent pane is `blocked`, `working`, `done` (finished, not
  yet viewed), `idle`, or `unknown`; state rolls up from pane to tab to
  workspace in the sidebar; each client tracks its own "seen" state
  **[docs](https://herdr.dev/docs/concepts/)**,
  **[docs](https://herdr.dev/docs/agents/)**.
- **Notifications**: in-app toast, outer-terminal notification (works over
  SSH), OS notification (terminal-notifier/osascript on macOS, Notification
  Center on Windows with click-to-focus pane), or off; suppressed for the active
  tab; per-event sounds (`done_path`, `request_path`)
  **[docs](https://herdr.dev/docs/configuration/)**.
- **Persistence**: background server keeps panes running on client detach
  (`ctrl+b q`) or SSH drop; after server restart restores layout, cwd, and
  focus; optional pane screen-history replay (off by default because it can
  hold secrets); native agent resume using session IDs reported by integrations
  (`claude --resume <id>`, `codex resume <id>`, `opencode --session <id>`, and
  ~15 more); live handoff that transfers PTYs to a replacement server during
  updates; 48 rolling layout snapshots
  **[docs](https://herdr.dev/docs/session-state/)**.
- **Multiple clients and machines**: several clients can attach to one server
  with independent views; `herdr --remote host` draws the UI locally while the
  remote server owns panes; saved SSH machines appear in one window with a
  combined agent list; clipboard image paste bridged to the remote host
  **[docs](https://herdr.dev/docs/persistence-remote/)**.
- **Agent-to-agent coordination**: agents inside panes use the CLI to split
  panes, start helper agents, `agent prompt <name> "..." --wait`, `agent wait
  --until blocked`, `agent read`, and `pane wait-output --match`; prompts can be
  forwarded to agents on another machine with `--machine`
  **[docs](https://github.com/herdrdev/herdr/blob/main/skills/herdr/SKILL.md)**.
- **UI**: tmux-style prefix keys plus full mouse support (click, drag splits,
  right-click menus); themes with light/dark auto-switch; zoom, swap, resize
  panes **[docs](https://herdr.dev/docs/concepts/)**,
  **[docs](https://herdr.dev/docs/configuration/)**.
- **Configuration**: `~/.config/herdr/config.toml`, `herdr --default-config`,
  live `herdr server reload-config`; headless virtual terminal size for
  clientless orchestration **[docs](https://herdr.dev/docs/configuration/)**.
- No built-in diff review, PR creation, cost tracking, or task board in core
  docs; plugins add some of these (e.g. a worktree/PR/CI panel plugin)
  **[source](https://github.com/minhtran3124/herdr-worktree-agents)**,
  **[inferred]**.

### Harness integration

- Harness-agnostic terminal host: "doesn't wrap or replace them; it owns their
  terminals" **[docs](https://github.com/herdrdev/herdr)**.
- Detection of ~23 agents (Claude Code, Codex, Copilot CLI, Cursor Agent,
  OpenCode, Pi, Droid, Devin CLI, Kimi, Kilo, Qwen, Grok, Antigravity, Amp,
  Gemini CLI, Cline, ...) from the pane's foreground process plus per-agent
  TOML "detection manifests" that match the live bottom of the screen;
  manifests auto-update from herdr.dev and can be overridden locally in
  `~/.config/herdr/agent-detection/<agent>.toml`
  **[docs](https://herdr.dev/docs/agents/)**.
- `herdr integration install <agent>` writes hooks into the harness's own
  config: for Claude Code, `~/.claude/hooks/herdr-agent-state.sh` plus
  SessionStart entries in `settings.json`; for Codex, a script plus
  `hooks.json` and `[features] hooks = true` in `config.toml`; for Pi, an
  extension file in `~/.pi/agent/extensions/`. Hooks report session identity
  (for resume); Pi, OMP, Kimi, OpenCode, Kilo, MastraCode hooks also report
  state, replacing screen reading
  **[docs](https://herdr.dev/docs/integrations/)**.
- Reverse integration: any agent can report its own state and resume command by
  calling `"$HERDR_BIN_PATH" pane report-agent "$HERDR_PANE_ID" --state working
  --seq N -- <resume argv>` when `HERDR_ENV=1`; Crush, Kiro CLI, Command Code,
  and others do this natively
  **[docs](https://herdr.dev/docs/add-herdr-support/)**.
- Sandbox wrappers: `HERDR_AGENT=claude fence -- claude` tells Herdr which
  manifest to use when a wrapper hides the agent process
  **[docs](https://herdr.dev/docs/agents/)**.

### Observation and control

- State: screen manifests or hook reports; strict "blocked" detection (only on
  a recognized approval/question UI, otherwise falls back to idle);
  `herdr agent explain` shows the matched rule and evidence
  **[docs](https://herdr.dev/docs/agents/)**.
- Transcripts: `agent read` / `pane read --source recent|recent-unwrapped`;
  for Claude Code it can read the transcript file
  **[docs](https://github.com/herdrdev/herdr/issues/5462)**.
- Control: `agent prompt` (bracketed paste + Enter; refuses with
  `agent_blocked` if an approval dialog is showing), `agent send-keys`,
  `pane send-text/send-keys`; approvals are answered by sending keys to the
  pane **[docs](https://github.com/herdrdev/herdr/blob/main/skills/herdr/SKILL.md)**.
- No usage or cost tracking found in docs **[inferred]**.

### Extension points

- CLI (most commands return JSON), local socket API (Unix socket / Windows
  named pipe) with JSON Schema via `herdr api schema --json`; methods for
  workspace, worktree, tab, pane, layout, agent, notification, and
  `events.subscribe` **[docs](https://herdr.dev/docs/socket-api/)**.
- Plugins: a directory with `herdr-plugin.toml` declaring build commands,
  startup commands, actions, event hooks (e.g. `worktree.created`), panes
  (overlay placement), and link handlers; any language; "the entire Herdr CLI
  is the plugin API"; not sandboxed; marketplace at herdr.dev/plugins
  **[docs](https://herdr.dev/docs/plugins/)**.
- Agent skill file (`skills/herdr/SKILL.md`, installable with `npx skills add`)
  teaches agents to drive Herdr **[docs](https://herdr.dev/docs/agent-skill/)**.

### Known problems

- Screen-detection misses: Claude Code `AskUserQuestion` shown idle when
  scrolled up (#5593), Codex "Implement this plan?" detected idle (#5490),
  Gemini CLI not appearing (#5533), nix-wrapped Claude not detected (#803)
  **[docs](https://github.com/herdrdev/herdr/issues/5593)**,
  **[docs](https://github.com/herdrdev/herdr/issues/5490)**,
  **[docs](https://github.com/herdrdev/herdr/issues/803)**.
- Docs warn that automation trusting a mistaken idle state can type into an
  approval dialog; Antigravity and Grok report idle while subagents still run
  **[docs](https://herdr.dev/docs/agents/)**.
- Resume bugs: stale session IDs after Claude `/clear` or fork (#5547, #5557);
  Codex 0.157 shared daemon breaks per-pane sessions (#4649)
  **[docs](https://github.com/herdrdev/herdr/issues/5547)**,
  **[docs](https://github.com/herdrdev/herdr/issues/4649)**.
- Server memory growth to 16 GB with a stalled client (#5471); remote stream
  stalls (#5624); 60 Hz render cap is the most-upvoted issue (#1134)
  **[docs](https://github.com/herdrdev/herdr/issues/5471)**,
  **[docs](https://github.com/herdrdev/herdr/issues/1134)**.
- tmux inside a pane hides the agent from detection
  **[docs](https://herdr.dev/docs/agents/)**.

---
## agent-deck

### What it is

- Local TUI session manager for terminal coding agents, built in Go on tmux;
  also serves a web UI. macOS, Linux, Windows via WSL
  **[docs](https://github.com/asheshgoplani/agent-deck)**.
- Open source, MIT. Very active: v1.16.34 on 2026-10-25, ~1.2k stars
  **[source](https://github.com/asheshgoplani/agent-deck)**.

### Features

- **Creating**: `n` dialog or `agent-deck add . -c claude`; first-run welcome
  detects installed agents and login state and can run the tool's own login
  **[docs](https://github.com/asheshgoplani/agent-deck#quick-start)**.
- **Organizing**: nested groups (declarable in `config.toml` with default
  paths), profiles (`-p work`) with per-profile and per-group
  `CLAUDE_CONFIG_DIR`/env files, move sessions between groups, archive and
  unarchive, `Ctrl+Z` undo of deletes
  **[docs](https://github.com/asheshgoplani/agent-deck#features)**.
- **Watching**: status per session (running, waiting, idle, error); waiting
  sessions listed in the tmux status bar with jump keys; fuzzy search with
  status filters; "Recall" search across every conversation on the machine
  **[docs](https://github.com/asheshgoplani/agent-deck#status-detection)**.
- **Messaging**: composer sends to the selected session; `Shift+Q` queues a
  message after the current turn (native queueing for Claude/OpenCode 2);
  `agent-deck session send <s> --message-file`; `session ref` types a
  reference to one session into another without sending
  **[docs](https://github.com/asheshgoplani/agent-deck#key-shortcuts)**.
- **Forking**: `f` forks a session using each tool's native fork (Claude,
  OpenCode, Pi, Codex, Oh My Pi) into a new worktree and branch, carrying
  uncommitted state and Docker isolation
  **[docs](https://github.com/asheshgoplani/agent-deck#forking-sessions)**.
- **Worktrees**: `--worktree <branch> --new-branch`; sibling, subdirectory,
  or custom location; sparse-checkout inheritance; `.worktreeinclude` copies
  gitignored files; per-repo setup and destroy scripts that need approval;
  bare-repo layouts; jj workspaces; `worktree finish` merges the branch,
  removes the worktree, and deletes the session; `worktree cleanup` removes
  orphans **[docs](https://github.com/asheshgoplani/agent-deck#git-worktrees)**.
- **Sandboxing**: optional Docker container per session with the project
  bind-mounted; host tool auth shared into containers (except macOS Claude
  Keychain login) **[docs](https://github.com/asheshgoplani/agent-deck#docker-sandbox)**.
- **Conductor**: a persistent agent session (Claude or Codex) that supervises
  other sessions, auto-answers routine questions, and escalates to the user
  via Telegram, Slack, Discord, or Mattermost; child sessions launched from it
  are parent-linked and send transition nudges (`running -> waiting|error|idle`);
  15-minute heartbeat **[docs](https://github.com/asheshgoplani/agent-deck#conductor)**.
- **Watchers**: inbound webhook, GitHub, ntfy, and Slack adapters forward
  events to a conductor with `session send`
  **[docs](https://github.com/asheshgoplani/agent-deck#watchers)**.
- **Cost tracking**: Claude hook reads transcripts each turn; TUI (`$`) and web
  dashboards by day/week/month, session, model, group; budgets with 80% warning
  and optional hard stop; CSV/JSON export; status-line cost segment
  **[docs](https://github.com/asheshgoplani/agent-deck#cost-tracking-dashboard)**.
- **Provider quota**: `agent-deck usage` shows Claude 5-hour and 7-day plan
  usage read from the `rate_limits` block Claude Code passes to a `statusLine`
  command **[docs](https://github.com/asheshgoplani/agent-deck#provider-quota-agent-deck-usage)**.
- **MCP and skills managers**: toggle MCP servers per session or globally with
  automatic restart; MCP socket pool shares one MCP process across sessions;
  attach Claude skills from a pool into `.claude/skills`
  **[docs](https://github.com/asheshgoplani/agent-deck#mcp-manager)**.
- **Context inspector** (`C`), copy last AI response, switch a session's Claude
  account with conversation migration
  **[docs](https://github.com/asheshgoplani/agent-deck#key-shortcuts)**.
- **Remote**: register SSH remotes (auto-installs agent-deck there), browse
  and attach to remote sessions, drain remote completion reports into a local
  inbox, auto-update remotes
  **[docs](https://github.com/asheshgoplani/agent-deck#remote-instances)**.
- **Web mode**: `agent-deck web` serves a browser terminal, Command Center
  fleet view, costs, archive, and settings on `127.0.0.1:8420`; token auth,
  read-only mode, browser push notifications; Fleet board kanban columns
  (Needs input, Ready for review, In progress, Parked, Done) driven by
  `session annotate --hint status=...`
  **[docs](https://github.com/asheshgoplani/agent-deck#web-mode)**.
- Opt-in usage telemetry, off by default
  **[docs](https://github.com/asheshgoplani/agent-deck#usage-telemetry-opt-in-off-by-default)**.

### Harness integration

- Per-tool integration levels: Claude Code and Gemini CLI full (status, MCP,
  fork, resume); OpenCode, OpenCode 2 (via its session API), Codex, Copilot,
  Crush, Cursor, Hermes, Pi, Oh My Pi, DeepSeek Harness at varying levels;
  custom tools via `[tools.*]` in config
  **[docs](https://github.com/asheshgoplani/agent-deck#multi-tool-support)**.
- Hooks installed into harness config: `agent-deck hooks install` writes
  Claude hooks and a wrapping `statusLine` command into each account's
  `settings.json`; `codex-hooks install` uses Codex's notify hook (required for
  reliable running/waiting status); `pi-hooks install` adds a Pi extension;
  `hermes-hooks install` **[docs](https://github.com/asheshgoplani/agent-deck#multi-tool-support)**.
- Fallback is pane polling ("slower and occasionally wrong" for Pi)
  **[docs](https://github.com/asheshgoplani/agent-deck#multi-tool-support)**.
- Passes `--name` and `--channels` to Claude; launches tools with extra args
  through a wrapper **[docs](https://github.com/asheshgoplani/agent-deck#conductor)**.

### Observation and control

- Status from hooks plus tmux pane polling; cost from Claude transcripts;
  quota from status-line payload **[docs](https://github.com/asheshgoplani/agent-deck#cost-tracking-dashboard)**.
- Control by tmux keystrokes (attach, send, queue), restart, fork, archive;
  automated permission handling only by enabling Claude's dangerous mode for
  conductors **[docs](https://github.com/asheshgoplani/agent-deck#conductor)**.

### Extension points

- Full CLI with `--json` output; HTTP API and WebSocket from web mode (e.g.
  `POST /api/sessions/{id}/fork`); webhook watchers; bundled agent skill
  (`skills/agent-deck`) teaching agents the CLI
  **[docs](https://github.com/asheshgoplani/agent-deck#web-mode)**.

### Known problems

- Terminal input regressions: arrow keys, Shift, and Shift+Enter broken in some
  terminals and versions (#539, #544, #397, #445); escape sequences leaking
  into input when switching sessions (#585)
  **[docs](https://github.com/asheshgoplani/agent-deck/issues/544)**.
- UI freezes after detaching from long sessions (#39); no scrollback in
  control-mode attach (#1491) **[docs](https://github.com/asheshgoplani/agent-deck/issues/39)**.
- Cost tracking for Gemini/Codex and budget enforcement marked "untested"
  **[docs](https://github.com/asheshgoplani/agent-deck#cost-tracking-dashboard)**.
- Interface churn: README warns the new-session dialog behavior "changed again
  after v1.16.32" **[docs](https://github.com/asheshgoplani/agent-deck#quick-start)**.

---

## Claude Squad

### What it is

- Local Go TUI (`cs`) that runs several terminal agents in tmux sessions, each
  in its own git worktree; Claude Code by default, also Codex, Gemini, Aider,
  OpenCode, Amp **[docs](https://github.com/smtg-ai/claude-squad)**.
- Open source, AGPL-3.0; ~8.7k stars; last release v1.0.20 on 2026-08-20
  **[source](https://github.com/smtg-ai/claude-squad)**. Users opened
  "Project seems abandoned" and "No more updates?" issues (#250, #214)
  **[docs](https://github.com/smtg-ai/claude-squad/issues/250)**.

### Features

- **Creating**: `n` new session, `N` new session with a prompt; profiles in
  `~/.claude-squad/config.json` pick the program per session
  **[docs](https://github.com/smtg-ai/claude-squad#configuration)**.
- **Watching**: list of instances with a preview tab (pane content) and a diff
  tab **[docs](https://github.com/smtg-ai/claude-squad#menu)**.
- **Messaging**: `Enter` attaches to the tmux session to reprompt; `ctrl-q`
  detaches **[docs](https://github.com/smtg-ai/claude-squad#menu)**.
- **Review and merge**: diff tab; `s` commits and pushes the branch to GitHub
  (requires `gh`); `c` "checkout" commits changes and pauses the session; `r`
  resumes **[docs](https://github.com/smtg-ai/claude-squad#menu)**.
- **Stopping**: `D` kills a session; `cs reset` clears all instances
  **[docs](https://github.com/smtg-ai/claude-squad#usage)**.
- **Auto-accept**: `-y/--autoyes` (experimental) accepts prompts for Claude
  Code and Aider **[docs](https://github.com/smtg-ai/claude-squad#usage)**.

### Harness integration

- Program is any shell command; no hooks or adapters
  **[docs](https://github.com/smtg-ai/claude-squad#profiles)**.
- Built-in string matches per harness: auto-accepts Claude's "Do you trust the
  files in this folder?" with Enter, and detects a pending prompt by searching
  `tmux capture-pane` output for "No, and tell Claude what to do differently"
  (Claude), "(Y)es/(N)o/(D)on't ask again" (Aider), "Yes, allow once" (Gemini)
  **[source](https://github.com/smtg-ai/claude-squad/blob/main/session/tmux/tmux.go)**.

### Observation and control

- Polls `tmux capture-pane`, hashes content to decide whether the pane changed
  (working) and whether a prompt is showing
  **[source](https://github.com/smtg-ai/claude-squad/blob/main/session/tmux/tmux.go)**.
- Control by attaching to tmux or sending Enter in autoyes mode **[inferred]**.

### Extension points

- None beyond config profiles and the CLI flags
  **[docs](https://github.com/smtg-ai/claude-squad#usage)**.

### Known problems

- "Error capturing pane content" / "timed out waiting for tmux session"
  (#216, #189, #51, #132) **[docs](https://github.com/smtg-ai/claude-squad/issues/189)**.
- Requests: configurable worktree location and branch names (#86, #88),
  multi-repo sessions (#89, #56), worktree setup hooks (#260), resume after
  reboot (#212), slow TUI (#215), autoyes not working (#151)
  **[docs](https://github.com/smtg-ai/claude-squad/issues?q=sort%3Areactions-%2B1-desc)**.

---
## CCManager

### What it is

- Local terminal app (TypeScript, Ink-style TUI, compiled with Bun) that
  manages multiple coding-agent sessions across Git worktrees and projects
  without tmux; it owns the PTYs itself
  **[docs](https://github.com/kbwo/ccmanager)**,
  **[source](https://github.com/kbwo/ccmanager/blob/main/src/services/bunTerminal.ts)**.
- Open source, MIT; `npm install -g ccmanager` or `npx ccmanager`. Active:
  v4.4.11 on 2026-10-27, ~1.3k stars
  **[source](https://github.com/kbwo/ccmanager)**.

### Features

- **Agents supported**: Claude Code (default), Gemini CLI, Codex CLI, Cursor
  Agent, Copilot CLI, Cline CLI, OpenCode, Kimi CLI, MiniMax Code (`mcode`),
  Grok Build **[docs](https://github.com/kbwo/ccmanager#supported-ai-assistants)**.
- **States**: `idle`, `busy`, `waiting_input` shown per session in the menu
  **[docs](https://github.com/kbwo/ccmanager/blob/main/docs/status-hooks.md)**.
- **Worktrees**: create, merge, and delete worktrees from the menu; creation
  can continue in the background; auto-generated directory paths from a
  `{branch}` pattern; `.worktreeinclude` copies gitignored files ("same file
  name and semantics as Claude Code, Conductor, OpenAI Codex, and
  `git-worktreeinclude`"); post-creation worktree hook with path/branch/root
  env vars **[docs](https://github.com/kbwo/ccmanager#copying-gitignored-files-into-new-worktrees)**.
- **Git status per worktree** (with `extensions.worktreeConfig`): `+10 -5`
  changes, `↑3 ↓1` ahead/behind the parent branch, parent branch name
  **[docs](https://github.com/kbwo/ccmanager#git-worktree-configuration)**.
- **Session data copying**: copies `~/.claude/projects/[source-path]` to the new
  worktree's project dir so a Claude conversation continues in a new branch
  **[docs](https://github.com/kbwo/ccmanager#session-data-copying)**.
- **Restore after restart**: records open sessions in
  `~/.config/ccmanager/sessions.json` as they change and offers "Restore / Don't
  restore" on next start; restore re-runs the command preset, not the
  conversation or screen output **[docs](https://github.com/kbwo/ccmanager#restoring-sessions-after-a-restart)**.
- **Command presets** with primary args (e.g. `--resume`) and fallback args,
  retrying with no args last **[docs](https://github.com/kbwo/ccmanager#command-configuration)**.
- **Status change hooks**: shell commands per state with `CCMANAGER_OLD_STATE`,
  `CCMANAGER_NEW_STATE`, `CCMANAGER_WORKTREE_PATH`, `CCMANAGER_WORKTREE_DIR`,
  `CCMANAGER_WORKTREE_BRANCH`, `CCMANAGER_SESSION_ID`; examples for `noti`,
  Slack webhooks, tmux status
  **[docs](https://github.com/kbwo/ccmanager/blob/main/docs/status-hooks.md)**.
- **Auto approval (experimental)**: when a session is waiting, captures up to
  300 lines of output and runs `claude --model haiku -p --output-format json
  --json-schema ...`; if `{"needsPermission": false}` it sends `\r`; timeout
  120 s; any keypress cancels; custom approver command receives
  `DEFAULT_PROMPT` and `TERMINAL_OUTPUT`
  **[docs](https://github.com/kbwo/ccmanager/blob/main/docs/auto-approval.md)**.
- **Devcontainer**: `--devc-up-command` and `--devc-exec-command` run sessions
  inside a container while the manager stays on the host
  **[docs](https://github.com/kbwo/ccmanager#devcontainer-integration)**.
- **Multi-project mode**: `--multi-project` with
  `CCMANAGER_MULTI_PROJECT_ROOT` discovers repos recursively; recent projects;
  `/` vi-like search; per-project counts `[active/busy/waiting]`
  **[docs](https://github.com/kbwo/ccmanager/blob/main/docs/multi-project.md)**.
- **Configuration**: `~/.config/ccmanager/config.json` merged with per-repo
  `.ccmanager.json`; configurable shortcuts (default `Ctrl+E` returns to menu);
  "Reload Config from Disk"; a `ccmanager-config` Claude Code / Codex plugin
  teaches agents the schema and validates it
  **[docs](https://github.com/kbwo/ccmanager#per-project-configuration)**.

### Harness integration

- One TypeScript state detector per harness (`claude.ts`, `codex.ts`,
  `gemini.ts`, `cursor.ts`, `github-copilot.ts`, `cline.ts`, `grok.ts`,
  `kimi.ts`, ...) that reads the rendered terminal buffer
  **[source](https://github.com/kbwo/ccmanager/tree/main/src/services/stateDetector)**.
- The Claude detector looks for spinner lines matching `^[✱✲✳...] \S+ing.*…`,
  a `(... tokens)` stats line, and the `─` prompt-box borders, and requires
  1500 ms of unchanged output before declaring idle, because "Claude Code
  sometimes appears idle in terminal output while still actively processing"
  **[source](https://github.com/kbwo/ccmanager/blob/main/src/services/stateDetector/claude.ts)**.
- Appends `--teammate-mode in-process` to `claude` so Claude Code agent teams
  do not try to open their own split panes inside CCManager's PTY
  **[docs](https://github.com/kbwo/ccmanager#claude-code-teammate-mode)**.
- No hooks installed into harness configs **[inferred]**.

### Observation and control

- Screen reading only for state; Claude transcripts are copied, not parsed
  **[inferred]**.
- Control by attaching to the session's PTY; auto-approval sends Enter
  **[docs](https://github.com/kbwo/ccmanager/blob/main/docs/auto-approval.md)**.

### Extension points

- Status hooks, worktree hooks, custom auto-approval command; no API or socket
  **[docs](https://github.com/kbwo/ccmanager)**.

### Known problems

- Closing CCManager kills the agent processes it started
  **[docs](https://github.com/kbwo/ccmanager#restoring-sessions-after-a-restart)**.
- Auto approval "expect occasional false positives/negatives" and only sends
  Enter **[docs](https://github.com/kbwo/ccmanager/blob/main/docs/auto-approval.md)**.
- Requests: true multi-repo view (#174), squash merge (#223), filter by state
  (#310), per-worktree auto-approval toggle (#194), Droid CLI (#127)
  **[docs](https://github.com/kbwo/ccmanager/issues?q=sort%3Areactions-%2B1-desc)**.

---
## cmux (Manaflow)

### What it is

- Native macOS terminal app (Swift/AppKit, rendering via libghostty, reads
  `~/.config/ghostty/config`) with vertical tabs, notifications, an embedded
  browser, and a CLI/socket API for coding agents
  **[docs](https://github.com/manaflow-ai/cmux)**. Also a separate Rust "cmux
  TUI" multiplexer for macOS, Linux, Windows
  **[docs](https://cmux.com/docs/tui)**, an iOS companion (TestFlight beta)
  **[docs](https://cmux.com/docs/ios)**, paid cmux Cloud VMs (early access)
  **[docs](https://cmux.com/docs/cloud)**, and the hosted CodeRouter model
  router **[docs](https://cmux.com/docs/coderouter)**.
- Source available on GitHub; README says "Free and GPL-licensed", the API
  reports license "Other" with Manaflow, Inc. copyright
  **[source](https://github.com/manaflow-ai/cmux)**. Very active: v0.65.2.2 on
  2026-10-27, ~28.6k stars, nightly builds, Sparkle auto-update
  **[source](https://github.com/manaflow-ai/cmux/releases)**.
- Author's stated motivation: Claude Code's notification body is always
  "Claude is waiting for your input" with no context, and existing
  orchestrators were Electron/Tauri apps that "lock you into their workflow";
  "cmux is a primitive, not a solution"
  **[docs](https://github.com/manaflow-ai/cmux#why-cmux)**.

### Features

- **Hierarchy**: Window → Workspace (sidebar entry, `CMUX_WORKSPACE_ID`) →
  Pane (split) → Surface (tab in a pane, `CMUX_SURFACE_ID`) → Panel (terminal
  or browser) **[docs](https://cmux.com/docs/concepts)**.
- **Sidebar**: vertical workspace tabs showing git branch, linked PR
  status/number, cwd, listening ports, latest notification text; workspace
  groups with anchor workspace, pin, color, SF Symbol icon, collapse, and a
  `cmux workspace-group` CLI with idempotency keys
  **[docs](https://github.com/manaflow-ai/cmux#features)**,
  **[docs](https://cmux.com/docs/workspace-groups)**.
- **Status**: sidebar "Running" and "Needs input" indicators; Claude questions
  and plan approvals, Codex TUI approvals, and Cursor approvals show "Needs
  input" **[docs](https://cmux.com/docs/changelog)**.
- **Notifications**: blue ring on panes and lit tabs when an agent needs
  attention; notification panel (`⌘I`/`⌘⇧I`), jump to latest unread
  (`⌘⇧U`); lifecycle Received → Unread → Read → Cleared; desktop alerts
  suppressed when the window/workspace is focused; picks up OSC 9/99/777
  sequences and `cmux notify`; custom notification command with
  `CMUX_NOTIFICATION_TITLE/SUBTITLE/BODY`; JSON notification hooks that can
  rewrite or suppress each notification and see `agent.kind`,
  `agent.category` (`turn-complete`, `needs-permission`, `idle-reminder`),
  `pending`, `isSubagent` **[docs](https://cmux.com/docs/notifications)**.
- **Feed**: a stream of agent activity and approval banners with "Deny" and
  "Allow Once" buttons; replies routed to the owning agent; also on iOS
  **[docs](https://cmux.com/docs/changelog)**.
- **Usage**: optional status entry with model, context window used, and for
  Claude Code an estimated API cost including subagents
  **[docs](https://cmux.com/docs/configuration)**.
- **Task Manager** (`cmux top`): CPU and memory per window, workspace, pane,
  agent process, and browser webview **[docs](https://cmux.com/docs/task-manager)**.
- **Vault**: right-sidebar search over local Codex, Claude Code, OpenCode, and
  Pi transcripts; drag a session into the current workspace to resume
  **[docs](https://cmux.com/docs/vault)**.
- **TextBox**: rich prompt composer above a terminal
  **[docs](https://cmux.com/docs/textbox)**.
- **Dock**: right-sidebar terminal or browser controls from `.cmux/dock.json`
  (project, trusted before launch) or `~/.config/cmux/dock.json`
  **[docs](https://cmux.com/docs/dock)**.
- **In-app browser**: scriptable API ported from Vercel's agent-browser
  (accessibility snapshot, element refs, click, fill, eval JS); import cookies
  from 20+ browsers **[docs](https://github.com/manaflow-ai/cmux#features)**.
- **Markdown viewer** panel with live reload for plans
  **[docs](https://cmux.com/docs/skills)**.
- **Custom commands** in `cmux.json` launched from the command palette
  **[docs](https://github.com/manaflow-ai/cmux#features)**.
- **Session restore**: on relaunch restores layout, cwd, best-effort
  scrollback, and browser history; not live processes unless the opt-in
  `cmux local-tmux` or `cmux local-zellij` owner is used; agent sessions resume
  from native session IDs saved by hooks (Claude Code, Codex, Grok, OpenCode,
  Pi, Amp, Cursor CLI, Gemini, Rovo Dev, Copilot, CodeBuddy, Factory, Qoder);
  `cmux surface resume set --shell "..."` binds custom resume commands, with
  approval of signed prefixes and secret env keys stripped
  **[docs](https://github.com/manaflow-ai/cmux#session-restore)**.
- **Remote**: `cmux ssh user@host` workspaces with browser routed through the
  remote network and image drag-upload via scp; `cmux ssh-tmux` over
  EternalTerminal **[docs](https://github.com/manaflow-ai/cmux#features)**,
  **[docs](https://cmux.com/docs/changelog)**.
- **cmux Cloud** (paid): persistent Ubuntu 24.04 VMs with Claude Code, Codex,
  OpenCode, Pi preinstalled and hooks preconfigured; `cmux vm new/ls/open`,
  `cmux vm agent --agent claude --machine X --sync --wait -- "prompt"`,
  `vm run`, `vm exec`, `vm dev`, checkpoints and forks of machines, VNC
  desktop, image paste up to 20 MB; daemon restart loses processes
  **[docs](https://cmux.com/docs/cloud/workspaces)**.
- **CodeRouter** (hosted): pools ChatGPT, Claude Max/Pro setup tokens,
  Anthropic/OpenAI/OpenRouter keys, Bedrock, OpenCode Go; keeps a session on
  one account while healthy, fails over on rate limit before any output
  streams; team sharing; per-machine token usage and "API-equivalent value"
  **[docs](https://cmux.com/docs/coderouter)**.
- **iOS app**: pair with the Mac (off by default), view terminals, send input,
  forwarded agent notifications; direct connection over local network or
  Tailscale/WireGuard **[docs](https://cmux.com/docs/ios)**.
- Managed policies for organizations
  **[docs](https://cmux.com/docs/managed-policies)**.

### Harness integration

- `cmux hooks setup [codex | --agent opencode]` installs hooks into each agent
  found on PATH; Claude Code runs through a cmux `claude` wrapper when
  `automation.claudeCodeIntegration` is on; Codex is also wrapped (setting
  turns the wrapper off) **[docs](https://github.com/manaflow-ai/cmux#session-restore)**,
  **[docs](https://cmux.com/docs/configuration)**.
- `agents.launchers` in `cmux.json` declares external wrappers (e.g. a
  multi-account router) so restore resumes through them
  **[docs](https://cmux.com/docs/changelog)**.
- `cmux claude-teams` runs Claude Code agent teams with teammates as native
  splits: it puts a tmux shim first on `PATH`, sets synthetic `TMUX` and
  `TMUX_PANE`, sets `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1` and teammate mode
  `auto`, and translates `new-session`, `split-window`, `send-keys`,
  `capture-pane`, `select-pane`, `kill-pane`, `list-panes` into socket calls
  **[docs](https://cmux.com/docs/agent-integrations/claude-code-teams)**.
- Integration pages for oh-my-claudecode, oh-my-codex, oh-my-opencode,
  oh-my-pi **[docs](https://cmux.com/docs/agent-integrations/oh-my-codex)**.

### Observation and control

- State from hooks and the wrappers; OSC notification sequences; screen read
  via `read-screen` **[docs](https://cmux.com/docs/api)**,
  **[inferred]**.
- Control via `send`/`send-key` to surfaces, Feed Allow/Deny for permission
  requests, TextBox for composing prompts
  **[docs](https://cmux.com/docs/changelog)**.

### Extension points

- CLI and Unix socket (`/tmp/cmux.sock`, newline-delimited JSON
  `{"id","method","params"}`), every command on both; access modes Off, "cmux
  processes only" (default, ancestry check), `allowAll`
  **[docs](https://cmux.com/docs/api)**.
- cmux TUI JSON-lines control socket with typed bindings for TypeScript,
  Python, Go, Java, Rust **[docs](https://cmux.com/docs/tui)**.
- Skills (`npx skills add manaflow-ai/cmux`): cmux core, workspace, settings,
  customization, diagnostics, browser, markdown
  **[docs](https://cmux.com/docs/skills)**.
- Notification hooks, Dock controls, custom commands, custom JavaScript
  sidebars **[docs](https://cmux.com/docs/changelog)**.

### Known problems

- macOS only for the app; Linux (#330) and Windows (#1012) are top requests
  **[docs](https://github.com/manaflow-ai/cmux/issues/330)**.
- "Sidebar 'Running' and 'Needs Input' status indicators are flaky and
  unreliable" (#1027); "Claude notifications are flaky" (#2322)
  **[docs](https://github.com/manaflow-ai/cmux/issues/1027)**.
- Wrapper side effects: "Latest update forces Claude bypass permissions"
  (#3547); "cmux claude wrapper blocks new `claude agents` (Agent View)
  dashboard" (#4005); "Codex wrapper globally bypasses trust for all enabled
  hooks" (#8136) **[docs](https://github.com/manaflow-ai/cmux/issues/4005)**,
  **[docs](https://github.com/manaflow-ai/cmux/issues/8136)**.
- Memory leak to OOM (#4529) **[docs](https://github.com/manaflow-ai/cmux/issues/4529)**.
- Live processes do not survive quit without the tmux/zellij owner
  **[docs](https://github.com/manaflow-ai/cmux#session-restore)**.

---
## Uzi

### What it is

- Local Go CLI that starts many agents in parallel, each in a git worktree and
  tmux session, with a dev server per agent
  **[docs](https://github.com/devflowinc/uzi)**; whitepaper at
  `cdn.trieve.ai/uzi-whitepaper.pdf` **[docs](https://cdn.trieve.ai/uzi-whitepaper.pdf)**.
- Open source, MIT, by Devflow (Trieve). Dormant: last push 2025-06-04, ~580
  stars **[source](https://github.com/devflowinc/uzi)**.

### Features

- **Creating**: `uzi prompt --agents claude:3,codex:2 "task"` starts N agents
  per tool with the same prompt; `random` gives random agent names
  **[docs](https://github.com/devflowinc/uzi#uzi-prompt-alias-uzi-p)**.
- **Dev servers**: `uzi.yaml` `devCommand` with `$PORT` and `portRange`
  (e.g. `3000-3010`); each agent gets its own port and URL
  **[docs](https://github.com/devflowinc/uzi#uziyaml)**.
- **Watching**: `uzi ls` / `uzi ls -w` (refresh every second) shows `AGENT`,
  `MODEL`, `STATUS` (`ready` or `running`), `DIFF` (`+n/-n`), `ADDR`, `PROMPT`
  **[docs](https://github.com/devflowinc/uzi#uzi-ls-alias-uzi-l)**.
- **Messaging**: `uzi broadcast "..."` sends text to all agents; `uzi run "git
  status"` runs a command in every session (`--delete` removes the window)
  **[docs](https://github.com/devflowinc/uzi#uzi-run-alias-uzi-r)**.
- **Auto-confirm**: `uzi auto` presses Enter on trust prompts and continuation
  confirmations until Ctrl+C **[docs](https://github.com/devflowinc/uzi#uzi-auto-alias-uzi-a)**.
- **Merging**: `uzi checkpoint <agent> "msg"` commits in the agent's worktree
  and rebases onto the current branch
  **[docs](https://github.com/devflowinc/uzi#uzi-checkpoint-alias-uzi-c)**.
- **Stopping**: `uzi kill <agent>|all`; `uzi reset` deletes all Uzi data
  **[docs](https://github.com/devflowinc/uzi#uzi-kill-alias-uzi-k)**.

### Harness integration

- Any CLI named in `--agents`; no adapters or hooks
  **[docs](https://github.com/devflowinc/uzi#uzi-prompt-alias-uzi-p)**.
- Screen matching via `tmux capture-pane`: status is `running` if the pane
  contains "esc to interrupt" or "Thinking", else `ready`; `auto` sends Enter
  on "Do you trust the files in this folder?", "Press Enter to continue",
  "Continue? (Y/n)", "Do you want to proceed?", "Allow command", "Do you want
  to" **[source](https://github.com/devflowinc/uzi/blob/main/cmd/watch/auto.go)**.

### Observation and control

- tmux `capture-pane` and `send-keys`; diff stats from git
  **[source](https://github.com/devflowinc/uzi/blob/main/cmd/watch/auto.go)**.

### Extension points

- None beyond the CLI and `uzi.yaml` **[docs](https://github.com/devflowinc/uzi)**.

### Known problems

- `uzi auto` panics with "concurrent map writes" (#9); tmux renaming fails if
  pane index isn't 0 (#11); question about Claude Max login (#13)
  **[docs](https://github.com/devflowinc/uzi/issues/9)**.
- `uzi auto` approves every recognized prompt indiscriminately
  **[inferred]**.

---
## Toad (Batrachian)

### What it is

- Local terminal UI (Python, Textual) that is a client for many coding agents
  over the Agent Client Protocol (ACP), by Will McGugan; also runs as a web app
  via `toad serve` **[docs](https://github.com/batrachianai/toad)**.
- Open source, AGPL-3.0; ~3.5k stars; v0.6.20 on 2026-05-26, last push
  2026-05-26 **[source](https://github.com/batrachianai/toad)**. OpenHands
  announced a partnership naming Toad a "universal terminal interface"
  **[docs](https://www.openhands.dev/blog/20251218-openhands-toad-collaboration)**.
- Linux and macOS; Windows via WSL; recommends Ghostty, "much reduced
  experience" in macOS Terminal **[docs](https://github.com/batrachianai/toad#compatibility)**.

### Features

- **Agent "app store"**: find, install, and launch agents from the UI; agent
  definitions are TOML files (`src/toad/data/agents/*.toml`) for Claude Code,
  Codex (openai.com), Gemini CLI, Copilot, Cursor, Amp, Augment, Goose, Grok,
  Kimi, Kiro, OpenCode, OpenHands, OpenClaw, Mistral Vibe, Docker, LangChain
  Deep Agents, Hugging Face, Stakpak, VT Code, crowai
  **[docs](https://github.com/batrachianai/toad#what-is-toad)**,
  **[source](https://github.com/batrachianai/toad/tree/main/src/toad/data/agents)**.
- **Toad Shell**: a real shell (persistent cwd and env, full-color,
  interactive commands, tab completion) interleaved with agent prompts;
  claimed as the only terminal UI that does this
  **[docs](https://github.com/batrachianai/toad#what-is-toad)**.
- **Prompt editor**: Markdown with code-fence highlighting, mouse, cut/paste;
  `@` fuzzy file picker with tree mode
  **[docs](https://github.com/batrachianai/toad#what-is-toad)**.
- **Diffs**: side-by-side or unified, syntax highlighted
  **[docs](https://github.com/batrachianai/toad#what-is-toad)**.
- **Concurrent sessions**: multiple agents from different providers; `ctrl+s`
  shows the state of all agents on one screen
  **[docs](https://github.com/batrachianai/toad#what-is-toad)**.
- **Session resume**: `ctrl+r` picks a previous session (ACP `session/load`)
  **[docs](https://github.com/batrachianai/toad#what-is-toad)**,
  **[source](https://github.com/batrachianai/toad/blob/main/src/toad/acp/agent.py)**.
- **Settings UI** instead of JSON editing; context-usage percentage display
  **[docs](https://github.com/batrachianai/toad#what-is-toad)**,
  **[source](https://github.com/batrachianai/toad/blob/main/src/toad/acp/agent.py)**.
- CLI: `toad <project-dir>`, `toad -a open-hands` skips the agent picker
  **[docs](https://github.com/batrachianai/toad#toad-cli)**.
- No worktrees, merge, or PR features found **[inferred]**.

### Harness integration

- ACP only: each agent TOML has `protocol = "acp"` and a `run_command` (Claude
  Code runs `claude-agent-acp`), plus install actions (Claude:
  `curl -fsSL https://claude.ai/install.sh | bash && npm install -g
  @zed-industries/claude-code-acp`)
  **[source](https://github.com/batrachianai/toad/blob/main/src/toad/data/agents/claude.com.toml)**.

### Observation and control

- ACP client methods implemented: `session/update` (streamed messages, tool
  calls, plans), `session/request_permission` (permission dialog),
  `fs/read_text_file`, `fs/write_text_file`, `terminal/create`,
  `terminal/output`, `terminal/kill`, `terminal/release`,
  `terminal/wait_for_exit`; requests `initialize`, `session/new`,
  `session/load`, `session/prompt`, `session/set_mode`, `session/cancel`
  **[source](https://github.com/batrachianai/toad/blob/main/src/toad/acp/agent.py)**.

### Extension points

- New agents by adding ACP agent definitions ("Custom ACP server" was a
  request, #94) **[docs](https://github.com/batrachianai/toad/issues/94)**.

### Known problems

- Permission request shows duplicate keys (#83); crash typing `/` with
  claude-code-acp (#51); diff rendering corrupted with tabs (#124); requests
  for customizable key mappings (#95) and better stderr on ACP launch failures
  (#31) **[docs](https://github.com/batrachianai/toad/issues?q=sort%3Areactions-%2B1-desc)**.

---
## acpx (OpenClaw)

### What it is

- Headless CLI client for the Agent Client Protocol: "one structured interface
  for persistent sessions, one-shot runs, permissions, and machine-readable
  output across ACP-compatible coding agents"; tagline "Agents talking to
  agents, minus the terminal séance"
  **[docs](https://github.com/openclaw/acpx)**.
- Open source, MIT, npm `acpx`, Node 22.13+; pre-1.0 ("Treat its CLI and
  runtime interfaces as evolving"); v0.21.1 on 2026-10-27, ~3.4k stars; part of
  the OpenClaw project **[source](https://github.com/openclaw/acpx)**. Docs at
  acpx.sh **[docs](https://acpx.sh/)**.

### Features

- **Built-in agent registry** (friendly name → ACP adapter command): `pi`
  (`npx pi-acp`), `openclaw` (`openclaw acp`), `codex` (`npx -y
  @agentclientprotocol/codex-acp`), `claude` (`npx -y
  @agentclientprotocol/claude-agent-acp`), `gemini` (`gemini --acp`),
  `cursor` (`cursor-agent acp`), `copilot` (`copilot --acp --stdio`),
  `antigravity`, `devin` (`devin acp`), `droid` (`droid exec --output-format
  acp`), `fast-agent`, `fx`, `grok-build` (`grok agent stdio`), `iflow`,
  `junie` (`junie --acp=true`), `kilocode`, `kimi` (`kimi acp`), `kiro`
  (`kiro-cli-chat acp`), `mcode`, `mux` (`mux acp`), `octomind`, `opencode`
  (`npx -y opencode-ai acp`), `pool` (Poolside), `qoder`, `qwen`, `trae`,
  `zeroclaw`; unknown names run as raw commands; `--agent '<command>'` for any
  ACP server; default agent is `codex`
  **[docs](https://github.com/openclaw/acpx/blob/main/docs/agents.md)**.
- **Common command surface**: `acpx <agent> [prompt]`, `prompt`, `exec`
  (one-shot, no saved session), `cancel`, `set-mode`, `set <key> <value>`,
  `status`, `sessions [list|new|ensure|close|show|history|prune]`
  **[docs](https://github.com/openclaw/acpx/blob/main/docs/agents.md)**.
- **Sessions**: keyed by `(agentCommand, absoluteCwd, optional name)`; JSON
  record under `~/.acpx/` plus a "queue owner" process holding the live ACP
  connection; prompts auto-resume by walking up from cwd to the git root;
  exit code `4` if none found; named sessions (`-s backend`) run side by side;
  follow-up prompts queue while a turn runs; `sessions history` (last 20
  turn previews), `export`/`import`, soft `close`, `prune --older-than 30`;
  `sessions list` uses the agent's ACP `session/list` when advertised
  **[docs](https://github.com/openclaw/acpx/blob/main/docs/sessions.md)**.
- **Session control**: `cancel` sends ACP `session/cancel` through the queue
  owner (works without a TTY, "useful from scripts and other agents");
  `set-mode` → `session/set_mode` (modes adapter-defined, e.g. `plan`, `auto`);
  `set model gpt-5.4` → `session/set_config_option`; settings persisted and
  replayed on reconnect
  **[docs](https://github.com/openclaw/acpx/blob/main/docs/session-control.md)**.
- **Permissions**: `--approve-all`, `--approve-reads` (default), `--deny-all`;
  per-tool policy JSON with `autoApprove`, `autoDeny`, `escalate`,
  `defaultAction`; non-interactive escalations are denied for the turn and
  reported in `_meta.acpx.permissionEscalation` so an orchestrator can resume
  with a broader policy; config in `.acpxrc.json` / `~/.acpx/config.json`
  **[docs](https://github.com/openclaw/acpx/blob/main/docs/permissions.md)**.
- **Output**: text (default), `--format json` (NDJSON ACP events with thinking,
  tool calls, diffs, completion), `--format quiet` (final text only); exit codes
  documented **[docs](https://github.com/openclaw/acpx#automation-and-permissions)**.
- **Compare**: `acpx compare pi openclaw codex 'prompt'` runs the same one-shot
  prompt across agents serially in one checkout and reports `status` (`ok`,
  `cancelled`, `permission_denied`, `error`) and `wall_ms`
  **[docs](https://github.com/openclaw/acpx/blob/main/docs/compare.md)**.
- **Flows**: `acpx flow run` executes TypeScript workflows mixing ACP turns
  with deterministic actions, decisions, and checkpoints
  **[docs](https://github.com/openclaw/acpx#flows-and-embedding)**.
- Options forwarded at session creation: `--model`, `--allowed-tools`,
  `--max-turns`, `--system-prompt`; `--no-fs` and `--no-terminal` hide client
  capabilities; `--prompt-retries`
  **[docs](https://github.com/openclaw/acpx/blob/main/docs/compare.md)**.

### Harness integration

- Exclusively through ACP adapters (vendor-native `--acp` modes or
  `@agentclientprotocol/*` adapter packages); no screen reading
  **[docs](https://github.com/openclaw/acpx/blob/main/docs/agents.md)**.

### Observation and control

- Structured ACP event stream for state, tool calls, and diffs; cooperative
  cancel; mode/model switching; permission policy decides approvals
  **[docs](https://github.com/openclaw/acpx/blob/main/docs/permissions.md)**.

### Extension points

- CLI; library exports `acpx/runtime` and `acpx/flows`;
  `createSharedAcpRuntime()` shares one session owner between an app and CLI
  processes; embedded `onPermissionRequest` callback and per-session
  `sessionPermissions` **[docs](https://github.com/openclaw/acpx#flows-and-embedding)**,
  **[docs](https://github.com/openclaw/acpx/blob/main/docs/permissions.md)**.

### Known problems

- Adapter failures: Claude sessions ending with 0 turns (#29), Gemini adapter
  hangs (#33), "can no longer use Opus via acpx" (#374), `Invalid
  permissions.defaultMode: auto` (#238)
  **[docs](https://github.com/openclaw/acpx/issues/29)**.
- An acpx-spawned `claude` loads global channel plugins and "silently hijacks
  parent session's Telegram bot" (#361)
  **[docs](https://github.com/openclaw/acpx/issues/361)**.
- ~190 s delay to first text event for Codex sessions (#165); history empty for
  an active session (#157); requests for async mode with completion
  notification (#153) and JSON-Schema structured responses (#366)
  **[docs](https://github.com/openclaw/acpx/issues/165)**.
- Mode values not standardized across adapters
  **[docs](https://github.com/openclaw/acpx/blob/main/docs/session-control.md)**.

---
## workmux

### What it is

- Local Rust CLI pairing each git worktree with a tmux window (or WezTerm,
  kitty, Zellij backends, experimental), with a TUI dashboard and sidebar for
  agents; "Build on your terminal setup instead of yet another agentic GUI that
  won't exist next year" **[docs](https://github.com/raine/workmux)**.
- Open source, MIT; ~3.0k stars; v0.1.277 on 2026-10-23, very frequent
  releases **[source](https://github.com/raine/workmux)**. Docs at workmux.dev
  **[docs](https://workmux.dev/)**.

### Features

- **Creating**: `workmux add <branch>` creates worktree + tmux window with a
  configured pane layout (editor, shell, agent, watchers); `--prompt`,
  `--prompt-file`, `--prompt-editor` inject a prompt into the pane running the
  agent command; `--prompt-file-only` writes `.workmux/PROMPT-<branch>.md` for
  editor-embedded agents; `--background`; PR checkout by number, URL, or
  `owner:branch`; move uncommitted changes into a new worktree; `--wait` blocks
  until the agent closes its window
  **[docs](https://github.com/raine/workmux#workmux-add-branch-name)**.
- **Fan-out**: `-a claude -a gemini` (one worktree per agent), `-n 3`
  (count), `--foreach "env:staging,production;task:smoke,integration"` (zipped
  matrix), stdin lines (including JSON lines as variables), MiniJinja
  `--branch-template`, `--max-concurrent N` worker pool (agents must run
  `workmux remove --keep-branch` to free a slot)
  **[docs](https://github.com/raine/workmux#parallel-workflows--multi-worktree-generation)**.
- **Branch names** generated from the prompt by an LLM (`llm` CLI or the
  configured agent) **[docs](https://github.com/raine/workmux#automatic-branch-name-generation)**.
- **Setup**: post-create lifecycle hooks, copy or symlink files (`.env`,
  `node_modules`), `sync-files` to existing worktrees, port-allocation recipe
  for monorepos **[docs](https://github.com/raine/workmux#configuration-options)**.
- **Watching**: status icons in tmux window names (🤖 working, 💬 waiting, ✅
  done; waiting/done auto-clear on focus); "working" with no output for 10 s is
  marked interrupted; tmux bindings to jump to completed or waiting agents
  **[docs](https://github.com/raine/workmux#agent-status-tracking)**.
- **Dashboard** (`workmux dashboard`): all agents across tmux sessions, live
  preview, columns, sort modes, session/name/stale filters, send commands to
  agents; diff view with WIP (`git diff HEAD`) and review (`git diff
  main...HEAD`) modes; patch mode stages hunks like `git add -p`; `c` sends a
  commit command to the agent, `m` merges
  **[docs](https://github.com/raine/workmux#workmux-dashboard)**.
- **Sidebar** (`workmux sidebar`): persistent view of all agents
  **[docs](https://workmux.dev/guide/sidebar/)**.
- **Merging**: `workmux merge [--into <branch>] [--rebase|--squash] [--keep]
  [--notification]` merges and removes worktree, window, and local branch;
  `workmux rebase`; `workmux remove` (`--gone` removes worktrees whose remote
  branch was deleted after PR merge); `list --pr` shows PR status; `list
  --json` **[docs](https://github.com/raine/workmux#workmux-merge-branch-name)**.
- **Resuming**: `workmux open --continue` resumes the agent's last
  conversation; `workmux resurrect` reopens worktrees that had agents after a
  tmux or machine crash, from state files in `~/.local/state/workmux/agents/`
  **[docs](https://github.com/raine/workmux#workmux-resurrect)**.
- **Sandbox**: containers (Docker, Podman, Apple Container; ephemeral; optional
  domain allowlist) or Lima VMs (persistent; Nix/Devbox); host command
  proxying; git identity injection; credentials shared
  **[docs](https://github.com/raine/workmux#sandbox)**.
- **Delegation**: `/worktree` skill lets a coordinator agent on main spin up
  worktree agents **[docs](https://github.com/raine/workmux#delegating-tasks-with-worktree)**.
- Session mode (one tmux session per worktree), `workmux claude prune` (clean
  stale Claude Code entries), `reap-agents`, `workmux docs` for agents
  **[docs](https://github.com/raine/workmux#session-mode)**.

### Harness integration

- Prompt injection for `claude`, `codex`, `opencode`, `gemini`, `agy`,
  `kiro-cli`, `vibe`, `pi`, `omp`, `grok`
  **[docs](https://github.com/raine/workmux#ai-agent-integration)**.
- Status by hooks installed with `workmux setup`: Claude Code via a
  `workmux-status` plugin (`claude plugin install workmux-status`) or hooks in
  `~/.claude/settings.json`; Copilot CLI hook in `~/.copilot/hooks/`;
  Antigravity hooks in `~/.gemini/config/hooks.json` (`PreInvocation` →
  working, `Stop` → done); OpenCode plugin; Oh My Pi extension; no waiting
  state for Copilot, Pi, Antigravity; Kiro and Mistral Vibe pending upstream
  **[docs](https://github.com/raine/workmux#setup)**.

### Observation and control

- Hook-reported state written to state files; pane content via tmux for
  previews; control by sending keys/commands to tmux panes
  **[docs](https://github.com/raine/workmux#workmux-dashboard)**, **[inferred]**.

### Extension points

- CLI with `--json`; YAML config (global `~/.config/workmux/config.yaml`,
  project `.workmux.yaml`); lifecycle hooks; skills
  **[docs](https://github.com/raine/workmux#configuration)**.

### Known problems

- Requests: jj support (#59), multi-repo workspaces (#161), Herdr as a
  backend (#195); OpenCode v2 status plugin fails to load (#290); teardown by
  window name rather than stable id (#212)
  **[docs](https://github.com/raine/workmux/issues?q=sort%3Areactions-%2B1-desc)**.
- Worktree caveats documented: gitignored files, conflicts, pnpm/yarn, Rust
  target dirs, port conflicts **[docs](https://github.com/raine/workmux#git-worktree-caveats)**.

---
## container-use (Dagger)

### What it is

- Local MCP server and CLI (`container-use`, alias `cu`) that gives each agent
  a fresh container plus its own git branch; works with any MCP-capable agent
  (Claude Code, Cursor, Goose, VS Code, ...); powered by Dagger
  **[docs](https://github.com/dagger/container-use)**.
- Open source, Apache-2.0; "experimental" and "early development"; ~4.1k
  stars; last release v0.4.2 on 2025-08-19, last push 2026-09-21
  **[source](https://github.com/dagger/container-use)**.

### Features

- **Environments**: each = git branch + container + complete history of
  commands, file changes, and container state; named like `fancy-mallard`
  **[docs](https://container-use.com/environment-workflow)**.
- **Observing**: `container-use list`, `log <env>` (every command the agent
  ran), `diff <env>`; "see complete command history and logs of what agents
  actually did, not just what they claim"
  **[docs](https://container-use.com/environment-workflow)**,
  **[docs](https://github.com/dagger/container-use)**.
- **Intervening**: `container-use terminal <env>` drops into the live
  container; `checkout <env>` brings the branch locally
  **[docs](https://container-use.com/environment-workflow)**.
- **Deciding**: accept with `merge` or `apply`, iterate, or discard
  **[docs](https://container-use.com/environment-workflow)**.
- **Secrets** resolved inside the container so the model never sees values
  **[docs](https://container-use.com/environment-workflow)**.
- Agents return URLs for apps they run
  **[docs](https://github.com/dagger/container-use#try-it)**.

### Harness integration

- MCP only: `claude mcp add container-use -- container-use stdio`, plus an
  optional rules file appended to `CLAUDE.md` telling the agent to use
  environment tools **[docs](https://github.com/dagger/container-use#setup-with-your-agent)**.

### Observation and control

- Observes through its own tool-call log (agent file and shell operations go
  through container-use's MCP tools), not through the agent UI **[inferred]**.

### Extension points

- MCP tool surface and CLI **[docs](https://container-use.com/)**.

### Known problems

- Requests: jj (#194), devcontainer support (#279), create environments
  manually (#247), LSP in edit tools (#277); uses global git config instead of
  repo config (#256); exits 1 if Docker is not up (#125); engine version
  conflicts with `dagger` (#148)
  **[docs](https://github.com/dagger/container-use/issues?q=sort%3Areactions-%2B1-desc)**.

---

## Claude Code Agent Farm

### What it is

- Local Python 3.13 orchestration script that runs 20+ (up to 50) Claude Code
  sessions in tmux panes for bug fixing, best-practices sweeps, or coordinated
  development across 34 tech stacks
  **[docs](https://github.com/Dicklesworthstone/claude_code_agent_farm)**.
- Source available, "MIT + OpenAI/Anthropic Rider" license; ~920 stars; last
  push 2026-09-21 **[source](https://github.com/Dicklesworthstone/claude_code_agent_farm)**.

### Features

- Launches agents by typing a `cc` alias (`ENABLE_BACKGROUND_TASKS=1 claude
  --dangerously-skip-permissions`) into each tmux pane; the alias can point to
  OpenCode or Codex **[docs](https://github.com/Dicklesworthstone/claude_code_agent_farm#important-the-cc-alias)**.
- Monitoring dashboard in a tmux controller window: context warnings,
  heartbeat tracking, pane titles; auto-restart with adaptive idle timeout;
  agents clear their own context near limits; `Ctrl+R` broadcasts `/clear` to
  all agents; HTML run reports; git commits with diff summaries
  **[docs](https://github.com/Dicklesworthstone/claude_code_agent_farm#key-features)**.
- Coordination workflow implemented entirely in the prompt: agents maintain
  `/coordination/active_work_registry.json`, `completed_work_log.json`,
  `planned_work_queue.json`, and `agent_locks/{agent_id}_{timestamp}.lock`;
  stale locks after 2 hours **[docs](https://github.com/Dicklesworthstone/claude_code_agent_farm#multi-agent-coordination-system)**.
- Backs up and restores `~/.claude/settings.json` with rotation
  **[docs](https://github.com/Dicklesworthstone/claude_code_agent_farm#key-features)**.

### Harness integration, observation, control

- Screen scraping specific to Claude Code: readiness via the "Welcome to Claude
  Code!" banner, context-percentage parsing, usage-limit detection; other CLIs
  run but monitoring does not apply
  **[docs](https://github.com/Dicklesworthstone/claude_code_agent_farm#using-other-agent-clis-opencode-codex-etc)**.

### Known problems

- Requires `--dangerously-skip-permissions`; open question "Coordination rules
  at 20+ agents: what patterns survive?" (#4)
  **[docs](https://github.com/Dicklesworthstone/claude_code_agent_farm/issues/4)**.

---
# Part 2. Claude Code's own multi-agent features

## Claude Code (Anthropic): subagents, agent view, agent teams, cross-session messaging, worktrees

### What it is

- Claude Code is Anthropic's closed-source coding agent (CLI, Desktop app Code
  tab, IDE extensions, web). It has five built-in ways to run several tasks:
  subagents, agent view (`claude agents`, research preview), agent teams
  (experimental, off by default), dynamic workflows, and Projects (cloud, public
  beta on Pro, Max, Ultra); plus worktrees, cross-session messaging, and the
  `/batch` skill ("split one large change into 5 to 30 worktree-isolated
  subagents") **[docs](https://code.claude.com/docs/en/agents.md)**.
- "In every approach the workers are Claude sessions. To involve a different
  tool, expose it to Claude as an MCP server"
  **[docs](https://code.claude.com/docs/en/agents.md)**.

### Agent view and background sessions (`claude agents`, `claude --bg`)

- **Creating**: type a prompt in agent view's dispatch input (each prompt is a
  new session, named by a Haiku-class model); `claude --bg "prompt"` from the
  shell (prints `backgrounded · 7c5dcf5d · name` and the attach/logs/stop
  commands); `claude --agent code-reviewer --bg "..."`; `claude --resume <id>
  --bg`; `/background` (`/bg`) or `←` on an empty prompt moves the current
  session to the background; `/fork` copies the session into a new background
  session; `claude --bg --exec 'pytest -x'` or `!cmd` runs a PTY shell job as a
  row **[docs](https://code.claude.com/docs/en/agent-view.md)**.
- **Dispatch targets**: `@<repo>` mentions child repos, registered worktrees,
  or directories with sessions; `/model`, `/effort`, `/fast` in the dispatch
  input set defaults for later dispatches
  **[docs](https://code.claude.com/docs/en/agent-view.md)**.
- **Watching**: rows grouped as Pinned, Ready for review, Needs input, Working,
  Completed; states Working (animated), Needs input (yellow), Idle (dimmed),
  Completed (green), Failed (red), Stopped (grey); icon shapes `✻`/`✽` running,
  `∙` exited, `✢` sleeping `/loop`; one-line row summaries from a Haiku-class
  model, refreshed from output every ≤15 s and rewritten each turn end; terminal
  tab title `2 awaiting input · claude agents`; `← 2 agents` / `← 2 done` hint
  in foreground sessions; group by state or by directory (`Ctrl+S`); filters
  `a:`, `s:`, `n:`, `o:` (e.g. `s:blocked a:reviewer`)
  **[docs](https://code.claude.com/docs/en/agent-view.md)**.
- **PR status**: when a session opens or touches a PR (detected from `gh`
  output, `gh pr view` after push, retrying after up to five later
  git/gh/glab/curl commands) the row shows `#1234` (or `!1234` for GitLab
  MRs), colored yellow (checks/review pending or failed), green (passed),
  purple (merged), grey (draft/closed)
  **[docs](https://code.claude.com/docs/en/agent-view.md#pull-request-status)**.
- **Peek and reply**: `Space` opens a peek panel with the question, result, or
  status; type a reply (queued if working; `/model`, `/effort`, `/rename`,
  `/usage` run immediately; `/stop` stops); numbered choices for questions;
  permission prompts cannot be answered from peek, only by attaching
  **[docs](https://code.claude.com/docs/en/agent-view.md#peek-and-reply)**.
- **Attach/detach**: `Enter`/`→` attaches (always fullscreen; Claude posts a
  recap of what happened while away); `←`, `Ctrl+Z`, `/exit`, double `Ctrl+C`
  detach without stopping **[docs](https://code.claude.com/docs/en/agent-view.md#attach-to-a-session)**.
- **Organizing**: `Ctrl+T` pin (keeps process alive), `Shift+↑/↓` reorder,
  `Ctrl+R` rename, `Ctrl+X` stop then `Ctrl+X` delete; `/resume` brings a past
  session back as a row **[docs](https://code.claude.com/docs/en/agent-view.md#organize-the-list)**.
- **Isolation**: a dispatched session moves itself into a worktree under
  `.claude/worktrees/` before its first edit (unless already in a worktree,
  outside git without a `WorktreeCreate` hook, or `worktree.bgIsolation:
  "none"`); it then commits without asking, pushes if a remote exists, may open
  a draft PR, never pushes to main/master, force-pushes, or merges; ends with a
  report of where the work is
  **[docs](https://code.claude.com/docs/en/agent-view.md#how-file-edits-are-isolated)**.
- **Deleting**: agent view deletes the worktree including uncommitted changes;
  `claude rm` keeps it if dirty; unpushed commits block deletion until
  `--discard-unpushed`; failed removal offers `--force-remove-worktree`
  **[docs](https://code.claude.com/docs/en/agent-view.md#what-deleting-a-session-removes)**.
- **Shell commands**: `claude agents [--cwd] [--json [--all]]`, `claude attach
  <id|name>`, `claude logs <id|name>`, `claude stop <id>` (`kill`), `claude
  respawn <id>|--all`, `claude rm <id>`, `claude daemon status|logs|stop --any
  [--keep-workers]` **[docs](https://code.claude.com/docs/en/agent-view.md#manage-sessions-from-the-shell)**.
- **Script interface**: `claude agents --json` entries have `cwd`, `kind`
  (`interactive`/`background`), `startedAt`, `id`, `state` (`working`,
  `blocked`, `done`, `failed`, `stopped`), `pid`, `status` (`busy`, `waiting`,
  `idle`), `waitingFor` (`permission prompt`, `input needed`, `sandbox
  request`, `worker request`, `dialog open`), `sessionId`, `name`; files under
  `~/.claude/jobs/<id>/` are "not a stable interface"
  **[docs](https://code.claude.com/docs/en/agent-view.md#read-session-state-from-a-script)**.
- **Hosting**: a supervisor process runs each background session as its own
  Claude Code process; it stops finished/waiting unattached sessions after
  about an hour (resumed on attach), restarts crashed ones, migrates idle ones
  after auto-update; sessions survive sleep but not shutdown; if the supervisor
  dies, sessions stop within a minute unless a new one starts; state in
  `~/.claude/daemon.log`, `~/.claude/daemon/roster.json`,
  `~/.claude/jobs/<id>/state.json`, scratch `$CLAUDE_JOB_DIR/tmp`
  **[docs](https://code.claude.com/docs/en/agent-view.md#how-background-sessions-are-hosted)**.
- **Notifications** through the configured terminal notification channel when a
  background session needs input, finishes, or fails
  **[docs](https://code.claude.com/docs/en/agent-view.md#read-session-state)**.
- **Limits**: research preview; each session uses quota independently ("ten
  agents ... uses quota roughly ten times as fast"); local only; can be
  disabled with `disableAgentView` / `CLAUDE_CODE_DISABLE_AGENT_VIEW`
  **[docs](https://code.claude.com/docs/en/agent-view.md#limitations)**.

### Agent teams

- Enabled with `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`; requires an
  interactive session (not `-p` or the Agent SDK); one session is lead, others
  are teammates with their own context windows
  **[docs](https://code.claude.com/docs/en/agent-teams.md)**.
- **Creating**: ask in natural language ("Spawn three teammates..."); a teammate
  launches whenever Claude calls the Agent tool with a `name`; teammates can
  use subagent definitions (`tools`, `model`, `disallowedTools`, `effort`,
  body, `mcpServers` apply with mode-specific rules)
  **[docs](https://code.claude.com/docs/en/agent-teams.md#use-subagent-definitions-for-teammates)**.
- **Display modes**: `in-process` (default; agent panel below the prompt,
  arrows select, Enter views and messages, Escape interrupts, `x` stops,
  `Ctrl+T` task list) or split panes via tmux or iTerm2 `it2` (`teammateMode`
  `auto`, `tmux`, `iterm2`; `--teammate-mode`); split panes unsupported in VS
  Code terminal, Windows Terminal, Ghostty
  **[docs](https://code.claude.com/docs/en/agent-teams.md#choose-a-display-mode)**.
- **Coordination**: shared task list (pending, in progress, completed;
  dependencies shown as blocked; lead assigns or teammates self-claim; file
  locking) at `~/.claude/tasks/{team-name}/`; mailbox JSON files at
  `~/.claude/teams/{team-name}/inboxes/{agent-name}.json`; team config at
  `~/.claude/teams/{team-name}/config.json` with `members`; automatic message
  delivery; idle notifications with the teammate's final answer; plan approval
  requests; shutdown requests the teammate can approve or reject
  **[docs](https://code.claude.com/docs/en/agent-teams.md#architecture)**.
- **Permissions**: teammates inherit the lead's mode; teammate permission
  prompts appear in the lead session; messages from other agents are marked as
  not from the user and cannot approve prompts; auto-mode classifier reviews
  inter-agent messages **[docs](https://code.claude.com/docs/en/agent-teams.md#permissions)**.
- **Hooks**: `TeammateIdle`, `TaskCreated`, `TaskCompleted` (exit 2 to block
  and send feedback) **[docs](https://code.claude.com/docs/en/agent-teams.md#enforce-quality-gates-with-hooks)**.
- **Limits**: no resumption of in-process teammates; task status can lag;
  slow shutdown; one team per session; no nested teams; lead is fixed; no
  per-teammate permission mode at spawn; no worktree isolation for teammates
  ("partition the work") **[docs](https://code.claude.com/docs/en/agent-teams.md#limitations)**,
  **[docs](https://code.claude.com/docs/en/agents.md)**.

### Cross-session messaging

- Claude lists sessions with `ListAgents` and sends with `SendMessage`; user
  can `@name` a session; `/list-agents` shows subagents, teammates, other local
  sessions, cloud sessions, and Remote Control sessions on other machines
  **[docs](https://code.claude.com/docs/en/cross-session-messaging.md)**.
- **Delivery**: read between tool calls during a turn, or starts a new turn
  when idle; arrives as plain text with a dim preview line
  `› Message from @api-worker: ...`; local transport is a per-session Unix
  socket (named pipe on Windows), cross-machine and cloud go through Anthropic
  servers via Remote Control **[docs](https://code.claude.com/docs/en/cross-session-messaging.md#message-delivery)**.
- **Idle notice**: `SendMessage` `notify_when_idle` subscribes to one notice
  when another local session next goes idle or exits; expires after 12 hours
  **[docs](https://code.claude.com/docs/en/cross-session-messaging.md#get-a-notice-when-another-session-goes-idle)**.
- **Safety**: incoming messages cannot approve prompts, change configuration,
  or run slash commands; `crossSessionInbound` `accept`/`hold`/`refuse`;
  default holds messages when permission-bypass classes differ; approval dialog
  expires after `dialogExpiry` (5 min); at most 100 held messages;
  `isolatePeerMachines` requires approval for cross-machine sends
  **[docs](https://code.claude.com/docs/en/cross-session-messaging.md#control-inbound-messages)**.
- **Socket for scripts and hooks**: path in `CLAUDE_CODE_MESSAGING_SOCKET`,
  token in `CLAUDE_CODE_MESSAGING_TOKEN` (auth line `{"type":"auth","token":...}`
  required on Windows); own-child messages delivered; connection closed after
  30 s without a complete line; `claude -p` workers bind a socket too (not in
  bare mode) **[docs](https://code.claude.com/docs/en/cross-session-messaging.md#the-sessions-inbox-socket)**.
- **Limits**: plain text only; ~1 million character cap; burst refusal; loop
  throttling (at most 50 queued); container and host cannot reach each other;
  cloud sessions cannot reply to other machines
  **[docs](https://code.claude.com/docs/en/cross-session-messaging.md#limitations)**.

### Worktrees

- `claude --worktree <name>` (`-w`) creates `.claude/worktrees/<name>/` on
  branch `worktree-<name>`; generated names like `bright-running-fox`;
  `--worktree "#1234"` or a PR/MR URL branches from a pull request;
  `worktree.baseRef` `fresh` (default branch on remote) or `head`;
  `.worktreeinclude` copies gitignored files; `EnterWorktree` tool;
  `WorktreeCreate`/`WorktreeRemove` hooks for non-git VCS
  **[docs](https://code.claude.com/docs/en/worktrees.md)**.
- Exit cleanup: clean unnamed worktree removed with branch; dirty ones prompt
  keep/remove; `-p` runs never clean up
  **[docs](https://code.claude.com/docs/en/worktrees.md#clean-up-worktrees)**.
- Isolation enforcement blocks edits, command cwd, and git redirects into the
  main checkout, but not `cp` or shell redirects
  **[docs](https://code.claude.com/docs/en/worktrees.md#how-claude-code-enforces-isolation)**.

### Other surfaces relevant to multi-agent use

- **Subagents**: delegated workers inside one session with their own context;
  frontmatter `isolation: worktree`, `background: true`; `/tasks` lists
  background items; forked subagents via `/subtask`
  **[docs](https://code.claude.com/docs/en/agents.md)**.
- **Dynamic workflows**: a script Claude writes that runs many subagents and
  cross-checks results; `/workflows` lists runs
  **[docs](https://code.claude.com/docs/en/workflows.md)**.
- **Channels** (research preview): MCP servers that push events into a running
  session (Telegram, Discord, iMessage plugins; two-way replies)
  **[docs](https://code.claude.com/docs/en/channels.md)**.
- **Remote Control**: drive a local session from claude.ai/code or the mobile
  app; conversation synced across devices
  **[docs](https://code.claude.com/docs/en/remote-control.md)**.
- **Desktop app Code tab**: parallel sessions in a sidebar, split view, per-
  session worktree toggle (custom location, branch prefix), archive (auto-
  archive after PR merge/close), OS notification when a session finishes, PR
  status monitoring, diff view, integrated terminal and file editor, "Work
  across sessions" (Claude lists, reads, messages, renames, archives other
  Desktop sessions), suggested-task cards with Start / Do it here / Start all,
  move a session to Cloud **[docs](https://code.claude.com/docs/en/desktop.md)**.
- **Scheduling**: `/loop` and cron tools in-session; Desktop scheduled tasks;
  cloud scheduled tasks (formerly routines)
  **[docs](https://code.claude.com/docs/en/scheduled-tasks.md)**,
  **[docs](https://code.claude.com/docs/en/routines.md)**.

### Harness integration, observation, control (for external tools)

- **Hooks** (shell command, HTTP, prompt, or agent handlers in settings files):
  `SessionStart`, `Setup`, `InstructionsLoaded`, `UserPromptSubmit`,
  `UserPromptExpansion`, `MessageDisplay`, `PreToolUse`, `PermissionRequest`,
  `PostToolUse`, `PostToolUseFailure`, `PostToolBatch`, `PermissionDenied`,
  `Notification`, `SubagentStart`, `SubagentStop`, `TaskCreated`,
  `TaskCompleted`, `Stop`, `StopFailure`, `TeammateIdle`, `ConfigChange`,
  `CwdChanged`, `DirectoryAdded`, `FileChanged`, `WorktreeCreate`,
  `WorktreeRemove`, `PreCompact`, `PostCompact`, `PreModelSwitch`,
  `PostModelSwitch`, `SessionEnd`, `Elicitation`, `ElicitationResult`
  **[docs](https://code.claude.com/docs/en/hooks.md)**. These are what Herdr,
  agent-deck, and cmux install into `~/.claude/settings.json` **[inferred]**.
- **Headless**: `claude -p` with `--output-format text|json|stream-json` and
  `--input-format stream-json`; JSON output includes `total_cost_usd` and
  per-model cost **[docs](https://code.claude.com/docs/en/headless.md)**.
- **Status line** command receives JSON including `rate_limits` (used by
  agent-deck for quota) **[docs](https://github.com/asheshgoplani/agent-deck#provider-quota-agent-deck-usage)**.
- **Mods**: JavaScript/TypeScript plugins that run inside Claude Code, draw
  panes and bands, intercept tool calls and requests, add commands, call a
  model, message other sessions; `/diff` is a built-in mod
  **[docs](https://code.claude.com/docs/en/plugins/mods/overview.md)**.
- **Transcripts** under `~/.claude/projects/<path>/` (read by agent-deck,
  cmux Vault, CCManager copy) **[docs](https://code.claude.com/docs/en/sessions.md)**,
  **[docs](https://github.com/kbwo/ccmanager#session-data-copying)**.
- **ACP**: via the `@agentclientprotocol/claude-agent-acp` adapter (Claude
  Agent SDK), not built in **[docs](https://github.com/agentclientprotocol/claude-agent-acp)**.

### Known problems

- Third-party managers conflict with agent teams' tmux pane spawning
  (CCManager forces `--teammate-mode in-process`; cmux uses a tmux shim)
  **[docs](https://github.com/kbwo/ccmanager#claude-code-teammate-mode)**,
  **[docs](https://cmux.com/docs/agent-integrations/claude-code-teams)**.
- cmux's `claude` wrapper blocked the `claude agents` dashboard (cmux #4005)
  **[docs](https://github.com/manaflow-ai/cmux/issues/4005)**.
- Documented limits listed above (team resumption, task lag, quota use)
  **[docs](https://code.claude.com/docs/en/agent-teams.md#limitations)**.

---
# Part 3. Local desktop and web apps

## Vibe Kanban (BloopAI)

### What it is

- Local web app launched with `npx vibe-kanban` (Rust backend + web UI), with
  a hosted Cloud tier for team kanban; plans work as kanban issues and runs
  agents in workspaces **[docs](https://github.com/BloopAI/vibe-kanban)**.
- Open source, Apache-2.0; ~28.5k stars; last push 2026-10-29
  **[source](https://github.com/BloopAI/vibe-kanban)**.
- **Sunsetting**: Bloop, the company, shut down; the project continues "open
  source and community maintained"; remote services (kanban issues, comments,
  projects, organisations) remain 30 days, then "a fully local architecture";
  a data export feature was added; launched June 2025, "the first to ship
  multi-agent support, diff commenting, live preview, click-to-edit, remote
  access" (company claim); reason: "the vast majority are free users and we
  couldn't find a business model" **[docs](https://www.vibekanban.com/blog/shutdown)**.

### Features

- **Planning**: kanban issues (create, prioritise, assign) on a board, list
  view, filtering, custom columns; team organisations, members, projects in
  Cloud (being removed) **[docs](https://vibekanban.com/docs/llms.txt)**.
- **Workspaces**: an isolated environment per task with a branch (e.g.
  `vk/abc123-task`), terminal, dev server; can include several repositories;
  multiple sessions per workspace that share files but not conversation
  (start a new session when the context gauge turns orange/red, or to use a
  different agent) **[docs](https://vibekanban.com/docs/workspaces/index.md)**,
  **[docs](https://vibekanban.com/docs/workspaces/sessions.md)**.
- **Interface**: four-panel layout (workspace sidebar, conversation, changes
  panel, details sidebar), command bar `Cmd/Ctrl+K`, slash commands passed to
  the agent, integrated terminal
  **[docs](https://vibekanban.com/docs/workspaces/index.md)**.
- **Review**: changes panel with file tree, syntax-highlighted diffs, inline
  comments sent as feedback to the agent
  **[docs](https://vibekanban.com/docs/workspaces/changes.md)**.
- **Preview**: built-in browser with devtools, inspect mode (click-to-edit),
  device emulation **[docs](https://github.com/BloopAI/vibe-kanban#overview)**.
- **Git**: create PRs with AI-generated descriptions (GitHub, Azure Repos),
  rebase onto the target branch, "Merge" pulls target into the working branch,
  repository status in the sidebar
  **[docs](https://vibekanban.com/docs/workspaces/git-operations.md)**.
- **Agent profiles**: reusable variants per agent: planning mode, model,
  `dangerously_skip_permissions`, Claude Code Router, Codex sandbox
  (`read-only`, `workspace-write`, `danger-full-access`) and approval levels
  (`untrusted`, `on-failure`, `on-request`, `never`)
  **[docs](https://vibekanban.com/docs/settings/agent-configurations.md)**.
- **Tags**: reusable prompt snippets inserted with `@`
  **[docs](https://vibekanban.com/docs/settings/creating-task-tags.md)**.
- **Remote access**: pair a phone or another browser with the local host via a
  pairing code through cloud.vibekanban.com
  **[docs](https://vibekanban.com/docs/remote-access.md)**.
- MCP server configuration for agents; VS Code/Cursor/Windsurf extension
  **[docs](https://vibekanban.com/docs/llms.txt)**.
- Worktree cleanup of orphaned and expired workspaces
  (`DISABLE_WORKTREE_CLEANUP`) **[docs](https://github.com/BloopAI/vibe-kanban#environment-variables)**.

### Harness integration

- One Rust executor per agent in `crates/executors`: `claude.rs`, `codex.rs`,
  `amp.rs`, `copilot.rs`, `cursor.rs`, `droid.rs`, `gemini.rs`, `opencode.rs`,
  `qwen.rs`, plus a generic `acp` harness
  **[source](https://github.com/BloopAI/vibe-kanban/tree/main/crates/executors/src/executors)**.
- Claude Code runs with `--verbose --output-format=stream-json
  --input-format=stream-json` and `--permission-prompt-tool=stdio`; Vibe Kanban
  registers SDK-style hook callbacks (`PreToolUse` → `tool_approval` or
  auto-approve; a `Stop` git-check callback that reminds the agent to commit)
  over the control protocol
  **[source](https://github.com/BloopAI/vibe-kanban/blob/main/crates/executors/src/executors/claude.rs)**.
- Codex through the Codex app-server JSON-RPC protocol
  (`codex_app_server_protocol`) **[source](https://github.com/BloopAI/vibe-kanban/blob/main/crates/executors/src/executors/codex.rs)**.
- Gemini CLI through ACP (`--experimental-acp`, `AcpAgentHarness`)
  **[source](https://github.com/BloopAI/vibe-kanban/blob/main/crates/executors/src/executors/gemini.rs)**.
- OpenCode by spawning an OpenCode server with a generated password and using
  its HTTP SDK **[source](https://github.com/BloopAI/vibe-kanban/blob/main/crates/executors/src/executors/opencode.rs)**.

### Observation and control

- Structured event streams normalized into a chat log; approvals surfaced in
  the UI via permission-prompt tool / app-server approval requests; follow-up
  messages sent over the same protocol
  **[source](https://github.com/BloopAI/vibe-kanban/blob/main/crates/executors/src/executors/claude.rs)**, **[inferred]**.

### Extension points

- Local MCP server (`npx -y vibe-kanban@latest --mcp`) with tools for
  context (`get_context`), organisations, projects, issues, workspaces, and
  task execution, usable from Claude Desktop, Raycast, or agents inside Vibe
  Kanban **[docs](https://vibekanban.com/docs/integrations/vibe-kanban-mcp-server.md)**.
- Self-hostable Cloud via Docker Compose
  **[docs](https://vibekanban.com/docs/self-hosting/deploy-docker.md)**.

### Known problems

- UI churn: most-upvoted issues are "Bring back old UI" (#2687), "please keep
  the Kanban board" (#2509), "Personal Kanban View with Legacy UX" (#2730),
  and confusing cloud migration in v0.1.13 (#2812); "Why do I need to log in
  to use features such as Kanban?" (#2952)
  **[docs](https://github.com/BloopAI/vibe-kanban/issues/2687)**.
- "Opencode server exited before printing listening URL" (#2077); "Timeout
  waiting for shape data" (#2838); requests for GitLab (#1697), Kiro (#1708),
  configurable worktree cleanup (#765), GitHub issue import (#1677)
  **[docs](https://github.com/BloopAI/vibe-kanban/issues?q=sort%3Areactions-%2B1-desc)**.

---
## Conductor (Melty Labs)

### What it is

- Closed-source macOS desktop app that runs Claude Code, Codex, Cursor, and
  OpenCode in parallel, one isolated workspace (git worktree + branch) per
  task, plus Conductor Cloud sandboxes, multiplayer, an HTTP API, and an iOS
  app **[docs](https://www.conductor.build/docs)**.
- Pricing: Free (local workspaces, bring your own subscriptions), Pro $50/mo
  (Cloud workspace hours, collaboration with up to 5 Pro users, API, mobile
  app), Teams $60/mo/user, Enterprise; "Trusted by 100k+ builders"
  **[docs](https://www.conductor.build/pricing)**. Very active: v0.92.0 on
  2026-10-23 **[docs](https://www.conductor.build/changelog)**.

### Features

- **Workspaces**: one per "shippable unit"; named after cities (stable
  directory names; `cmd+K → Passport` lists visited cities); sidebar shows PR
  title or branch; created from a branch, PR, GitHub issue, or Linear issue
  (`⌘⇧N`) **[docs](https://www.conductor.build/docs/faq)**,
  **[docs](https://www.conductor.build/docs/concepts/workflow)**.
- **Parallel agents**: multiple workspaces for independent work, or multiple
  chat tabs (any mix of Claude Code, Codex, Cursor, OpenCode) in one workspace
  sharing a branch, e.g. implement + review + fix tests
  **[docs](https://www.conductor.build/docs/concepts/parallel-agents)**.
- **Agent modes**: Plan Mode and Fast Mode (Claude Code, Codex), reasoning
  level, Codex personalities, Codex goals (`/goal`, goal bar with token usage,
  pause/resume), checkpoints (turn-by-turn changes with revert), skills
  **[docs](https://www.conductor.build/docs/concepts/agent-modes)**.
- **Review**: Diff Viewer (`⌘⇧D`) with unified view, commit filtering, inline
  comments that become composer attachments sent to the agent, GitHub review
  comments shown; "Review" action for an agent review
  **[docs](https://www.conductor.build/docs/reference/diff-viewer)**.
- **Checks tab**: git status, PR metadata, CI/status checks, deployments,
  GitHub comments and review threads, todos; may block merge on unresolved
  todos or failed checks **[docs](https://www.conductor.build/docs/reference/checks)**.
- **PR and merge**: create PR (`⌘⇧P`) with drafted description; respond to
  review comments; fix failing checks; merge; archive (restorable with chat
  history from the History pane) **[docs](https://www.conductor.build/docs/concepts/workflow)**.
- **Project settings**: `.conductor/settings.toml` (shared, committed) and
  `.conductor/settings.local.toml`, with `[scripts]` `setup`, `run` actions on
  the Run button using `$CONDUCTOR_PORT`, `run_mode`, `archive`; copy untracked
  files; a bundled Conductor skill writes the config
  **[docs](https://www.conductor.build/docs/configure-your-project)**.
- **Cloud** (Pro+): isolated Linux sandboxes from an org-wide "Cloud Computer"
  build; agents keep running after the Mac closes; SSH, file sync, forwarded
  ports, preview URLs that wake sleeping workspaces
  **[docs](https://www.conductor.build/docs/cloud)**,
  **[docs](https://www.conductor.build/changelog)**.
- **Multiplayer**: Home view of org workspaces filtered by person/project,
  presence dots, live shared chats with typing indicators, follow, "Reassign
  to", copy link (`⌘⇧C`) **[docs](https://www.conductor.build/docs/cloud/collaboration)**.
- **Routines**: saved prompts run in a fresh workspace on webhook, cron, or
  GitHub triggers **[docs](https://www.conductor.build/docs/api)**.
- **Remote control** from the iOS app: browse and create workspaces, read
  chats, send messages, pick models, stop agents on the Mac
  **[docs](https://www.conductor.build/changelog)**.
- Cost per cloud workspace in the list; context usage indicator; Artifacts tab;
  MCP status indicator; Big Terminal Mode
  **[docs](https://www.conductor.build/changelog)**.
- Claude Code agent teams via `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1` in
  Settings → Environment **[docs](https://www.conductor.build/docs/faq)**.

### Harness integration

- Bundles its own Claude Code, Codex, and OpenCode executables in
  `~/Library/Application Support/com.conductor.app/bin` ("Do not update or
  modify them"); system installs can be configured instead; Cursor via the
  Cursor API with `CURSOR_API_KEY` **[docs](https://www.conductor.build/docs/faq)**,
  **[docs](https://www.conductor.build/docs/reference/harnesses)**.
- Uses the user's existing auth tokens; warns that `ANTHROPIC_API_KEY` in the
  environment switches Claude Code to API billing
  **[docs](https://www.conductor.build/docs/reference/harnesses/claude-code)**.
- Injects its own system prompts explaining the workspace and UI actions
  ("Create PR") **[docs](https://www.conductor.build/docs/faq)**.
- Protocol used to drive the bundled harnesses is not documented
  **[inferred]**.

### Observation and control

- Chat UI per session; session status `idle`, `working`, `errored`; cancel
  stops the current turn and drops queued messages
  **[docs](https://www.conductor.build/docs/api)**.
- Agents run with the user's permissions, unsandboxed locally
  **[docs](https://www.conductor.build/docs/faq)**.

### Extension points

- HTTP API (beta) at `https://api.conductor.build/v0` for cloud workspaces:
  projects, workspaces (create, rename, status `initializing`/`ready`/
  `sleeping`/`archived`, archive, unarchive, sleep), sessions (create, send
  message, read transcript with `after=<id>`, status, cancel, attachments),
  `POST /v0/sql` read-only SQL over `session_transcripts_view`, routines with
  webhook triggers; OpenAPI spec; `conductor` CLI; MCP server; a suggested
  prompt makes Claude Code/Codex/OpenClaw a "Conductor manager"
  **[docs](https://www.conductor.build/docs/api)**.

### Known problems

- No local sandboxing; macOS-only; protected-folder permission pop-ups appear
  as "Conductor" **[docs](https://www.conductor.build/docs/faq)**.
- Bundled harness versions must not be updated by the user
  **[docs](https://www.conductor.build/docs/faq)**.
- API in beta; "Request and response shapes may change"
  **[docs](https://www.conductor.build/docs/api)**.

---
## Crystal → Nimbalyst (Stravu)

### What it is

- **Crystal**: open-source (MIT) Electron desktop app for running multiple
  Claude Code and Codex sessions in parallel git worktrees; ~3.1k stars;
  **deprecated February 2026** and replaced by Nimbalyst ("You can continue
  using Crystal, but we recommend moving to Nimbalyst")
  **[docs](https://github.com/stravu/crystal)**.
- **Nimbalyst**: open-source (MIT) Electron desktop app for macOS, Windows,
  Linux with an iOS companion; "visual workspace" for Codex, Claude Code,
  OpenCode, and others; ~2.0k stars; v0.81.7 on 2026-10-16
  **[docs](https://github.com/nimbalyst/nimbalyst)**,
  **[source](https://github.com/nimbalyst/nimbalyst)**.

### Features (Crystal, before deprecation)

- Create one or more sessions from a prompt, each an isolated worktree; each
  iteration makes a commit so you can go back; run scripts per project; diff
  viewer with manual edits; continue conversations; "Rebase from main";
  "Squash and rebase to main" with a new message; git commands previewed in
  tooltips **[docs](https://github.com/stravu/crystal/blob/5f36368addeede63872280adb28e932d5ad142e5/README.md)**,
  **[inferred]** (README at a January 2026 commit).

### Features (Nimbalyst)

- **Parallel agent sessions**, each in its own git worktree; Agent Manager
  view; search and resume sessions; session kanban board; sessions linked to
  the files they touched **[docs](https://github.com/nimbalyst/nimbalyst#features)**.
- **Visual editors** shared by user and agent: Markdown (Lexical), mockups with
  annotations, Mermaid, Excalidraw, CSV (RevoGrid), data models, Monaco code;
  agent edits stream into open editors; red/green diffs accepted or rejected
  per change **[docs](https://github.com/nimbalyst/nimbalyst#what-you-get)**.
- **Task tracking**: plans, bugs, features, todos that agents read and update
  **[docs](https://github.com/nimbalyst/nimbalyst#features)**.
- **Git**: branch and worktree management, staging, AI-drafted commits,
  embedded Ghostty terminal **[docs](https://github.com/nimbalyst/nimbalyst#features)**.
- **MCP**: client for any MCP server, tool results rendered as widgets
  **[docs](https://github.com/nimbalyst/nimbalyst#what-you-get)**.
- **iOS app**: session dashboard showing which agents need you, reply by text
  or voice, swipe through diffs, queue next tasks, push notifications
  **[docs](https://github.com/nimbalyst/nimbalyst#features)**.
- Plain files on disk; release channels (stable/alpha); anonymous PostHog
  analytics with opt-out **[docs](https://github.com/nimbalyst/nimbalyst#telemetry)**.

### Harness integration

- Provider classes per agent in `packages/runtime/src/ai/server/providers/`:
  `ClaudeCodeProvider` / `ClaudeCodeCliProvider`, `OpenAICodexProvider` and
  `OpenAICodexACPProvider`, `OpenCodeProvider`, `CopilotCLIProvider`,
  `CursorAgentProvider`, `GeminiAntigravityProvider`, `GrokBuildProvider`,
  `HeadlessCliAgentProvider`, `ExtensionAgentProvider`, plus a
  `TeammateManager` **[source](https://github.com/nimbalyst/nimbalyst/tree/main/packages/runtime/src/ai/server/providers)**.
- SDK dependencies: `@anthropic-ai/claude-agent-sdk`, `@openai/codex-sdk`,
  `@opencode-ai/sdk`, `@zed-industries/codex-acp`
  **[source](https://github.com/nimbalyst/nimbalyst/blob/main/packages/runtime/package.json)**.
- Codex hooks shipped as resources (`codex-pre-edit-hook.mjs`,
  `codex-shell-hook.cjs`) **[source](https://github.com/nimbalyst/nimbalyst/tree/main/packages/electron/resources)**.

### Extension points

- Extension SDK: every editor implements the same `EditorHost` contract;
  extension marketplace (Astro editor, visual git log, mindmap, slides, 3D
  editor); agent providers via `ExtensionAgentProvider` / registry
  **[docs](https://github.com/nimbalyst/nimbalyst#features)**,
  **[source](https://github.com/nimbalyst/nimbalyst/blob/main/packages/electron/src/main/extensions/AgentProviderRegistry.ts)**.

### Known problems

- Requests: remote SSH (#49), Android app (#95), native WSL (#26), headless
  server + desktop client (#416), Cursor and Pi providers (#513, #770);
  "Nimbalyst extremely token-greedy" (#889)
  **[docs](https://github.com/nimbalyst/nimbalyst/issues?q=sort%3Areactions-%2B1-desc)**.

---
## Sculptor (Imbue)

### What it is

- Desktop app (Mac Apple Silicon, Linux, Linux ARM64) for running coding
  agents in parallel; "experimental research preview"; open source, MIT, but
  not taking many external contributions
  **[docs](https://github.com/imbue-ai/sculptor)**. ~240 stars;
  `sculptor-v0.49.0` on 2026-10-21 **[source](https://github.com/imbue-ai/sculptor)**.

### Features

- **Workspaces**: isolated working copy per repo and branch; default mode is a
  git worktree; experimental "Clone" (full clone with mirrored remotes) and
  "In-place" modes; branch pattern `<user>/<slug>`; target branch per
  workspace; branch cleanup policy (never, when safe, always); per-repo setup
  command and env vars **[docs](https://github.com/imbue-ai/sculptor/blob/main/docs/help/workspaces.md)**.
- **Multiple agents per workspace**: agent tab bar below the chat; each agent
  has its own conversation; all share the working copy ("if two agents edit
  the same file simultaneously, they can produce conflicting changes")
  **[docs](https://github.com/imbue-ai/sculptor/blob/main/docs/help/agents.md)**.
- **Changes**: Files panel (Browse, Changes with count, Commits); cumulative
  uncommitted diff; Commit button asks the agent to write the message and
  commit (prompt editable); per-file discard
  **[docs](https://github.com/imbue-ai/sculptor/blob/main/docs/help/changes.md)**.
- **Pull requests**: "Create PR" pushes and opens a GitHub PR via `gh`; button
  shows open/merged/closed, approvals, unresolved comments, CI status
  **[docs](https://github.com/imbue-ai/sculptor/blob/main/docs/help/pull_requests.md)**.
- **UI from deep integration**: WYSIWYG Markdown prompting, typed collapsible
  tool-call cards with inline diffs, live diff review across merge-base
  changes, "Context Compacted" pills, "% context" chip per turn, visible and
  editable message queue, pending questions that survive reloads, task
  dependency graph **[docs](https://github.com/imbue-ai/sculptor/blob/main/docs/help/integrated_harnesses.md)**.
- **Skills**: `sculptor-workflow` pipeline (`spec` → `mock` → `architect` →
  `plan` → `build` → `review`, plus `setup-repo` writing
  `.sculptor/code.md`, `testing.md`, `docs.md`, and `fix-bug` with
  interactive or `--autonomous` modes); `sculptor-experimental`
  **[docs](https://github.com/imbue-ai/sculptor/blob/main/docs/help/skills.md)**.
- Built-in terminal per workspace; command palette `Cmd+K`
  **[docs](https://github.com/imbue-ai/sculptor#docs)**.
- **Container backend** (experimental): a custom backend command can run the
  backend in Docker, over SSH, or in a VM; the app connects to the URL it
  prints **[docs](https://github.com/imbue-ai/sculptor/blob/main/docs/help/experimental/container_backend.md)**.

### Harness integration

- "Integrated harnesses" with per-harness code: Claude Code and Pi; also runs
  "any terminal-based agents" in a terminal without the integrated features
  **[docs](https://github.com/imbue-ai/sculptor#your-choice-of-agent-harness)**.
- Claude Code runs "as a streaming JSON process with its control protocol
  enabled"; "Claude runs sandboxed, with tool permissions auto-approved";
  Sculptor disables Claude's AskUserQuestion and ExitPlanMode tools and
  registers replacements that open native panels; adds system prompt text;
  loads three bundled plugins (`/help`, `/plan`, `/review`); registers a
  pre-compaction hook; queries context fill each turn
  **[docs](https://github.com/imbue-ai/sculptor/blob/main/docs/help/integrated_harnesses.md)**.
- Pi runs in RPC mode with discovery off and a pinned extension set that adds
  ask-question and plan-approval tools; model switching between turns
  **[docs](https://github.com/imbue-ai/sculptor/blob/main/docs/help/integrated_harnesses.md)**.

### Observation and control

- Event streams from both harnesses; questions and plan approvals answered in
  native panels; agent processes survive a quit or crash of Sculptor
  **[docs](https://github.com/imbue-ai/sculptor/blob/main/docs/help/integrated_harnesses.md)**.

### Known problems

- Few public issues: terminal unresponsive after sleep (#182), slow Claude
  install (#14) **[docs](https://github.com/imbue-ai/sculptor/issues/182)**.
- On macOS the containerized backend cannot read Claude's Keychain login;
  needs `claude setup-token` **[docs](https://github.com/imbue-ai/sculptor/blob/main/docs/help/experimental/container_backend.md)**.

---
## Paradev (formerly Superset)

### What it is

- Desktop app (macOS; experimental Linux x64 AppImage; no Windows) for running
  CLI coding agents in parallel, one git worktree + branch + terminals per
  task; CLI command is still `superset`; "not a coding agent itself", does not
  proxy model calls **[docs](https://github.com/ParadevAI/Paradev)**.
- Source available under Elastic License 2.0 (ELv2); ~15.8k stars; very
  active ("we ship daily") **[source](https://github.com/ParadevAI/Paradev)**.
  Pro plan for mobile, automations, cloud workspaces
  **[docs](https://github.com/ParadevAI/Paradev#work-across-devices-and-on-a-schedule)**.

### Features

- **Workspaces**: one worktree per task; "Worktrees separate working files;
  they do not sandbox processes or prevent merge conflicts"; setup/teardown
  scripts; terminal presets; open in IDE
  **[docs](https://github.com/ParadevAI/Paradev#run-independent-tasks)**.
- **Agent monitoring**: sidebar working/done/waiting indicators, completion
  chimes, dock badges **[docs](https://github.com/ParadevAI/Paradev#run-independent-tasks)**.
- **Terminal**: tabs, infinite splits, persistent sessions that survive
  restarts, `⌘I` rich prompt editor with @-file mentions
  **[docs](https://github.com/ParadevAI/Paradev#run-independent-tasks)**.
- **Review**: Changes pane (review, edit, commit, push, open PR); PR pane with
  checks, review threads, merge; select PR diff lines and send feedback to a
  running or new agent session; "Pages" (agent-published reports with pinned
  comments that a watching agent revises)
  **[docs](https://github.com/ParadevAI/Paradev#review-and-revise)**.
- **Browser**: in-app preview with per-workspace port detection; Design mode
  sends a selected element's context to an agent
  **[docs](https://github.com/ParadevAI/Paradev#review-and-revise)**.
- **Agent sessions**: resume, fork (configurable fork args such as `--resume
  {sessionId} --fork-session`), hand work to another agent; Auto-run toggle
  stages the command without pressing Enter
  **[docs](https://docs.paradev.com/agent-integration)**.
- **Built-in chat** pane with inline tool approvals and plan review
  **[docs](https://github.com/ParadevAI/Paradev#supported-agents)**.
- **Automations** (Pro): agent sessions on schedule or events (PR review
  request, Sentry issue, Slack message); Slack and Linear workspace creation
  **[docs](https://github.com/ParadevAI/Paradev#work-across-devices-and-on-a-schedule)**.
- **Remote access** to another host's workspaces with wake command; iPhone and
  iPad app (iOS 26+) to send prompts, review diffs, merge PRs; cloud workspaces
  **[docs](https://github.com/ParadevAI/Paradev#work-across-devices-and-on-a-schedule)**.
- Providers: OpenRouter, Bedrock, Vertex, Vercel AI Gateway; custom themes;
  remappable shortcuts **[docs](https://github.com/ParadevAI/Paradev#customize-your-workflow)**.

### Harness integration

- 22 built-in agents (Claude Code, Amp, Codex, Gemini CLI, Mastracode,
  OpenCode, Pi, Oh My Pi, Copilot, Mistral Vibe, Kimi Code, Grok, Cursor Agent,
  Droid, Polygraph, Hermes, Muse Code, Devin, fx, Antigravity, Kiro, UFO);
  "Any other CLI agent ... works without configuration"
  **[docs](https://docs.paradev.com/agent-integration)**.
- Per-agent settings: command, prompt-only args, prompt transport (argument or
  stdin), resume args, fork args, and whether "Paradev hooks" (status hooks)
  are installed; custom agents with icons
  **[docs](https://docs.paradev.com/agent-integration)**.
- Built-in `superset:*` skills provisioned into agents at launch (orchestrate
  parallel agents, schedule automations, file feedback, diagnose)
  **[docs](https://github.com/ParadevAI/Paradev#customize-your-workflow)**.

### Extension points

- `superset` CLI (create workspaces, launch agents, read terminals, manage
  automations and hosts); TypeScript SDK `@superset_sh/sdk`; MCP server for
  agents to manage workspaces **[docs](https://github.com/ParadevAI/Paradev#more-than-a-desktop-app)**.

### Known problems

- Requests: use on an SSH server (#1513), login not required (#2685),
  multi-repo projects (#2364), Linux (#405), Windows (#2692), comment on all
  diffs (#935); "claude agent always uses hardcoded argument
  `--dangerously-skip-permissions`" (#2419); input lag regression (#2861)
  **[docs](https://github.com/ParadevAI/Paradev/issues?q=sort%3Areactions-%2B1-desc)**.

---
## Emdash (General Action)

### What it is

- Desktop app (macOS, Windows, Linux x64/ARM64) for running agents in
  parallel, each in its own git worktree, locally or on remote machines over
  SSH/SFTP; YC W26 **[docs](https://github.com/generalaction/emdash)**.
- Open source, Apache-2.0; ~6.1k stars; last push 2026-10-30
  **[source](https://github.com/generalaction/emdash)**.

### Features

- Parallel agents in worktrees; issues from Linear, GitHub, Jira, GitLab,
  Asana, Featurebase, Monday.com, Forgejo, Plain, YouTrack sent to agents;
  diff review, PR creation, CI checks, merge
  **[docs](https://github.com/generalaction/emdash#what-you-can-do)**.
- Remote projects over SSH/SFTP (agent, key, password, OpenSSH certificates;
  credentials in OS keychain) **[docs](https://github.com/generalaction/emdash#remote-projects)**.
- Local SQLite state; optional telemetry (`TELEMETRY_ENABLED=false`)
  **[docs](https://github.com/generalaction/emdash#privacy)**.

### Harness integration

- Detects 35 agent CLIs on PATH (Amp, Antigravity `agy`, Auggie, Autohand,
  Crush, Claude Code, Cline, CodeBuddy, Codebuff, Codex, Command Code,
  Continue `cn`, Cursor, Devin, Droid, Freebuff, Copilot, Goose, Grok, Hermes,
  Jules, Junie, Kilocode, Kimi, Kiro, Letta, MiMo, Mistral Vibe, Oh My Pi,
  OpenCode, Pi, Qoder, Qwen, Rovo Dev, ...) with install commands
  **[docs](https://emdash.sh/docs/providers)**.
- For agents with lifecycle hooks, installs "marker-tagged entries in the
  agent's user-level config" for status, notifications, and resumable sessions;
  hooks do nothing outside an Emdash session
  **[docs](https://github.com/generalaction/emdash#agents)**.

### Known problems

- Requests: remote development (#901), choose terminal backend such as Ghostty
  (#287), distinct icon for tool approval requests instead of a spinner
  (#531), submodules (#1104), diff since last turn (#1635); agents not detected
  when installed in WSL (#2140)
  **[docs](https://github.com/generalaction/emdash/issues?q=sort%3Areactions-%2B1-desc)**.

---

## Xum (Coder; formerly Mux)

### What it is

- Desktop app (macOS, Linux) "Coding Agent Multiplexer" with its own agent
  loop (not a wrapper around other CLIs): "Xum has a custom agent loop but much
  of the core UX is inspired by Claude Code"; renamed from Mux after a
  trademark concern from Mux.com **[docs](https://github.com/coder/xum)**.
- Open source, AGPL-3.0; ~2.1k stars; last push 2026-10-31
  **[source](https://github.com/coder/xum)**.

### Features

- Workspaces with runtimes: Local (project dir), Worktree, SSH; central view of
  git divergence and potential conflicts **[docs](https://github.com/coder/xum#features)**.
- Multi-model (Anthropic, OpenAI, xAI, Ollama, OpenRouter); Plan/Exec mode,
  vim input, `/compact`, opportunistic compaction, mode prompts; Mermaid and
  LaTeX output; integrated code review; cost and token tracking; VS Code
  extension **[docs](https://github.com/coder/xum#features)**.
- Exposed as an ACP agent (`mux acp`), listed in acpx's registry
  **[docs](https://github.com/openclaw/acpx/blob/main/docs/agents.md)**.

### Known problems

- Requests: Coder integration (#964), multi-repo mode (#2097), log in with
  OpenAI/Anthropic subscriptions instead of API keys (#687), universal skills
  (#2422); Windows bash tool opens visible terminal windows (#891)
  **[docs](https://github.com/coder/xum/issues?q=sort%3Areactions-%2B1-desc)**.

---

## Paseo

### What it is

- Agentic development environment with a local daemon and desktop, web, iOS,
  Android, and CLI clients; agents, editor, terminals, diffs, PRs, browser in
  one window **[docs](https://github.com/getpaseo/paseo)**.
- Apache-2.0 (third-party portions excepted); ~22.6k stars; last push
  2026-10-31; "no telemetry, tracking, or forced log-ins"
  **[source](https://github.com/getpaseo/paseo/blob/main/LICENSE)**,
  **[docs](https://github.com/getpaseo/paseo)**.

### Features

- Parallel agents, each in its own worktree; split panes; voice mode;
  cross-device via end-to-end encrypted relay or direct TCP/Tailscale; Docker
  image serving the daemon and web UI on port 6767 with `PASEO_PASSWORD`
  **[docs](https://github.com/getpaseo/paseo#getting-started)**.
- **Orchestration**: agents inside Paseo launch other agents (any provider and
  model), send prompts to other agents by ID, check progress, stop tasks,
  collect results, create worktrees, work across machines, create schedules
  and heartbeats (periodic prompts to the same agent); workers appear in a
  "Subagents" track and the parent gets a notification when they finish
  **[docs](https://paseo.sh/docs/orchestration)**.
- Agent profiles with "When to use" notes so an orchestrating agent can pick
  settings **[docs](https://paseo.sh/docs/orchestration)**.
- CLI: `paseo run --provider claude/opus-4.6 "..."`, `--worktree`, `--host
  workstation.local:6767`, `paseo ls`, `paseo attach <id>` (stream output),
  `paseo send <id> "..."`, `--no-wait`
  **[docs](https://github.com/getpaseo/paseo#cli)**.

### Harness integration

- Native providers: Claude Code, Codex, OpenCode, Pi, Antigravity (started
  with `--dangerously-skip-permissions` because "Antigravity cannot ask for
  permission when another app drives it"; transcript not replayed after
  restart; Paseo MCP tools and steering unavailable), Muse Code; plus an ACP
  catalog of one-click installs (Amp, Auggie, Autohand, Cline, Codebuddy,
  CodeWhale, ...) **[docs](https://paseo.sh/docs/supported-providers)**.
- Claude provider uses `@anthropic-ai/claude-agent-sdk` launching the local
  Claude Code executable (per user report)
  **[docs](https://github.com/getpaseo/paseo/issues/1029)**. Anthropic had
  announced that Agent SDK use under a Claude plan would draw from separate
  credits, then paused that change on 2026-06-15; SDK and `claude -p` "still
  draw from your subscription limits"
  **[docs](https://support.claude.com/en/articles/15036540-use-the-claude-agent-sdk-with-your-claude-plan)**.
- Provider overrides in `~/.paseo/config.json` (`command`, `env`)
  **[docs](https://paseo.sh/docs/supported-providers)**.

### Extension points

- Built-in Paseo MCP tools injected into agents (off by default; "Enable Paseo
  tools"); CLI; TypeScript SDK `@getpaseo/client` over WebSocket
  (`ws://127.0.0.1:6767/ws`, `agents.create`, `waitForFinish`)
  **[docs](https://github.com/getpaseo/paseo#typescript-sdk)**.
- Plugins run on the daemon and render in every client: screens, sidebar
  items, panels, Command Center items, slash commands, composer pills,
  timeline items, themes; agent lifecycle (config, env, MCP servers, answer
  permissions, follow up when a turn ends); new providers; `paseo plugin add
  owner/slug` **[docs](https://github.com/getpaseo/paseo#plugins)**.

### Known problems

- Most-upvoted: Antigravity support (#1131, #1277), Claude provider future
  under Agent SDK billing (#1029), OpenCode v2 provider discovery failing
  (#4878), "Use existing sessions from Agents CLIs" (#237)
  **[docs](https://github.com/getpaseo/paseo/issues?q=sort%3Areactions-%2B1-desc)**.

---

## Parallel Code

### What it is

- Desktop app (macOS universal, Linux AppImage/deb) that runs Claude Code,
  Codex, OpenCode, Copilot, Antigravity (and Gemini) side by side, each in a
  worktree; MIT, "no account, no extra subscription", no server of its own
  **[docs](https://github.com/johannesjo/parallel-code)**. ~1.1k stars; last
  push 2026-10-31 **[source](https://github.com/johannesjo/parallel-code)**.

### Features

- Start a task → branch + worktree, symlinks `node_modules` and other
  git-ignored directories, starts the agent
  **[docs](https://github.com/johannesjo/parallel-code#how-it-works)**.
- **Board** sorted by what each task needs: Needs you, Stopped, Working, In
  pull request; `Ctrl+Tab` jumps to the next task waiting for you
  **[docs](https://github.com/johannesjo/parallel-code#how-it-works)**.
- **Review**: inline comments sent back to the agent, guided tour of changes,
  confidence ratings from a separate reviewer model ("an agent never grades its
  own work"), per-file test coverage, before/after screenshots of UI changes
  **[docs](https://github.com/johannesjo/parallel-code#optional-features)**.
- **Finish** merges the branch or pushes it for a PR; closing a task removes
  its worktree **[docs](https://github.com/johannesjo/parallel-code#how-it-works)**.
- **Planning**: Kanban plan in `KANBAN.md`, agents that start and manage their
  own subtasks, "arena" running one prompt with 2-4 agents to compare, mind map
  linked to tasks, live reasoning graph, Markdown canvas, notes
  **[docs](https://github.com/johannesjo/parallel-code#optional-features)**.
- Per-task shell terminals, browser preview, per-project Docker sandboxes (not
  for Antigravity), direct mode without worktree, import existing worktrees
  **[docs](https://github.com/johannesjo/parallel-code#optional-features)**.
- Phone access by QR code over Wi-Fi or Tailscale (port 7777)
  **[docs](https://github.com/johannesjo/parallel-code#getting-started)**.
- Keyboard-first **[docs](https://github.com/johannesjo/parallel-code#why-parallel-code)**.

### Known problems

- "Not for agents that keep working in the cloud while your laptop is off";
  Windows unsupported (#9) **[docs](https://github.com/johannesjo/parallel-code#where-it-fits)**,
  **[docs](https://github.com/johannesjo/parallel-code/issues/9)**.

---
## Remote-control and GUI wrappers for single agents

These are adjacent to multi-agent managers: they wrap one or more agent CLIs
for phones, browsers, or a GUI, and several now run many sessions.

### Happy (Slopus)

- Open source, MIT; ~24.4k stars; mobile app (iOS, Android, web), original
  CLI, relay server for end-to-end encrypted sync; separate `happy-desktop`
  and `happy-agent` runtime repos **[docs](https://github.com/slopus/happy)**.
- CLI: `happy` / `happy claude` starts Claude Code and shows a QR code for a
  phone or browser; `happy codex`, `happy agy`, `happy openclaw`, `happy acp
  opencode`, `happy acp -- custom-agent`; a daemon (`happy daemon
  start|stop|status|list`) lets the phone spawn sessions without an open
  terminal; daemon uses the bundled `claude-agent-sdk`
  **[docs](https://github.com/slopus/happy/blob/main/packages/happy-cli/README.md)**.
- Desktop/Happy Harness: any model (Claude, Codex, Grok subscriptions; Gemini
  via Antigravity; OpenCode or any ACP agent as a harness), delegation to
  subagents on any model, multiplayer sessions where two people watch and steer
  the same agent **[docs](https://github.com/slopus/happy#what-you-get)**.
- Limits: `agy --print` has no approval surface, so tool calls proceed without
  prompts regardless of the chosen mode
  **[docs](https://github.com/slopus/happy/blob/main/packages/happy-cli/README.md)**.
- Requests: OpenCode support (#265, #394, #344, #538), pi harness (#1213)
  **[docs](https://github.com/slopus/happy/issues?q=sort%3Areactions-%2B1-desc)**.

### Omnara

- Previously "Mission Control for Your AI Agents": `pip install omnara`,
  `omnara` started Claude Code (or `--agent codex`) synced between terminal,
  web dashboard, and an iOS app for answering agent questions
  **[docs](https://github.com/omnara-ai/omnara/blob/be6ef792fc70cd203ef2519cafc10e0e5e282758/README.md)**,
  **[inferred]** (README at a September 2025 commit).
- Now "The open-source managed agent platform", Apache-2.0, a self-hostable
  alternative to Claude Managed Agents and OpenAI's agent platform: agents
  defined in `agent.yaml` (instruction, model provider, machine pools), run by
  `npx omnara agents launch`, chat via `npx omnara agents chat`, TypeScript SDK
  `@omnara/sdk` with an event stream; Omnara Cloud or Docker Compose self-host
  **[docs](https://github.com/omnara-ai/omnara)**. ~3.3k stars
  **[source](https://github.com/omnara-ai/omnara)**.

### CloudCLI (Claude Code UI, siteboon)

- Web UI (desktop and mobile) for Claude Code, Cursor CLI, Codex, OpenCode;
  chat, integrated shell, file explorer with editing, git explorer (stage,
  commit, switch branches), browser sessions, session resume and history,
  plugin system (tabs, backend services), optional TaskMaster integration; hosted
  "CloudCLI Cloud" containers **[docs](https://github.com/siteboon/claudecodeui)**.
- AGPL-3.0; ~14.4k stars **[source](https://github.com/siteboon/claudecodeui)**.
- Requests: accept/deny individual tool use in the UI (#15), MCP management
  (#18), remote sessions over SSH (#163)
  **[docs](https://github.com/siteboon/claudecodeui/issues?q=sort%3Areactions-%2B1-desc)**.

### opcode (formerly Claudia, Asterisk)

- Tauri 2 desktop GUI for Claude Code: browses `~/.claude/projects/`, resumes
  sessions, custom "CC Agents" with system prompts run as background processes
  with execution history, usage analytics (cost and tokens by model, project,
  time; export), MCP server registry with Claude Desktop import, timeline
  checkpoints with fork and diff, CLAUDE.md editor
  **[docs](https://github.com/winfunc/opcode#-features)**.
- AGPL-3.0; ~22.4k stars; last push 2026-09-18
  **[source](https://github.com/winfunc/opcode)**.
- Requests: other agent CLIs (#66), HTTP remote access (#79), Windows build
  (#332) **[docs](https://github.com/winfunc/opcode/issues?q=sort%3Areactions-%2B1-desc)**.

---
## Vendor agent apps and IDE agent managers

### OpenAI Codex desktop (now inside the ChatGPT desktop app)

- Closed source; macOS, Windows, Linux; the Codex app's docs now describe the
  "ChatGPT desktop app" where users choose ChatGPT or Codex and run projects in
  parallel **[docs](https://developers.openai.com/codex/app.md)**.
- **Worktrees**: per chat choose Local or Worktree and a starting branch;
  Codex-managed worktrees in `$CODEX_HOME/worktrees`, detached HEAD by default,
  uncommitted changes carried over; "Create branch here"; **Handoff** moves a
  chat and its code between Local (foreground) and Worktree (background);
  `.worktreeinclude` for ignored files; local environment setup scripts
  **[docs](https://developers.openai.com/codex/app/worktrees.md)**.
- **Scheduled tasks** (automations): standalone or inside a chat, on dedicated
  background worktrees; respect sandbox mode (`read-only`, `workspace-write`,
  full access) **[docs](https://developers.openai.com/codex/app/automations.md)**.
- **Mobile**: the ChatGPT mobile app controls Codex on a connected computer
  **[docs](https://developers.openai.com/codex/app/worktrees.md)**.
- **Subagents**: "spawn two agents", "one agent per point, wait for all";
  steer, stop, or close subagent threads; custom agents in config
  **[docs](https://learn.chatgpt.com/docs/agent-configuration/subagents.md)**.
- Integration surface for other tools: Codex app-server (see Part 5) and the
  Codex SDK `@openai/codex-sdk` (`startThread`, `run`, resume) for CI and
  custom agents; `codex mcp-server` has been removed
  **[docs](https://learn.chatgpt.com/docs/codex-sdk.md)**.

### Cursor 3 Agents Window and Cloud Agents

- Closed source. Cursor 3 (2026-04-02) added the Agents Window: multi-workspace
  agents across local, worktree, cloud, and remote SSH; new diffs view with
  commit and PR management; local↔cloud handoff; cloud subagents via
  `/in-cloud`; `/autopilot` for a PR; `/worktree`; `.cursor/worktrees.json`
  setup **[docs](https://cursor.com/docs/agent/agents-window)**,
  **[docs](https://cursor.com/changelog/3-0)**.
- **Cloud Agents**: isolated Ubuntu VMs with environments (agent-led setup,
  snapshot, or Dockerfile in `.cursor/environment.json`), builds, secrets,
  egress restrictions, Tailscale/private connectivity; multi-repo; computer
  use and remote desktop; artifacts (screenshots, videos, logs); MCP (HTTP and
  stdio, OAuth) plus a built-in Cursor Cloud MCP for transcripts and run
  events; repo hooks from `.cursor/hooks.json` (`preToolUse`,
  `beforeShellExecution`, `afterFileEdit`, `beforeSubmitPrompt`,
  `subagentStart`/`subagentStop`, ...); start from iOS app, web, desktop,
  Slack, GitHub/Bitbucket comments, Linear, API
  **[docs](https://cursor.com/docs/cloud-agent)**.
- **API**: `POST /v1/agents`, `GET /v1/agents`, `GET /v1/agents/{id}`, `POST
  /v1/agents/{id}/runs` (follow-up or steer), `GET /v1/agents/{id}/runs`,
  `GET /v1/models` **[docs](https://cursor.com/docs/cloud-agent/api/endpoints)**.
- **Automations**: schedule or GitHub, GitLab, Slack, webhook, Linear events;
  `/automate` skill; managed Bugbot and Security Agents
  **[docs](https://cursor.com/docs/cloud-agent/automations)**.
- **Self-Hosted Machines**: Cursor runs the agent loop and inference; tool
  execution happens on a customer worker (Enterprise)
  **[docs](https://cursor.com/docs/cloud-agent/self-hosted)**.

### VS Code Agents window (Agent HQ)

- VS Code (open-source editor, Copilot service closed) runs Copilot, Claude, or
  Codex "agent harnesses"; Agents window (`code --agents`, or
  insiders.vscode.dev/agents in a browser) for sessions across projects; start
  in a folder, repo, or remote workspace (SSH, dev tunnel); attach issues/PRs
  as context; choose harness, model, permission level, isolation mode
  (current folder or a git worktree); `Alt+Enter` starts a session in the
  background; sessions list shows status and change statistics
  **[docs](https://code.visualstudio.com/docs/copilot/agents/agents-window)**.
- Agents run locally, in the background via Copilot CLI, or in the cloud
  (Copilot coding agent); GitHub Environments lets other clients (Copilot app,
  CLI, GitHub.com, Mobile) continue local sessions; cloud sessions and cloud
  automations in preview **[docs](https://code.visualstudio.com/docs/copilot/agents/overview)**.
- Claude and Codex harnesses disabled under Copilot enterprise-managed settings
  **[docs](https://code.visualstudio.com/docs/copilot/agents/agents-window)**.

### Google Antigravity

- Closed-source agent-first IDE with an Agent Manager ("Manager Surface") for
  spawning, orchestrating, and observing multiple agents across workspaces;
  agents produce Artifacts (task lists, implementation plans, screenshots,
  browser recordings) that users comment on while the agent continues
  **[docs](https://developers.googleblog.com/en/build-with-google-antigravity-our-new-agentic-development-platform/)**.
- Antigravity CLI (`agy`) succeeded Gemini CLI per third-party tools (Happy:
  "Antigravity CLI (Gemini's successor)"); when driven by another app it
  cannot ask permission (Paseo runs it with `--dangerously-skip-permissions`)
  **[docs](https://github.com/slopus/happy/blob/main/packages/happy-cli/README.md)**,
  **[docs](https://paseo.sh/docs/supported-providers)**.
- Current official Agent Manager docs could not be retrieved (404)
  **[inferred]**.

### Warp

- Closed-source terminal ("agentic development environment") with its own Warp
  Agent plus first-class support for third-party CLI agents (Claude Code,
  Codex, OpenCode, ...): auto-detection, rich input editor, agent
  notifications, inline code review, and Remote Control (publish a third-party
  agent session to the cloud to monitor and steer from a phone)
  **[docs](https://docs.warp.dev/_llms-txt/agents.txt)**.
- Integration via plugins installed into each harness:
  `claude plugin install warp@claude-code-warp`, `codex plugin add
  warp@codex-warp`, an `opencode-warp` plugin
  **[docs](https://docs.warp.dev/_llms-txt/agents.txt)**.
- Agent Management Panel (app) and Oz Runs page (web): local and cloud runs
  with status working/blocked/failed/succeeded/canceled, source (local, Oz CLI,
  Slack, Linear, schedule, API), parent-child runs, session links, usage;
  "not a replacement for full APM, OpenTelemetry"
  **[docs](https://docs.warp.dev/agent-platform/cloud-agents/managing-cloud-agents.md)**.
- Automation Platform runs Warp Agent or third-party harnesses (Claude Code,
  Codex) as cloud agents with triggers, environments, secrets; Warp Factories
  **[docs](https://docs.warp.dev/_llms-txt/automation-platform.txt)**.

### T3 Code (Ping.gg)

- Open source, MIT; ~32.8k stars; "agent harness control surface" with
  desktop, local web app (`t3` starts a server), and iOS/Android apps; drives
  Codex, Claude Code, Cursor CLI, Grok Build, OpenCode, Antigravity using the
  user's own subscriptions; `t3 service install` runs it in the background
  **[docs](https://github.com/pingdotgg/t3code)**,
  **[source](https://github.com/pingdotgg/t3code)**.
- Translates each harness's event stream into one interface (third-party
  description) **[docs](https://flaviocopes.com/t3-code)**.

---
# Part 4. Hosted and cloud agent platforms

## Claude Code on the web, Projects, and cloud scheduled tasks (Anthropic)

- Closed, hosted. Cloud sessions run in Anthropic-managed environments, started
  from claude.ai/code, the mobile app, Desktop, `claude --cloud "task"` (one
  repo per session; several `--cloud` commands run in parallel), or cloud
  scheduled tasks **[docs](https://code.claude.com/docs/en/claude-code-on-the-web.md)**.
- Handoff is one-way from CLI: `claude --teleport [id]`, `/teleport`, `/tasks`
  then `t` pull a cloud session into the terminal; Desktop can move a local
  session to Cloud with a summary
  **[docs](https://code.claude.com/docs/en/claude-code-on-the-web.md)**,
  **[docs](https://code.claude.com/docs/en/desktop.md)**.
- Review: diff view against base branch or any branch, inline comments sent
  with the next message, PR creation; Auto-fix watches PRs for CI failures and
  review comments; shareable sessions; GitHub App or `/web-setup`
  authentication **[docs](https://code.claude.com/docs/en/claude-code-on-the-web.md)**.
- **Projects** (public beta, Pro/Max/Ultra): one conversation where Claude
  starts a thread (usually a cloud session) per task, shares project
  instructions and memory, and shows an Overview pane of finished threads,
  PRs ready for review, and "Waiting on you"; threads can run on the user's
  computer through Remote Control **[docs](https://code.claude.com/docs/en/claude-projects.md)**.
- Cloud scheduled tasks (formerly routines): on a schedule, API call, or
  GitHub event **[docs](https://code.claude.com/docs/en/routines.md)**.
- Cloud sessions reachable by cross-session messaging but cannot reply across
  machines **[docs](https://code.claude.com/docs/en/cross-session-messaging.md)**.

## OpenAI Codex Cloud

- Closed, hosted. Tasks start from a published cloud environment (GitHub
  repos, dependencies, tools inspected and set up by Codex, network access and
  secrets configured); each task has its own workspace and runs while the
  user's computer sleeps; review changed files and results, request
  follow-ups, commit or open a PR; start from web, mobile, desktop, Slack, or
  the CLI (`codex cloud`) **[docs](https://learn.chatgpt.com/docs/cloud.md)**.
- Code review of PRs and local changes
  **[docs](https://learn.chatgpt.com/docs/code-review.md)**.

## GitHub Copilot cloud agent (coding agent) and Agent HQ

- Closed, hosted on GitHub Actions infrastructure. Assign an issue to Copilot
  (👀 reaction, `copilot/` branch, draft PR, session) or start from the agents
  panel, Copilot app, VS Code, CLI (`/delegate`), or Mobile
  **[docs](https://docs.github.com/en/copilot/how-tos/copilot-on-github/use-copilot-agents/overview)**,
  **[docs](https://code.visualstudio.com/updates/v1_106)**.
- Agents panel on every GitHub page and Copilot home: session log with
  reasoning and tools, token usage, session length; commits authored by
  Copilot with the requester as co-author and a link to session logs
  **[docs](https://docs.github.com/en/copilot/how-tos/use-copilot-agents/manage-agents)**.
- Steering: type into the prompt box below the log; applied after the current
  tool call; each message consumes AI credits; "Steering is not available for
  third-party coding agents" **[docs](https://docs.github.com/en/copilot/how-tos/use-copilot-agents/manage-agents)**.
- Continue the same cloud session from the Copilot app, Mobile, GitHub.com, or
  Copilot CLI (`/resume`) **[docs](https://docs.github.com/en/copilot/how-tos/use-copilot-agents/manage-agents)**.
- Stop session ends the Actions run and keeps pushed commits; cloud sessions
  can be archived, not deleted **[docs](https://docs.github.com/en/copilot/how-tos/use-copilot-agents/manage-agents)**.
- Mention `@copilot` in PR comments to push fixes to the same branch
  **[docs](https://learn.microsoft.com/en-us/training/modules/github-copilot-code-agent/3-assign-track-troubleshoot-copilot-code-agent-tasks)**.

## Cursor Cloud Agents

- See the Cursor section in Part 3: VMs, environments, API (`/v1/agents`,
  follow-ups and steering via `/runs`), automations, self-hosted machines,
  start from Slack/GitHub/Linear/iOS/web **[docs](https://cursor.com/docs/cloud-agent)**.

## Google Jules

- Closed, hosted asynchronous coding agent with a REST API (`v1alpha`):
  sessions are the core resource (create, get, list, `sendMessage`,
  `approvePlan`); `requirePlanApproval` at creation holds plans
  (`AWAITING_PLAN_APPROVAL`), otherwise plans auto-approve; state
  `AWAITING_USER_FEEDBACK`; activities list plan generated, plan approved, user
  messages, agent messages **[docs](https://developers.google.com/jules/api/reference/rest)**,
  **[docs](https://jules.google/docs/api/reference/types)**.
- A `jules` CLI is detected by Emdash
  **[docs](https://emdash.sh/docs/providers)**.

## Devin (Cognition)

- Closed, hosted; sessions in VMs; Ask and Agent modes; Slack, IDE, CLI
  (`devin`), Devin Review, stacked PRs, automations and schedules, knowledge,
  skills, environment blueprints, self-hosted "Outposts"
  **[docs](https://docs.devin.ai/llms.txt)**.
- **Managed Devins**: a coordinator session spins up child sessions (prompt,
  skills, tags, ACU limits), messages them, is woken automatically when a child
  finishes or needs input, monitors ACU consumption, puts children to sleep or
  terminates them, schedules reminders to itself
  **[docs](https://docs.devin.ai/work-with-devin/advanced-capabilities.md)**.
- MCP server for creating sessions and managing playbooks, knowledge,
  schedules; can wait for all child sessions in one call
  **[docs](https://docs.devin.ai/work-with-devin/devin-mcp.md)**.
- Devin CLI speaks ACP (`devin acp`) and supports Herdr reporting
  **[docs](https://github.com/openclaw/acpx/blob/main/docs/agents.md)**,
  **[docs](https://herdr.dev/docs/agents/)**.

## Factory (Droids)

- Closed; Droid CLI, Factory App (worktrees), `droid exec` headless mode
  (read-only by default, `--auto low` for edits) with recipes that fan out in
  parallel, Python SDK, Droid Sessions API (`GET /api/v0/sessions` with
  `parent_session_id` for Task subagents; selected orgs)
  **[docs](https://docs.factory.com/llms.txt)**,
  **[docs](https://docs.factory.com/api-reference/sessions.md)**.
- `droid exec --output-format acp` serves ACP
  **[docs](https://github.com/openclaw/acpx/blob/main/docs/agents.md)**.

## Terragon (shut down)

- Hosted background-agent orchestrator for Claude Code, Codex, Amp, Gemini in
  cloud sandbox containers; per-task branches with checkpointed commits and
  PRs; `terry` CLI for local takeover plus an MCP server; BYO Claude/ChatGPT
  subscriptions; automations (recurring, on new issues/PRs); Slack and GitHub
  @-mentions; real-time browser status and notifications
  **[docs](https://github.com/terragon-labs/terragon-oss)**.
- **Shut down**; open-source snapshot published 2026-01-16 "as-is, with no
  guarantees of maintenance" (~260 stars)
  **[docs](https://github.com/terragon-labs/terragon-oss)**.

## OpenHands Cloud

- Open-source agent (MIT, ~92k stars) with a hosted Cloud: GitHub resolver
  (label an issue `openhands`, bot links to progress), Cloud API for starting
  conversations and checking status, software-agent SDK
  **[docs](https://docs.all-hands.dev/modules/usage/cloud/cloud-github-resolver)**,
  **[source](https://github.com/OpenHands/OpenHands)**.
- Partnered with Toad as a terminal interface; ACP agent via Toad's registry
  **[docs](https://www.openhands.dev/blog/20251218-openhands-toad-collaboration)**.

## Ona (formerly Gitpod)

- Hosted (or in customer VPC) background agents, each in a fresh ephemeral
  environment destroyed after the task; tasks from Linear, Slack, GitHub;
  parallel tasks; review in browser VS Code or desktop IDE
  **[docs](https://ona.com/llms-full.txt)**.

## Other hosted surfaces (brief)

- **Warp Oz / Automation Platform**: cloud runs of Warp Agent, Claude Code, or
  Codex with triggers and a Runs page (Part 3)
  **[docs](https://docs.warp.dev/_llms-txt/automation-platform.txt)**.
- **Conductor Cloud**, **cmux Cloud**, **Paradev cloud workspaces**, **Vibe
  Kanban Cloud** (being removed): see their sections
  **[docs](https://www.conductor.build/docs/cloud)**,
  **[docs](https://cmux.com/docs/cloud)**.
- **Omnara**: self-hostable managed agent platform (Part 3)
  **[docs](https://github.com/omnara-ai/omnara)**.

---
# Part 5. Protocols

## Agent Client Protocol (ACP)

### What it is

- Open protocol (Apache-2.0) that "standardizes communication between code
  editors ... and coding agents"; started by Zed, now under the
  `agentclientprotocol` GitHub org; ~4.5k stars; official SDKs in Kotlin, Java,
  Python, Rust, TypeScript **[docs](https://github.com/agentclientprotocol/agent-client-protocol)**.
- Stable protocol version is `1`; v2 is documented and "still labeled draft";
  implementers should negotiate per connection and keep v1
  **[docs](https://agentclientprotocol.com/protocol/v2/migration.md)**.
- Changes go through "Requests for Dialog" (RFDs)
  **[docs](https://agentclientprotocol.com/rfds/about.md)**.

### Mechanics

- JSON-RPC 2.0, bidirectional; transport is stdio (client launches the agent
  as a subprocess, newline-delimited messages, stderr for logs); a Streamable
  HTTP remote transport is being specified in an RFD; custom transports allowed
  **[docs](https://agentclientprotocol.com/protocol/v1/transports.md)**.
- **v1 methods**: `initialize`, `authenticate`, `session/new`, `session/load`,
  `session/prompt` (response ends the turn with a stop reason),
  `session/cancel`, `session/set_mode`, `session/set_config_option`,
  `session/list`, `session/resume`; agent→client `session/update`
  (`user_message_chunk`, `agent_message_chunk`, `agent_thought_chunk`,
  `tool_call`, `tool_call_update`, plan, ...), `session/request_permission`,
  `fs/read_text_file`, `fs/write_text_file`, `terminal/create`,
  `terminal/output`, `terminal/release`, `terminal/wait_for_exit`,
  `terminal/kill`, `elicitation/create`
  **[docs](https://agentclientprotocol.com/protocol/v2/migration.md)**,
  **[source](https://github.com/batrachianai/toad/blob/main/src/toad/acp/agent.py)**.
- **v2 changes**: `session/prompt` response only acknowledges acceptance;
  progress and completion come as `state_update` notifications (`running`,
  `idle`, `requires_action`) with stop reasons; updates are upserts by ID;
  client file system, terminal, and session-mode APIs removed (use
  client-provided MCP servers); `session/load` replaced by `session/resume` with
  `replayFrom`; `session/list`, `session/close` required; `auth/login`,
  `auth/logout`; almost all enums extensible with `_`-prefixed custom values
  **[docs](https://agentclientprotocol.com/protocol/v2/migration.md)**.
- Other spec areas: content blocks, tool calls, elicitation, agent plan,
  slash commands, extensibility (`_meta`), security considerations
  **[docs](https://agentclientprotocol.com/llms.txt)**.

### Registry

- `https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json`;
  curated to agents that support user authentication (CI checks valid
  `authMethods` in the handshake); hourly cron updates versions from npm, PyPI,
  GitHub releases; JetBrains-specific indexes including one for JetBrains Air
  **[docs](https://github.com/agentclientprotocol/registry)**.
- 43 agents on 2026-10-30, including Amp, Auggie CLI, Claude Agent, Cline,
  Codex, Copilot, Cursor, Devin, Factory Droid, Gemini CLI, Google Antigravity,
  Grok Build, Junie, Kilo, Kimi CLI, Mistral Vibe, OpenCode, OpenCode V2, pi
  ACP, Qwen Code, Qoder CLI, goose, Poolside
  **[source](https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json)**.

### Adapters and native support

- `@agentclientprotocol/claude-agent-acp` (~2.7k stars): ACP agent built on the
  Claude Agent SDK; supports @-mentions, images, tool calls with permission
  requests, edit review, TODO lists, nested subagent transcripts, interactive
  and background terminals, slash commands, client MCP servers, Ultracode
  config option; runs `/mcp reconnect|enable|disable` through the SDK control
  API "because Claude Code refuses them in SDK mode"; JetBrains AIR extensions
  under `_meta` (goals, session failure, permission presentation)
  **[docs](https://github.com/agentclientprotocol/claude-agent-acp)**.
- `agentclientprotocol/codex-acp` (Codex CLI as an ACP server); the earlier
  `zed-industries/codex-acp` is archived
  **[source](https://github.com/agentclientprotocol/codex-acp)**,
  **[source](https://github.com/zed-industries/codex-acp)**.
- Native ACP modes: `gemini --acp`, `cursor-agent acp`, `copilot --acp
  --stdio`, `devin acp`, `droid exec --output-format acp`, `kimi acp`,
  `kiro-cli-chat acp`, `qwen --acp`, `opencode acp`, `mux acp`, `junie
  --acp=true`, `grok agent stdio`, `pool acp` **[docs](https://github.com/openclaw/acpx/blob/main/docs/agents.md)**.

### Clients

- Editors: Zed, JetBrains, Neovim (CodeCompanion, agentic.nvim, avante.nvim),
  Emacs (agent-shell.el), VS Code extensions, Visual Studio (Poolside), Qt
  Creator, Sublime, Obsidian plugins (some run several ACP agents in parallel),
  Unity; CLI/TUI: acpx, Toad, Hydra, Nori, pool, Raven (DAG of ACP agents),
  Microsoft Intelligent Terminal, SpurCode (fleet of ACP agents across
  worktrees); desktop/web: ACP UI and others
  **[docs](https://agentclientprotocol.com/get-started/clients.md)**.
- Multi-agent tools that use ACP for some harnesses: Vibe Kanban (Gemini,
  generic `acp` executor), Nimbalyst (Codex ACP provider), Paseo (ACP catalog),
  Happy (`happy acp`), Toad, acpx **[source](https://github.com/BloopAI/vibe-kanban/tree/main/crates/executors/src/executors)**,
  **[docs](https://paseo.sh/docs/supported-providers)**.

### Known limits

- Mode values are adapter-defined and not standardized; agents differ in which
  optional methods they implement; adapters lag harness features (e.g. Claude
  `/mcp` commands refused in SDK mode)
  **[docs](https://github.com/openclaw/acpx/blob/main/docs/session-control.md)**,
  **[docs](https://github.com/agentclientprotocol/claude-agent-acp)**.

---

## Zed external agents and terminal threads

- **External Agents**: ACP agents shown as threads in Zed's Agent Panel and
  Threads Sidebar; installed from the ACP Registry (`zed: acp registry`) or as
  custom `agent_servers` entries (`{"command": "pool", "args": ["acp"], "type":
  "custom"}`); the agent owns auth, billing, models, native config; Zed MCP
  servers "may be forwarded over ACP"; Zed agent profiles and skills do not
  apply **[docs](https://zed.dev/docs/ai/external-agents)**.
- **Terminal Threads**: run any agent CLI/TUI in a terminal that Zed organizes
  as a thread, grouped by project; the CLI owns everything else
  **[docs](https://zed.dev/docs/ai/terminal-threads)**.
- Zed passes its Google AI key to Gemini CLI as `GEMINI_API_KEY` if none is set
  **[docs](https://zed.dev/docs/ai/external-agents)**.

---

## Model Context Protocol (MCP)

- Open protocol for connecting models/agents to tools and data; spec versions
  `2024-11-05`, `2025-03-26`, `2025-06-18`, `2025-11-25`, `2026-07-28`, and
  `draft` **[source](https://github.com/modelcontextprotocol/modelcontextprotocol/tree/main/docs/specification)**.
- `2026-07-28` changes: stateless protocol (no `initialize` handshake; version
  and capabilities in each request's `_meta`); `Mcp-Session-Id` removed;
  `server/discover`; `subscriptions/listen` replaces GET stream and resource
  subscribe; tasks moved to an official extension
  (`io.modelcontextprotocol/tasks`, polling via `tasks/get`); Multi Round-Trip
  Requests replace server-initiated `sampling/createMessage`,
  `elicitation/create`, `roots/list`; results carry `resultType`; SSE
  resumability removed; OpenTelemetry trace context in `_meta`
  **[docs](https://modelcontextprotocol.io/specification/2026-07-28/changelog.md)**.
- Role in multi-agent tools: container-use is entirely an MCP server; Vibe
  Kanban, Conductor, Paradev, Paseo, Devin, Terragon expose MCP servers so agents
  can create workspaces/tasks; agent-deck pools MCP processes across sessions;
  Claude Code channels are MCP servers that push events
  **[docs](https://github.com/dagger/container-use)**,
  **[docs](https://vibekanban.com/docs/integrations/vibe-kanban-mcp-server.md)**,
  **[docs](https://code.claude.com/docs/en/channels.md)**.

---

## Agent2Agent (A2A)

- Open protocol (Apache-2.0, ~26.4k stars) for "communication and
  interoperability between opaque agentic applications": discovery via "Agent
  Cards", negotiation of modalities (text, forms, media), long-running tasks,
  without exposing internal state, memory, or tools; JSON-RPC 2.0 over HTTP(S);
  SDKs (e.g. `a2a-sdk` on PyPI) **[docs](https://github.com/a2aproject/A2A)**.
- No surveyed local coding-agent manager uses A2A to talk to harnesses
  **[inferred]**.

---

## Codex app-server (OpenAI)

- "The interface Codex uses to power rich clients (for example, the Codex VS
  Code extension)": authentication, conversation history, approvals, streamed
  events **[docs](https://learn.chatgpt.com/docs/app-server.md)**.
- JSON-RPC 2.0 without the `"jsonrpc"` header; transports stdio (JSONL,
  default), WebSocket (`--listen ws://IP:PORT`, experimental, with
  capability-token or signed-bearer-token auth and `/readyz`, `/healthz`),
  Unix socket; `codex --remote ws://...` connects the TUI to a remote
  app-server; overload error `-32001`
  **[docs](https://learn.chatgpt.com/docs/app-server.md)**.
- Methods include `thread/start`, `thread/resume`, `thread/fork`,
  `thread/list`, `thread/read`, `thread/archive`, `thread/rollback`,
  `thread/compact/start`, `thread/goal/set`, `thread/shellCommand`,
  `thread/backgroundTerminals/list`, `thread/status/changed`,
  `thread/tokenUsage/updated`, `turn/start`, `turn/steer`, `turn/interrupt`,
  `turn/diff/updated`, `turn/plan/updated`, `item/started`,
  `item/completed`, `item/agentMessage/delta`,
  `item/commandExecution/requestApproval`, `item/fileChange/requestApproval`,
  `item/permissions/requestApproval`, `item/tool/requestUserInput`,
  `mcpServer/elicitation/request`, `review/start`, `account/rateLimits/read`,
  `account/usage/read`, `model/list`, `skills/list`, `plugin/install`,
  `hooks/list`, `fs/*`, `process/spawn`, `command/exec`
  **[docs](https://learn.chatgpt.com/docs/app-server.md)**.
- Used by Vibe Kanban (`codex_app_server_protocol`) and Nimbalyst; the older
  `codex mcp-server` was removed in favor of app-server
  **[source](https://github.com/BloopAI/vibe-kanban/blob/main/crates/executors/src/executors/codex.rs)**,
  **[docs](https://learn.chatgpt.com/docs/codex-sdk.md)**.

---

## Harness-native programmatic interfaces (summary)

- **Claude Code**: `claude -p --output-format stream-json --input-format
  stream-json` with control protocol (`--permission-prompt-tool=stdio`, hook
  callbacks) as used by Vibe Kanban and Sculptor; Claude Agent SDK; hooks;
  status line JSON; cross-session inbox socket; `claude agents --json`
  **[source](https://github.com/BloopAI/vibe-kanban/blob/main/crates/executors/src/executors/claude.rs)**,
  **[docs](https://code.claude.com/docs/en/agent-view.md)**.
- **OpenCode**: HTTP server with SDK (`@opencode-ai/sdk`), used by Vibe
  Kanban, Nimbalyst, agent-deck (OpenCode 2 session API)
  **[source](https://github.com/BloopAI/vibe-kanban/blob/main/crates/executors/src/executors/opencode.rs)**.
- **Pi**: RPC mode and TypeScript extensions (used by Sculptor, Herdr,
  agent-deck) **[docs](https://github.com/imbue-ai/sculptor/blob/main/docs/help/integrated_harnesses.md)**.
- **Codex**: app-server, SDK, `hooks.json` with `[features] hooks = true`,
  notify hook **[docs](https://herdr.dev/docs/integrations/)**.

---
# Part 6. Executable plugin conventions

Each entry: how the host finds the plugin, how it invokes it, the wire format,
and lifecycle.

## git credential helpers

- **Discovery**: `credential.helper` config; a bare name `foo` runs
  `git-credential-foo` from PATH ("naming their program
  `git-credential-$NAME`"); a value starting with `!` is a shell snippet;
  arguments allowed (`store --file ~/.git-secret.txt`)
  **[docs](https://git-scm.com/docs/gitcredentials)**.
- **Invocation**: helper called with an action (`get`, `store`, `erase`);
  `git credential` front end supports `fill`, `approve`, `reject`,
  `capability` **[docs](https://git-scm.com/docs/git-credential)**.
- **Format**: newline-separated `key=value` attributes on stdin/stdout
  (protocol, host, path, username, password, ...), ended by a blank line
  **[docs](https://git-scm.com/docs/git-credential)**.
- **Lifecycle**: one short-lived process per call; multiple helpers tried in
  order; shipped helpers include cache, store, osxkeychain, libsecret, wincred,
  oauth **[docs](https://git-scm.com/docs/gitcredentials)**.

## Docker credential helpers

- **Discovery**: `credsStore` (global) or `credHelpers` (per registry) in
  `~/.docker/config.json` name a suffix; Docker runs
  `docker-credential-<suffix>` from PATH
  **[docs](https://github.com/docker/docker-credential-helpers)**.
- **Invocation**: one argument action: `store`, `get`, `erase`, `list`,
  `version` **[source](https://github.com/docker/docker-credential-helpers/blob/master/credentials/credentials.go)**.
- **Format**: JSON on stdin/stdout with `ServerURL`, `Username`, `Secret`
  (`get` takes the server URL as input)
  **[source](https://github.com/docker/docker-credential-helpers/blob/master/credentials/credentials.go)**.
- **Lifecycle**: short-lived process per call; Go `client` package for hosts
  **[docs](https://github.com/docker/docker-credential-helpers)**.

## kubectl plugins

- **Discovery**: any executable named `kubectl-*` on PATH; `kubectl plugin
  list` scans PATH and warns about non-executables and shadowed names; plugins
  cannot override built-in commands (`kubectl-version` never runs)
  **[docs](https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/)**.
- **Invocation**: command path from the filename, dash-separated
  (`kubectl-foo-bar` → `kubectl foo bar`); longest-match resolution with the
  rest passed as arguments; underscores in filenames for dashes in commands;
  `kubectl-create-something` extends `kubectl create`; inherits kubectl's
  environment **[docs](https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/)**.
- **Distribution**: Krew plugin manager and index (~7k stars)
  **[source](https://github.com/kubernetes-sigs/krew)**.

## git subcommands

- An executable `git-foo` on PATH (or in `git --exec-path`) runs as `git
  foo`; aliases via `alias.*` config
  **[docs](https://git-scm.com/docs/git)**.

## HashiCorp go-plugin

- Plugin system "over RPC" used by Terraform, Vault, Nomad, Packer: host
  launches the plugin as a subprocess; one connection over `net/rpc` or gRPC
  (gRPC allows plugins in any language); bidirectional calls; protocol
  versioning with human-readable errors; plugin stdout/stderr mirrored to the
  host; TTY preserved; "reattach" to running plugins across host upgrades;
  optional checksums and TLS; a plugin panic can't crash the host
  **[docs](https://github.com/hashicorp/go-plugin)**.
- **Handshake**: a magic-cookie env var guards against running the plugin
  directly; the plugin prints one stdout line
  `CORE-PROTOCOL-VERSION|APP-PROTOCOL-VERSION|NETWORK-TYPE|NETWORK-ADDR|PROTOCOL`,
  e.g. `1|3|unix|/path/to/socket|grpc`
  **[docs](https://github.com/hashicorp/go-plugin/blob/main/docs/internals.md)**.

## Language Server Protocol (LSP)

- JSON-RPC with a header part (`Content-Length: ...\r\n`) and content part,
  usually over stdio of a server the editor launches; capabilities exchanged in
  the `initialize` request; client ignores server capabilities it doesn't
  understand **[docs](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/)**.
- ACP explicitly modeled on this editor↔server split for agents
  **[docs](https://www.i-programmer.info/news/90-tools/18619-toad-pythonic-universal-terminal-interface-for-ai-agents.html)**.

## asdf plugins

- A git repo `asdf-<tool>` with executable scripts: required `bin/list-all`,
  `bin/install`; recommended `bin/download`, `bin/latest-stable`; optional
  `bin/help.*`, `bin/list-bin-paths`, `bin/exec-env`, `bin/exec-path`,
  `bin/uninstall`, `bin/list-legacy-filenames`, `bin/parse-legacy-file`, and
  hooks `bin/post-plugin-add`, `bin/post-plugin-update`,
  `bin/pre-plugin-remove`; inputs via env vars `ASDF_INSTALL_TYPE`,
  `ASDF_INSTALL_VERSION`, `ASDF_INSTALL_PATH`, `ASDF_DOWNLOAD_PATH`
  **[docs](https://asdf-vm.com/plugins/create.html)**.

## mise plugins

- Built-in backends install most tools without plugins (`github:owner/repo`,
  `npm:prettier`); plugins are directories of Lua scripts (vfox-compatible
  hooks) for tools, backends (`"my-backend:some-tool"`), environment, or package
  managers; asdf shell-script plugins supported as legacy (Linux/macOS only);
  signed "packslip" archives; name derived from URL dropping `asdf-`, `mise-`,
  `vfox-` **[docs](https://mise.jdx.dev/plugins.html)**.

## Neovim remote plugins

- Coprocesses connected over msgpack-RPC; language "plugin hosts" (e.g.
  pynvim) load plugins from `rplugin/{host}/` on 'runtimepath'; hosts start
  lazily when a plugin function is first needed; `:UpdateRemotePlugins`
  generates a manifest of `remote#host#RegisterPlugin` declarations so
  commands/functions exist before the host starts
  **[docs](https://github.com/neovim/neovim/blob/master/runtime/doc/remote_plugin.txt)**.

## Conventions observed in the surveyed agent tools

- PATH-named executables: Herdr plugins are "any argv command" declared in
  `herdr-plugin.toml`; agents call back through `$HERDR_BIN_PATH`
  **[docs](https://herdr.dev/docs/plugins/)**.
- Env-var handshakes: `HERDR_ENV`, `HERDR_PANE_ID`, `HERDR_SOCKET_PATH`;
  `CMUX_SOCKET_PATH`, `CMUX_SURFACE_ID`; `CLAUDE_CODE_MESSAGING_SOCKET`,
  `CLAUDE_CODE_MESSAGING_TOKEN`, `CLAUDE_JOB_DIR`; `AGENT_DECK_SESSION_ID`;
  `CCMANAGER_*` hook vars **[docs](https://herdr.dev/docs/add-herdr-support/)**,
  **[docs](https://cmux.com/docs/agent-integrations/claude-code-teams)**,
  **[docs](https://code.claude.com/docs/en/cross-session-messaging.md)**.
- Shims on PATH: cmux's fake `tmux` translates Claude Code agent-team tmux
  calls into its socket API **[docs](https://cmux.com/docs/agent-integrations/claude-code-teams)**.
- Hook installation into each harness's config (`~/.claude/settings.json`,
  `~/.codex/hooks.json`, `~/.pi/agent/extensions/`, `~/.copilot/hooks/`,
  `~/.gemini/config/hooks.json`) by Herdr, agent-deck, workmux, cmux, Emdash,
  Paradev **[docs](https://herdr.dev/docs/integrations/)**,
  **[docs](https://github.com/raine/workmux#setup)**.
- Harness plugin marketplaces as distribution: workmux status plugin, Warp
  plugins, CCManager config plugin, all installable with `claude plugin
  install` / `codex plugin add` **[docs](https://docs.warp.dev/_llms-txt/agents.txt)**.
- Skills (`SKILL.md`, `npx skills add ...`) as the way to teach an agent a
  manager's CLI: Herdr, cmux, agent-deck, Paradev, workmux, Sculptor
  **[docs](https://herdr.dev/docs/agent-skill/)**,
  **[docs](https://cmux.com/docs/skills)**.

---
# Part 7. Projects looked at and judged not notable

Each was found during the survey and skipped for the reason given (stars and
push dates from the GitHub API on 2026-10-30).

- **coplane/par** (142 stars, last push 2026-02-19): CLI worktree + session
  manager; small and inactive for eight months
  **[source](https://github.com/coplane/par)**.
- **mteam88/sir** (8), **gedeagas/braid** (12), **GhostlyGawd/grove** (1),
  **picktrace/agent-crew** (1), **akatz-ai/meow** (4),
  **warpforgehq/warpforge** (40), **getspur/SpurCode** (3; listed as an ACP
  client): worktree + agent wrappers with little adoption
  **[source](https://github.com/mteam88/sir)**,
  **[source](https://github.com/getspur/SpurCode)**.
- **kivo360/OmoiOS** (81): spec-to-PR agent swarm; low adoption
  **[source](https://github.com/kivo360/OmoiOS)**.
- **nornzach/oh-my-pi-gui** (119): GUI for one agent (omp)
  **[source](https://github.com/nornzach/oh-my-pi-gui)**.
- **MarkShawn2020/claude-code-manager** (90) and several other
  "claude-code-manager" repos (≤35 stars): config/session viewers for Claude
  Code only **[source](https://github.com/MarkShawn2020/claude-code-manager)**.
- **minhtran3124/herdr-worktree-agents** (7): a Herdr plugin (worktree panel
  with agent state, PR/CI); cited under Herdr as evidence of the plugin
  ecosystem **[source](https://github.com/minhtran3124/herdr-worktree-agents)**.
- Forks and mirrors: many `herdr`, `cmux`, and `acpx` copies; `janitrai/acpx`,
  `imWildCat/acpx`, and `imumesh18/acpx` (unrelated Rust crate) share the acpx
  name; `itzptk/t3-code` is a copy of T3 Code
  **[docs](https://docs.rs/crate/acpx/latest/source/README.md)**.
- `stanford-oval/storm` appeared in a metadata batch; it is a research-report
  generator, unrelated **[source](https://github.com/stanford-oval/storm)**.

Mentioned in sources but not researched individually: JetBrains Air (has its
own ACP registry index), Kiro, Amp, Roo Code's cloud pivot, Charlie, Tembo,
Cosine, Sweep, Replit Agent, Amazon Q/Kiro autonomous agent, Microsoft
Intelligent Terminal, Exo for VS Code, necoder
**[docs](https://github.com/agentclientprotocol/registry)**,
**[docs](https://agentclientprotocol.com/get-started/clients.md)**.

---
# Feature inventory

"LT" counts how many of the twelve local terminal tools have the feature:
Herdr, agent-deck, Claude Squad, CCManager, cmux, Uzi, Toad, acpx, workmux,
container-use, Claude Code Agent Farm, and the Claude Code CLI (agent view,
teams, worktrees, messaging). The "Projects" column lists every surveyed
project with the feature, per the sections above. Sources are in each
project's section.

## Creating and organizing agents

| Feature | LT | Projects |
| --- | --- | --- |
| Run any agent CLI in a real PTY/terminal pane | 8 | Herdr, agent-deck, Claude Squad, CCManager, cmux, Uzi, workmux, Agent Farm, Paradev, Emdash, Parallel Code, Sculptor (any terminal agent), Warp, Zed Terminal Threads |
| Drive harnesses through structured protocols (stream-json, SDK, app-server, ACP, RPC) instead of the screen | 2 | Toad, acpx, Vibe Kanban, Nimbalyst, Sculptor, Paseo, Happy, T3 Code, Zed, Conductor [inferred] |
| ACP client | 2 | Toad, acpx, Vibe Kanban (Gemini, generic), Nimbalyst (Codex), Paseo (catalog), Happy, Zed, JetBrains, VS Code extensions, Neovim/Emacs plugins |
| Exposes itself as an ACP agent | 0 | Xum (`mux acp`), Devin CLI, Factory Droid, OpenClaw |
| Own agent loop rather than wrapping CLIs | 1 | Claude Code (its multi-agent features run Claude sessions only), Xum, Warp Agent, Cursor, Antigravity, Devin, Jules, Factory, OpenHands, Omnara |
| Add any custom agent command | 6 | Claude Squad (profiles), agent-deck (`[tools.*]`), CCManager (presets), acpx (`--agent`), Uzi, workmux (`agent`), Paradev (custom agent), Zed (custom `agent_servers`), Paseo (custom providers) |
| Per-harness launch settings (resume args, fork args, prompt transport) | 1 | CCManager (primary/fallback args), Paradev |
| Fan out one prompt to N agents or N agent types | 3 | Uzi (`claude:3,codex:2`), workmux (`-a`, `-n`, `--foreach`, stdin), acpx (`compare`, serial), Parallel Code (arena), Cursor (`/best-of-n`) |
| Concurrency cap / worker pool | 1 | workmux (`--max-concurrent`) |
| Workspace / tab / pane hierarchy | 3 | Herdr, cmux, workmux (tmux windows/panes) |
| Groups of sessions (folders, pinned, collapsible) | 4 | agent-deck, cmux, Herdr (worktree groups), Claude Code (state/directory groups, pin) |
| Multi-project / multi-repo view | 5 | Herdr, agent-deck, CCManager (`--multi-project`), cmux, Claude Code (`claude agents` across repos), Vibe Kanban, Conductor, Emdash, Paradev, VS Code, Cursor |
| Multi-repo single workspace | 0 | Vibe Kanban, Cursor Cloud, Claude Code cloud, Conductor (cloud commits across repos) |
| Session naming and rename | 4 | Herdr, agent-deck, cmux, Claude Code, Conductor, Paradev |
| Archive / unarchive sessions | 1 | agent-deck, Conductor, Claude Desktop, GitHub Copilot, Codex app-server |
| Undo delete | 1 | agent-deck (`Ctrl+Z`) |
| Fork a session's conversation | 2 | agent-deck (native fork per tool), Claude Code (`/fork`), Paradev (fork args), opcode (checkpoint fork), Codex (`thread/fork`) |
| Carry conversation/session data into a new worktree | 2 | CCManager (copies `~/.claude/projects`), agent-deck (fork with state) |
| Agent profiles (model, mode, permissions presets) | 1 | Vibe Kanban, Paseo ("When to use" notes), Warp, CCManager (presets) |
| Per-group or per-profile account/config dir | 1 | agent-deck (`CLAUDE_CONFIG_DIR` per group/profile, account switch) |
| Run plain shell jobs as rows | 1 | Claude Code (`--bg --exec`) |

## Isolation and environments

| Feature | LT | Projects |
| --- | --- | --- |
| Git worktree per agent created automatically | 7 | Claude Squad, CCManager, Uzi, workmux, agent-deck, Herdr (sidebar action), Claude Code (`--worktree`, background isolation), Vibe Kanban, Conductor, Crystal/Nimbalyst, Sculptor, Paradev, Emdash, Xum, Paseo, Parallel Code, Codex app, Cursor, VS Code, Claude Desktop |
| Several agents sharing one workspace by design | 1 | Claude Code (agent teams), Conductor (tabs), Sculptor (agent tabs), Vibe Kanban (sessions) |
| Copy gitignored files (`.worktreeinclude`) or symlink deps | 4 | CCManager, agent-deck, workmux, Claude Code, Codex app, Conductor, Parallel Code, Claude Desktop |
| Worktree setup/teardown scripts or hooks | 3 | CCManager, agent-deck (approved scripts), workmux, Conductor, Paradev, Sculptor, Cursor (`worktrees.json`), Codex (local environments) |
| Sparse checkout / bare repo layouts | 1 | agent-deck |
| jj (Jujutsu) support | 1 | agent-deck (fork into jj workspace) |
| Branch from a PR or issue | 2 | Claude Code (`--worktree "#1234"`), workmux (PR checkout), Conductor, VS Code |
| Clone or in-place modes besides worktree | 0 | Sculptor |
| Container or VM sandbox per agent | 4 | agent-deck (Docker), CCManager (devcontainer), workmux (Docker/Podman/Apple Container/Lima), container-use, Parallel Code, Sculptor (container backend), all cloud platforms |
| Network egress allowlist for sandbox | 1 | workmux (container), Cursor Cloud, Codex Cloud |
| Secrets hidden from the model | 1 | container-use |
| Per-agent dev server port allocation | 2 | Uzi (`portRange`), Conductor (`$CONDUCTOR_PORT`), Paradev (port detection), cmux (ports in sidebar) |
| Remote machines over SSH | 3 | Herdr, agent-deck, cmux, Emdash, Xum, Paradev, Paseo, Cursor, VS Code, Claude Desktop |
| One view across several machines | 3 | Herdr, agent-deck, cmux TUI, Paseo |
| Managed cloud VMs offered by the tool | 1 | cmux Cloud, Conductor Cloud, Paradev, Cursor, Claude Code cloud, Codex Cloud, Copilot, Jules, Devin, Factory, Terragon, OpenHands, Ona, Warp |
| Self-hosted execution for a hosted agent | 0 | Cursor Self-Hosted Machines, Devin Outposts, Ona (VPC), Omnara |
| Move a session between local and cloud | 1 | Claude Code (`--teleport`, Desktop "Open in Cloud"), Cursor, Codex app (Local↔Worktree handoff), Conductor, GitHub Copilot, Warp |

## Watching agents

| Feature | LT | Projects |
| --- | --- | --- |
| Per-agent state (working/blocked/idle) | 9 | Herdr, agent-deck, CCManager, cmux, Uzi, workmux, Claude Code, Claude Squad (prompt detection), Agent Farm, Paradev, Emdash, Parallel Code, Conductor, Vibe Kanban, Paseo, Warp, Copilot |
| State from screen scraping | 6 | Herdr (manifests), Claude Squad, CCManager, Uzi, Agent Farm, agent-deck (fallback) |
| State from hooks installed in the harness | 4 | Herdr, agent-deck, workmux, cmux, Emdash, Paradev |
| Agents report their own state to the manager | 2 | Herdr (`pane report-agent`), cmux (`cmux notify`, OSC 9/99/777) |
| Remotely updated detection rules | 1 | Herdr |
| Explain why a state was chosen | 1 | Herdr (`agent explain`) |
| "Done but not yet seen" state | 4 | Herdr, workmux, cmux (unread), Claude Code (`← 2 done`) |
| State rolled up to tab/workspace/window title | 3 | Herdr, cmux, Claude Code (tab title count) |
| LLM-written one-line status per agent | 1 | Claude Code (Haiku-class summaries) |
| Desktop/OS notifications | 5 | Herdr, cmux, agent-deck (web push), Claude Code, CCManager (via hooks), Paradev, Warp, Claude Desktop, Terragon |
| Notification hooks or custom notification command | 2 | CCManager (status hooks), cmux (JSON hooks, command) |
| Sound on completion | 1 | Herdr, Paradev |
| Live preview of agent output in a list | 3 | Claude Squad, workmux (dashboard), Claude Code (peek) |
| Kanban or board of sessions by status | 1 | agent-deck (web Fleet board), Vibe Kanban, Nimbalyst, Parallel Code |
| Task list with dependencies shared by agents | 2 | Claude Code (teams), Agent Farm (prompt-driven files), Sculptor (dependency graph), Nimbalyst, Parallel Code (`KANBAN.md`) |
| Transcript read from the manager CLI | 3 | Herdr (`agent read`), Claude Code (`claude logs`), acpx (`sessions history`), Paseo (`attach`) |
| Full-text search across past transcripts | 2 | agent-deck (Recall), cmux (Vault), opcode, Conductor (SQL API) |
| Context-window usage display | 3 | agent-deck (context inspector), Toad, Agent Farm, Sculptor, Conductor, Vibe Kanban |
| Token/cost tracking | 2 | agent-deck, cmux (estimate), Xum, opcode, Conductor (cloud cost), Copilot (token usage), Warp, Devin (ACU) |
| Budget limits with hard stop | 1 | agent-deck |
| Subscription quota display | 2 | agent-deck (`usage`), cmux (CodeRouter `cr`), Codex app-server (`account/rateLimits/read`) |
| Account pooling / failover router | 1 | cmux CodeRouter |
| Process resource monitor | 1 | cmux (`cmux top`) |
| Agent-run command log (what the agent actually did) | 1 | container-use (`log`) |

## Messaging and control

| Feature | LT | Projects |
| --- | --- | --- |
| Attach to the agent's own TUI | 7 | Herdr, agent-deck, Claude Squad, CCManager, workmux, Claude Code (`attach`), cmux |
| Send a prompt from the manager without attaching | 6 | Herdr (`agent prompt`), agent-deck (`session send`), Uzi (`broadcast`), acpx, Claude Code (peek reply), cmux (`send`), Paseo, Conductor API, Cursor API |
| Queue a message until the turn ends | 3 | agent-deck, Claude Code, acpx, Sculptor (editable queue) |
| Editable visible message queue | 0 | Sculptor |
| Broadcast one message to all agents | 2 | Uzi (`broadcast`, `run`), Agent Farm (`/clear` to all) |
| Steer mid-turn | 1 | Claude Code (messages between tool calls), Copilot, Cursor API, Codex app-server (`turn/steer`) |
| Cancel/interrupt from outside | 3 | acpx (`cancel`), Claude Code (`/stop`, `claude stop`), Toad, Conductor API, Codex (`turn/interrupt`) |
| Switch mode/model of a running session | 3 | acpx (`set-mode`, `set model`), Toad, Claude Code (peek `/model`), Conductor, Sculptor, Paradev |
| Agent-to-agent messaging | 3 | Herdr, agent-deck, Claude Code (teams, cross-session), Paseo, Devin |
| Wait until another agent is idle/blocked | 3 | Herdr (`agent wait`), Claude Code (`notify_when_idle`), Paseo (`waitForFinish`), workmux (`--wait`) |
| Supervisor/conductor agent managing workers | 3 | agent-deck (conductor), Claude Code (team lead), workmux (`/worktree` skill), Paseo, Devin (managed Devins), Claude Projects |
| Answer approvals from the manager UI | 3 | cmux (Feed Allow/Deny), Toad, acpx (policy/callback), Vibe Kanban, Sculptor, Paradev chat, Happy |
| Auto-approve prompts | 4 | Claude Squad (`--autoyes`), Uzi (`auto`), CCManager, Agent Farm (`--dangerously-skip-permissions`) |
| LLM-judged auto-approval | 1 | CCManager |
| Per-tool permission policy with escalation | 2 | acpx, Claude Code (permission rules) |
| Plan approval step | 1 | Claude Code (teams), Jules (`requirePlanApproval`), Conductor (Plan Mode), Sculptor |
| Replace harness question/plan tools with manager UI | 0 | Sculptor |
| Remote control from phone | 3 | cmux (iOS), agent-deck (Telegram/Slack/web), Claude Code (Remote Control), Happy, Paseo, Parallel Code, Nimbalyst, Conductor, Paradev, T3 Code, Vibe Kanban, Warp, Cursor, Copilot |
| Chat-app bridges (Telegram, Slack, Discord) | 2 | agent-deck, Claude Code (channels), Cursor, Devin, Terragon, Ona |
| Web UI | 2 | agent-deck, Toad (`serve`), Vibe Kanban, Paseo, CloudCLI, T3 Code, Happy |
| Voice input | 1 | Claude Code (dictation), Paseo, Nimbalyst (iOS) |
| Multiplayer shared sessions | 0 | Conductor, Happy, Claude Code cloud (share), Warp, Copilot |

## Reviewing and merging

| Feature | LT | Projects |
| --- | --- | --- |
| Diff view in the tool | 4 | Claude Squad, workmux, Toad, container-use (`diff`), Vibe Kanban, Conductor, Nimbalyst, Sculptor, Paradev, Emdash, Parallel Code, Xum, Codex app, Cursor, Claude Code web/Desktop |
| Inline diff comments sent back to the agent | 0 | Vibe Kanban, Conductor, Paradev, Parallel Code, Claude Code web |
| Independent reviewer agent/model | 0 | Parallel Code (confidence ratings), Conductor (Review action), Cursor Bugbot, Devin Review, Codex code review |
| Hunk-level stage or accept/reject | 1 | workmux (patch mode), Nimbalyst |
| Commit from the tool | 3 | Claude Squad, workmux (send commit), Claude Code (bg sessions commit), Sculptor, Nimbalyst, Paradev, Vibe Kanban |
| Open a PR from the tool | 2 | Claude Squad (push), Claude Code (draft PR), Vibe Kanban, Conductor, Sculptor, Paradev, Emdash, Codex, Cursor, Copilot, Terragon |
| PR/CI status shown per agent | 3 | cmux, Claude Code (`#N` colors), workmux (`list --pr`), Conductor (Checks), Sculptor, Paradev, Emdash, Claude Desktop |
| Merge gate on failed checks or todos | 0 | Conductor |
| Merge branch from the tool | 4 | workmux, agent-deck (`worktree finish`), CCManager, Uzi (`checkpoint`), Conductor, Paradev, Parallel Code, Emdash, Crystal |
| Cleanup after merge (remove gone worktrees, auto-archive) | 1 | workmux (`remove --gone`), Claude Desktop |
| Per-turn checkpoints with revert | 0 | Conductor, opcode, Crystal (commit per iteration), Codex (`thread/rollback`) |
| Embedded browser preview | 1 | cmux, Vibe Kanban, Paradev, Paseo, Parallel Code, Antigravity, CloudCLI |
| Integrated file editor | 0 | Nimbalyst, CloudCLI, Claude Desktop, Paseo |
| Visual non-code editors (mockups, diagrams) | 0 | Nimbalyst |
| Rich tool-call cards | 1 | Toad, Sculptor, Vibe Kanban |
| Spec → plan → build → review pipeline | 0 | Sculptor, Nimbalyst |

## Persistence, scheduling, triggers

| Feature | LT | Projects |
| --- | --- | --- |
| Agents keep running with no client attached | 8 | Herdr, agent-deck, Claude Squad, Uzi, workmux, Agent Farm, Claude Code (supervisor), cmux (opt-in tmux), Paseo, Happy |
| Restore layout after restart | 2 | Herdr, cmux |
| Resume agent conversations after manager restart | 7 | Herdr, cmux, agent-deck, workmux (`resurrect`), Toad, acpx, Claude Code (respawn), Paradev |
| Live process handoff across manager upgrade | 1 | Herdr |
| Recurring/scheduled agent runs | 2 | Claude Code (`/loop`, scheduled tasks), agent-deck (heartbeat), Conductor, Cursor, Paradev, Paseo, Codex app, Devin, Terragon, VS Code, Warp |
| Event triggers (webhooks, GitHub, Slack) | 1 | agent-deck (watchers), Claude Code cloud, Conductor, Cursor, Paradev, Terragon, Warp |
| Issue tracker tasks routed to agents | 0 | Conductor, Emdash, Paradev, Cursor, Copilot, Devin, Terragon, Ona |
| Kanban issue planning | 0 | Vibe Kanban, Nimbalyst, Parallel Code |

## Extension points

| Feature | LT | Projects |
| --- | --- | --- |
| Scriptable CLI | 9 | Herdr, agent-deck, cmux, workmux, Uzi, acpx, container-use, Claude Code, Paradev, Paseo, Conductor, Claude Squad (limited) |
| Local socket or HTTP API | 4 | Herdr (socket), cmux (socket), agent-deck (HTTP/WS), Claude Code (inbox socket), Paseo (WS), Conductor (HTTP), Cursor, Devin, Jules, Factory, Codex app-server |
| Published API schema | 1 | Herdr (JSON Schema), Conductor (OpenAPI), Factory (OpenAPI) |
| Event subscription stream | 2 | Herdr (`events.subscribe`), cmux TUI, Paseo, Codex app-server, Omnara |
| Language SDK | 2 | cmux TUI (5 languages), acpx (`acpx/runtime`), Paradev, Paseo, Codex, Factory, Omnara |
| Plugin system | 3 | Herdr, cmux (Dock, custom sidebars), Paseo, CloudCLI, Nimbalyst, Claude Code (mods, plugins) |
| MCP server exposing the manager | 1 | container-use, Vibe Kanban, Conductor, Paradev, Paseo, Devin, Terragon |
| Skill teaching agents the manager's CLI | 5 | Herdr, cmux, agent-deck, workmux, CCManager (config skill), Paradev, Sculptor |
| Status/lifecycle hooks for user scripts | 5 | CCManager, cmux, workmux (lifecycle), Herdr (plugin events), Claude Code (hooks) |
| Config reload without restart | 3 | Herdr, cmux, CCManager |
| MCP management per session | 1 | agent-deck, opcode, Vibe Kanban, Cursor |
| Skills management per project | 1 | agent-deck |
| Opt-in or opt-out telemetry | 1 | agent-deck, Nimbalyst, Emdash |

## Features fewest local terminal tools have

Ten features with LT = 0 in the tables above, each present elsewhere:

1. Inline diff comments sent back to the agent: Vibe Kanban, Conductor,
   Paradev, Parallel Code, Claude Code web.
2. Independent reviewer agent or second model grading the diff: Parallel Code,
   Conductor, Cursor Bugbot, Devin Review, Codex code review.
3. Merge gate on failed checks or open todos: Conductor.
4. Per-turn checkpoints with revert: Conductor, opcode, Crystal, Codex
   app-server.
5. Multiplayer shared live sessions: Conductor, Happy, Claude Code cloud,
   Warp, Copilot.
6. Issue-tracker tasks routed to agents (Linear, GitHub, Jira, ...): Conductor,
   Emdash, Paradev, Cursor, Copilot, Devin, Terragon, Ona.
7. Integrated file editor: Nimbalyst, CloudCLI, Claude Desktop, Paseo.
8. Editable visible message queue: Sculptor.
9. Replacing the harness's own question/plan tools with manager UI: Sculptor.
10. Spec → plan → build → review pipeline: Sculptor, Nimbalyst.

Other LT = 0 rows: kanban issue planning, visual non-code editors, clone or
in-place workspace modes, multi-repo single workspace, self-hosted execution
for a hosted agent, exposing the manager as an ACP agent.

## What could not be determined

- How Conductor drives its bundled Claude Code, Codex, and OpenCode binaries
  (protocol undocumented) **[inferred]**.
- Google Antigravity's current Agent Manager documentation (official page
  returned 404); facts come from Google's launch blog
  **[docs](https://developers.googleblog.com/en/build-with-google-antigravity-our-new-agentic-development-platform/)**.
- Factory "Missions" headless and parallel behavior (only third-party
  descriptions found) **[docs](https://docs.factory.com/llms.txt)**.
- Devin REST API details beyond the MCP server and docs index; OpenHands Cloud
  parallel-conversation behavior; Ona reference docs beyond marketing pages.
- Cursor Agents Window internals (how local agents are run and observed).
- Adoption figures for closed products other than vendor claims (Conductor
  "100k+ builders").
