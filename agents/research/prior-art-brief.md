# Prior-art research brief

Survey the tools people use to run several coding agents at once: local
orchestrators, terminal multiplexers and managers built for coding agents,
agent IDEs and desktop apps, hosted or cloud agent platforms, and the
protocols and plugin conventions they use. The survey feeds the design of a
local tool that runs teams of terminal coding agents (Claude Code, Codex,
OpenCode, and others) side by side, but do not judge what that tool should
copy. Inventory what each project does.

Start with these, and add every other notable project you find:
- Local: Herdr, agent-deck, Claude Squad, ccmanager, cmux, Uzi, Vibe Kanban,
  Conductor, Crystal and Nimbalyst, Sculptor, Toad, acpx, Claude Code's own
  multi-agent features (agent teams, `claude agents`, background sessions).
- Hosted: Terragon, OpenAI Codex cloud, GitHub Copilot coding agent, Cursor
  background agents, Google Jules, Devin, Factory, and similar.
- Protocols and conventions: Agent Client Protocol and its clients and
  adapters, Zed external agents, MCP, A2A, Codex app server; executable
  plugin conventions (git and Docker credential helpers, kubectl plugins,
  git subcommands, HashiCorp go-plugin, LSP, asdf and mise, Neovim remote
  plugins).

For each project write a section in `prior-art.md` with:
1. What it is: local or hosted, open or closed, status (active,
   deprecated), adoption signal.
2. Every user-facing feature: how agents are created, organized, watched,
   messaged, reviewed, merged, stopped, and resumed; task boards, diffs,
   pull requests, notifications, cost tracking, scheduling, sandboxing,
   sharing, search, history, configuration, anything else.
3. How it integrates harnesses: built-in code per harness, adapters,
   protocols, config files, hooks installed where, screen reading.
4. How it observes agents (state, transcripts, usage) and controls them
   (prompts, steering, interrupts, approvals, questions).
5. Its extension points: plugins, APIs, CLIs, sockets.
6. Known problems, limits, and complaints from its docs, issues, or users.

Then add a final section, "Feature inventory": a table of every distinct
feature found across all projects, with the projects that have each.

Be concrete and cite sources. Tag every fact **[docs]** or **[source]** with
a URL, or **[inferred]**. Prefer primary sources: docs, READMEs, source
code, changelogs, issues. Do not install anything. Write only in your
working directory.
