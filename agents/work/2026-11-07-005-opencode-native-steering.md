# OpenCode native steering

Implemented `agents/plans/2026-11-07-005-opencode-native-steering.md`.

- Replaced the ACP client and its busy-input queue with one owned native
  `opencode serve --stdio --port 0` process, authenticated HTTP requests, and
  its event stream. Every prompt uses native `delivery: "steer"`.
- Preserved explicit Stop, native conversation resume and same-folder forks,
  model and effort selection, instructions and MCP environment, slash commands,
  image input/output, transcript rendering, usage, questions, and permissions.
  Native child questions and permissions remain answerable without mixing
  child transcript output into the main conversation.
- Discovery reads the native catalog without creating a conversation. Startup
  observes plugin activation rather than adding a timing delay. A lost event
  stream fails the plugin clearly instead of silently losing transcript output.
- Removed obsolete ACP tests and recordings. Added native boundary tests and an
  opt-in, deterministic real-OpenCode test for corrections during an actual
  blocked tool and explicit Stop.

During validation, an initial native probe exposed an object-valued `delta` in
an unrelated instructions event. The decoder now selects consumed event types
before decoding their payloads; a regression test covers that case.

Full `make check` and the real-OpenCode race tests passed. Independent review
found no confirmed issues; see
`agents/reviews/2026-11-07-008-opencode-native-steering.md`.
User guide changes belong to the separate documentation task. Nothing was
installed or restarted; existing sessions need a restart after installation
to load the new plugin.
