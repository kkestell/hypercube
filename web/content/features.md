+++
title = 'Features'
toc = false
+++

Sessions:

- Saved sessions that can be reopened, resumed, and managed.
- Recover a turn paused on a permission request.
- Cancel in-progress model, tool, or shell work.
- Run turns in separate sessions concurrently.
- Keep long sessions within the model's context limits.

Modes and models:

- Plan, code, and autonomous modes.
- Choose a configured model and reasoning level for each session.
- Configure provider routing, sampling, and output limits per model.

Coding tools:

- Read, search, edit, and create files in the workspace.
- Run shell commands.
- Query configured language servers for definitions, references, symbols, and
  diagnostics.
- Ask clarifying questions, track tasks, and delegate bounded tasks to parallel
  subagents.
- Fetch public web pages and store workspace memory.

Permissions and safety:

- Review permission prompts before tool calls run, or use autonomous mode to run
  without prompts.
- Reuse approved tool calls within a session.
- Keep file access inside the workspace.
- Restrict tools available in plan mode.

Extensibility:

- Load reusable Agent Skills from the workspace.
- Connect client-configured MCP servers over stdio and Streamable HTTP.
- Use search tools supplied by an MCP server.

Configuration and diagnostics:

- Store credentials in the OS keyring or a private local file with `ox login`.
- Configure Taikonaut globally or per workspace.
- Write an optional diagnostic JSONL trace.
