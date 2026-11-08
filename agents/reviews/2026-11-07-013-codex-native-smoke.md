# Codex native smoke tests

## Scope and coverage

Reviewed the complete staged `plugins/codex/native_test.go`,
`native_provider_test.go`, and `native_stream_test.go`, plus the public serve
test client, plugin command loop, native input handling, event translation,
and process cleanup that determine the tests' behavior. No plan was required
for this localized test addition.

The installed Codex app-server runs through public `serve` with isolated
`CODEX_HOME`, temporary working directories, a local deterministic Responses
provider, and its normal tool sandbox. The tests cover steering before model
text starts, steering during an observed text stream, two corrections during a
real blocking shell tool, completion followed by idle input, and explicit Stop.
They check native turn continuity, correction and tool-result delivery, final
answer translation, and the shell's continued operation or cancellation.

The model and MCP server are fixtures. These tests do not establish behavior
against a remote provider or reproduce every timing in the user's running
conversation. The thinking case holds an open model response before any text;
it does not simulate internal model reasoning. No production behavior changed.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | Medium | Test reliability | Fixed | Waits could miss events consumed before a command reply |

## F1. Waits could miss events consumed before a command reply

Medium · Test reliability · Fixed · `plugins/codex/native_test.go:218`

Command handling records events until the matching reply. A following wait
previously examined only future records, so a streaming delta, Stop completion,
or fast followup completion received before that reply could cause a false
timeout despite correct behavior.

### Evidence

Confirmed by tracing the plugin command loop and native notification handler:
native notifications emit events independently of the goroutine completing an
input request. `nativeProbe.command` retains those events, while
`harnesstest.Serving.Until` only reads subsequent records. The baseline live run
passed; this ordering issue was established by code tracing, not an observed
timeout.

### Resolution

Added a condition wait that checks collected events before reading more.
Lifecycle waits specify the complete expected sequence, so startup Idle cannot
satisfy a later completion wait. The streaming wait also scans collected text
events. Applied these waits to startup, steering completion, Stop, and idle
followup. The change needs no sleeps or production changes.

## Checks run

- `GOCACHE=/tmp/hypercube-00ba4eac-go-cache HYPERCUBE_TEST_CODEX=1 go test
  -race ./plugins/codex -run '^TestNative' -count=1 -v` — passed before and
  after F1 with installed `codex-cli 0.167.0`. Both thinking and streaming
  subcases, blocking-tool steering, idle followup, and Stop passed. The outer
  sandbox was lifted to permit Codex's own sandbox; model requests remained
  local with no paid model calls.
- `gofmt` on the two corrected Go files and `git diff --check` — passed.
- Full `make check` was not repeated by the reviewer; the parent owns final
  validation.

## Verdict

Accept after F1. No findings remain open. The live tests did not reproduce a
native turn cancellation during thinking, streaming, or tool execution.
