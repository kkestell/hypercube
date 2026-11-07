# OpenCode corrections during active work

## Required outcome

Send follow-up messages into the active OpenCode conversation at its next
native step boundary. Do not cancel model requests, tools, questions, or the
session to deliver a correction. Keep explicit Stop as cancellation.

## Evidence and choice

Hypercube's ACP adapter queues busy input until `session/prompt` returns.
Installed OpenCode 2.1.4 rejects overlapping ACP prompts. Its native HTTP API
accepts `POST /api/session/{id}/prompt` with `delivery: "steer"` during work.
A local fake-provider probe admitted a correction during a shell call; the shell
finished successfully and the next model request contained its result and the
correction, without an interrupted execution event.

Replace ACP with one owned `opencode serve --stdio --port 0` process and its
native authenticated HTTP API and event stream. Keep the existing plugin
protocol and core unchanged. Using ACP plus a second input connection would
leave two incompatible owners of turn completion and risks missing output
when steering starts a new execution just as ACP finishes. A JavaScript bridge
would add another transport and plugin lifecycle without removing that problem.

## Implementation

- Launch a private server with a generated password, inherited user environment,
  and per-launch configuration only. Read its JSON readiness URL from stdout;
  close stdin to stop it. Never use or restart the user's shared service.
- Preserve native conversation IDs, resume failures, independent same-folder
  forks, instructions, MCP environment, selected model/effort, image input,
  transcript text/reasoning/tools/images, questions, permissions, usage, errors,
  and explicit Stop. Keep existing cross-folder fork restriction.
- Subscribe before accepting input. Translate only the main session's events;
  native child output stays excluded as today. Native execution events own
  busy/idle and successful final responses; native inbox events expose pending
  accepted input. No Hypercube prompt queue or prompt-response goroutine.
- Submit every input with native steer delivery and acknowledge admission,
  not execution completion. Treat a failed event stream as a clear plugin
  failure rather than silently losing transcript output or adding recovery.
- Read models and variants directly for discovery and configuration; no
  temporary native discovery conversation is needed. Preserve saved `default`
  effort behavior and report the accepted selection.
- Replace ACP-only tests and fixtures with native protocol behavior tests.
  Cover steering while a tool runs, multiple corrections, input admission
  failure, explicit Stop, event ordering and failure, main/child filtering,
  final answers, model settings, resume/fork, questions, images, and startup.

## Validation and delivery

Run focused race tests, a deterministic real-OpenCode/fake-provider integration
test demonstrating uncancelled tool completion and corrected next request, and
`make check`. Inspect the entire resulting plugin. Do not commit or install
without the user asking. Active sessions require restart after installation.
