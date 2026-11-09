# Harness plugins

## Plan

`agents/plans/2026-10-30-001-harness-plugins.md`

## Summary

Claude Code and OpenCode support moved into `plugins/claude` and
`plugins/opencode`, built as `hypercube-harness-claude` and
`hypercube-harness-opencode`. The core launches through `<plugin> launch` and
receives `hypercube report` and `hypercube wait`. No file under `cmd/` or
`internal/` names a harness. The goal is met. The harness code is about 970
lines (harness 205, report 209, wait 96, Claude plugin 239, OpenCode plugin
108 plus 115 of JavaScript) against about 750 before; the extra is each
plugin's own command-line handling and JSON types.

## Departures from the plan

- `hypercube report` outside an agent writes an empty response instead of
  nothing, so every report gets one response object, as the plan's protocol
  says.
- Test fixtures in `internal/harness` and `internal/mcp` used `claude` and
  `opencode` as harness names; they now use neutral names so the core names no
  harness, tests included.

## Decisions

- The Claude plugin quotes every hook command argument, including the
  subcommand and event name.
- `make install` and `scripts/reinstall` install every `bin/hypercube*`;
  `scripts/dev` builds with `make build` so the dev binary has its plugins
  beside it.

## Checks run

- `make check` — passed.
- `grep -rn -i 'claude\|opencode' cmd internal` — no matches.
- `node --check` on `opencode.js` with the path substituted — passed.

## Manual verification

1. The built Claude plugin against the real `hypercube`, with a temporary
   state holding a worker with a queued message:

   ```sh
   make build
   echo '{"session_id":"s9"}' | bin/hypercube-harness-claude hook "$PWD/bin/hypercube" UserPromptSubmit
   echo '{}' | bin/hypercube-harness-claude hook "$PWD/bin/hypercube" Stop
   echo '{}' | bin/hypercube-harness-claude wait "$PWD/bin/hypercube"
   ```

   The prompt hook returned the message as `additionalContext` and recorded
   session `s9`; `Stop` with an empty queue wrote nothing and marked the
   worker idle; after queuing another message, `wait` printed it on stderr and
   exited 2. With `{"agent_id":"x"}`, `wait` exited 0 at once.

2. The built OpenCode plugin's launch printed today's argv and environment
   and wrote `opencode-plugin/index.js` calling `report`.

## Follow-up work

- No live-model smoke test (`make dev`) was run; it needs Ghostty and real
  Claude Code and OpenCode sessions.
