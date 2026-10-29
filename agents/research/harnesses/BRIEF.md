# Harness research brief

Hypercube runs coding-agent harnesses (Claude Code, OpenCode, ...) as agents in PTYs: a lead per session plus workers it starts, all able to message each other. We are designing a harness plugin interface so Hypercube has no hard dependency on any harness. We need facts about what each harness exposes for automation and integration.

Read first (read-only, do not edit): ~/projects/hypercube/docs/teamwork.md and ~/projects/hypercube/docs/harness-support.md. They describe how the existing (Rust) Hypercube does message passing between agents and what it needs from each harness. Research with that use case in mind.

For each harness assigned to you, write agents/research/harnesses/<harness-id>.md with exactly these sections. Be concrete: exact flags, env vars, config keys, event names, file paths. Mark every fact [verified] (you ran it locally) or [docs] (with the source URL). Write "unknown" rather than guess.

1. Overview: what it is, vendor, open source or not, license, current version, install method, rough popularity/adoption signal.
2. Interactive mode in a PTY: is it a full-screen TUI; anything that breaks when run under a web terminal (xterm.js); resize behavior.
3. Non-interactive/headless and server modes: print/exec modes, JSON/stream output, a server or API mode, and whether it supports the Agent Client Protocol (ACP) natively or via an adapter.
4. Conversation identity: start with a caller-chosen id, learn the id it picked, resume a specific conversation by id, where conversations are stored, what happens with several instances in one directory.
5. Hooks and events: lifecycle events it emits (session start, prompt submitted, turn finished, waiting for user / permission prompt, tool use, session end), how they're configured, and whether they can be injected PER PROCESS (CLI flag or env var) without editing the user's config files.
6. Plugins/extensions: plugin or extension API, what plugins can do, how they're loaded, whether loadable per process.
7. MCP: client support, transports, per-process MCP config.
8. Instructions and customization: per-process system prompt / appended instructions, custom commands, agents/subagents, model and provider selection, permission modes.
9. Getting messages INTO a running interactive session from outside: any API, hook that can inject context (e.g. a hook that blocks turn end and feeds text back), stdin tricks, or only typing into the PTY.
10. Status detection from outside: busy / idle / waiting for input / waiting for permission. Which events or signals expose it.
11. Usage and cost reporting.
12. Gotchas for running many instances side by side (shared state, locks, ports, auth).
13. Fit for Hypercube: 3-5 bullets on how it would map onto a harness plugin (launch, conversation tracking, status, message delivery) and what's missing.

Rules: do not install anything or change any global config. For installed harnesses you may run non-model commands (--help, --version, listing config); avoid model calls. Use web search / official docs / source repos for the rest. Write only your own files in agents/research/harnesses/. When done, send the orchestrator a 3-line summary per harness: biggest strengths and the biggest gap for Hypercube.
