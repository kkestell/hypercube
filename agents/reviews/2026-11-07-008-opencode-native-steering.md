# OpenCode native steering review

## Outcome

One independent review by `steering-review` (Codex, GPT 6.1 Terra, high effort)
found no confirmed issues. The reviewer changed no files. The parent assessed
the result; no findings remain open.

## Scope and assessment

Reviewed the resulting OpenCode adapter and tests against
`agents/plans/2026-11-07-005-opencode-native-steering.md`: input admission,
native event decoding, execution state, questions and permissions, process
ownership, configuration, transcript output, and preservation of normal behavior.

The single native connection removes ACP's overlapping-prompt restriction and
the adapter's turn queue. Input and transcript completion now follow the same
native execution. There is no cancellation fallback, second input transport,
JavaScript bridge, or transcript recovery subsystem.

The reviewer also checked a concrete startup uncertainty with installed
OpenCode 2.1.4 and isolated temporary state: a fresh session preserved a supplied
symlinked working directory, so no path-normalization fix was warranted.

## Validation

- `make check` passed: website and application builds, TypeScript checks,
  345 browser-view tests, 28 desktop tests, Go vet, formatting, and the Go race
  suite. Browser tests emitted React `act(...)` warnings but had no failures.
- `HYPERCUBE_TEST_OPENCODE=1 go test -race ./plugins/opencode -count=1`
  passed with installed OpenCode 2.1.4, an isolated native database, a local
  deterministic provider, and a real blocking shell tool. Both corrections and
  the successful tool result reached the next model request. Explicit Stop
  cancelled a second real tool.
- Question preservation during steering is tested at the adapter boundary,
  not with a model-driven native question. Native form creation/reply payloads
  were separately checked against the installed server.

The reviewer did not repeat the full test suite. No installation, service
restart, session restart, or merge was performed.
