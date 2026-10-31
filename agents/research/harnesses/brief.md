# Harness research brief

We are surveying interactive coding-agent harnesses (terminal coding agents
such as Claude Code or Codex) to learn everything each one exposes to a
program that launches it, configures it, observes it, and controls it. The
survey feeds the design of a tool that runs many harnesses side by side, but
do not filter by what you think that tool needs. Inventory everything.

Write one report per harness, `<harness-id>.md`, with exactly these sections.
Be concrete: exact flags, environment variables, config keys, file paths,
event names, payload fields, API methods. Tag every fact **[verified]** (you
ran it locally), **[help]** (seen in local `--help` output), or **[docs]**
with the source URL. Write "unknown" rather than guess. Give the version you
checked.

1. Overview: vendor, license, version, install, release cadence, adoption.
2. Interactive terminal interface: full screen or inline, alternate screen,
   terminal sequences it emits (titles, OSC 9, OSC 9;4 progress, OSC 7501,
   notifications, bell), mouse, resize, known terminal problems.
3. Launch and configuration: every way to configure one process without
   touching the user's files (flags, env, inline config, config roots,
   plugin directories), and the config file layers and their precedence.
4. Models and reasoning: choosing model, provider, effort or variant;
   listing available models; behavior with an unknown model.
5. Conversations: create with a chosen ID, learn the ID, resume, continue,
   fork, list, export, delete, compaction, where they are stored, behavior
   with several instances in one directory.
6. Transcript and structured output: transcript files and format, JSON or
   streaming output, plans or todo lists, diffs, what an outside program can
   read while a session runs.
7. Hooks and events: the full list of events with payload fields and what a
   hook's response can do (add context, block, modify input, decide
   permissions, continue a turn), sync versus async, timeouts, limits on
   output size and loops, and how hooks are configured per process.
8. Plugins and extensions: the full API surface: what a plugin can register
   (tools, commands, UI, providers, agents), what it can observe and
   control, how it is loaded per process.
9. Tools, permissions, and approvals: built-in tools, permission modes,
   allow and deny rules, approval dialogs, how a program can answer an
   approval, sandboxing, questions the agent asks the user and how they can
   be answered.
10. Subagents and delegation: native subagents, background tasks, agent
    teams, how to disable or restrict them.
11. MCP: client support, transports, per-process configuration, resources,
    prompts, elicitation, any way an MCP server can push into a session.
12. Control from outside: server, RPC, SDK, ACP or other protocol modes;
    remote control; attaching a terminal interface to a server; any way to
    send a message, steer a running turn, interrupt, or start a turn in a
    running interactive session.
13. Status from outside: busy, idle, waiting for permission or input, error,
    and which signals expose each.
14. Usage, cost, and limits: tokens, cost, context use, rate limits, how a
    program can read them.
15. Authentication: methods, where credentials live, how to detect a
    logged-out or expired state without a model call.
16. Running many instances: shared state, locks, ports, memory per
    instance, rate limits, known conflicts.
17. Anything else a program driving this harness should know.

Rules: Do not install anything. Do not change any global or project
configuration or credentials. Make no model calls. For installed harnesses
you may run `--version`, `--help` and subcommand help, and read-only listing
commands. Use official docs, source repositories, and changelogs for the
rest; prefer primary sources and read source code where docs are thin.
Write only in your working directory.
