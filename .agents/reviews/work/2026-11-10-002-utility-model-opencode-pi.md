# Work review: utility prompts in the OpenCode and Pi plugins

## Round 1

- Task ID: 2026-11-10-002-utility-model-opencode-pi
- Mode: work
- Author: work-reviewer-2, Hypercube child session of 12efdd39 (Claude Code,
  Opus 5.5); native conversation `c2153148-974a-4c8d-bdf7-0971dc153e50`.
  Independent: I did not write the plan or the code.
- Work log (repository-relative): `.agents/work/2026-11-10-002-utility-model-opencode-pi.md`
- Plan (repository-relative): `.agents/plans/2026-11-10-002-utility-model-opencode-pi.md`,
  blob `50ce0817f232f085ae7091b6385ed08d87119990` (checked with
  `git hash-object`; matches the execution decision in
  `.agents/reviews/plans/2026-11-10-002-utility-model-opencode-pi.md`).
- Request: slice 2 of GitHub issue #66.

### Reviewed range

- Code base: `eb718b37025a2e586ecdafd5d8a59290feacc138` (checkout `HEAD`).
- Code head: the uncommitted working tree on the Code base, by user
  instruction. There is no commit anchor; the coordinator commits once.
- Reviewed change: `git diff eb718b3 -- docs plugins` plus the untracked
  `plugins/opencode/generate.go`, `plugins/opencode/generate_test.go`,
  `plugins/pi/generate.go`, `plugins/pi/generate_test.go`.
- Excluded: the three workflow records for this task.
- Fingerprint: SHA-256 of `git diff --binary eb718b3… -- docs plugins`
  followed by each untracked file as `== PATH` and its contents, in the order
  above: `19a3c7a7cf01bf1f8a9bb73f374c4cabbfc9aa323e9eeb61e745f304984f182b`.
  Untracked blobs: `generate.go` (OpenCode) `e9e08ddfb4ff3ed6158132a439019f482caf4964`,
  `generate_test.go` (OpenCode) `1fce2806bd924d8de6bc1aab6a53bd3c908fb2ed`,
  `generate.go` (Pi) `ebfb7d3b98eb69d10fad8800cbe9ca5e211ce79a`,
  `generate_test.go` (Pi) `254bb0fe6d1489ab8093dc8dba03550f691978d5`.

### Verdict

`changes-required`. The plugin code is correct and follows the plan; the
findings are in the tests and the docs table.

### Findings

1. **OpenCode rejection test accepts any error.** Medium.
   `plugins/opencode/generate_test.go:92-110`
   (`TestGenerateRejectsASelectionTheServerDoesNotOffer`). Each case checks only
   `err != nil` and that no generate call was sent. A launch, subscribe or
   catalog failure also satisfies both, so the test passes without reaching
   `choose`. The plan requires the same rules and messages as a session start.
   Correction: give each case its expected text and assert it, for example
   `effort` → `does not offer effort "minimal"`, `missing` →
   `does not offer model "p/missing"`, `none` → `needs an effort`.

2. **Duplicated proxy test helper.** Low.
   `plugins/opencode/generate_test.go:17-37` (`generating`) repeats the password
   proxy in `TestOwnedServerDiscoveryAndLaunch`
   (`plugins/opencode/main_test.go:366-383`) line for line. `development.md`
   asks to remove duplication. Correction: move the helper to `main_test.go`
   with a name that is not generate-specific (for example `owned`), and have
   `TestOwnedServerDiscoveryAndLaunch` call it.

3. **Success case inside the Pi failure table, selected by name.** Low.
   `plugins/pi/generate_test.go:82-120` (`TestGenerateFailures`). The
   `retried` case expects success, and the loop branches on
   `tt.name == "retried"`. A reader must notice the special case to understand
   the table. Correction: add a `text` field and assert one rule for every row
   (`want == ""` means no error and `text` equals the result), and rename the
   test (for example `TestGenerateResults`). Alternatively, move `retried` into
   its own test.

4. **"What still loads" column in the docs table.** Low.
   `docs/harness-support.md:32,34`. The Pi cell, "Models from Pi extensions are
   unavailable.", states what is missing, not what loads. The Claude Code cell,
   "Your Claude Code login.", states what every reader assumes (a work-log
   deviation; the plan did not specify it). Only the OpenCode cell answers the
   column. Correction: drop the third column and follow the table with two
   sentences: OpenCode loads its global configuration, so global MCP servers
   and plugins start for each call; Pi runs with extensions off, so models that
   Pi extensions provide are unavailable.

### Checked and correct

- **OpenCode lifetime.** `os.RemoveAll` is deferred before `ag.stop`, so the
  server has stopped before removal. `launchCatalog` stops the agent on
  subscribe and catalog failures, as `models`' old deferred stop did; `launch`
  stops its own process on failure. Cancellation ends `server.call`; the
  deferred stop cancels the stream, stops the server and waits for the stream
  goroutine.
- **OpenCode extractions.** `choose` keeps the same predicate, order and
  messages as the old `selectSettings` body; `selectSettings` uses the effort
  that `choose` validated. `models` behaves as before.
- **OpenCode request.** `providerID` and `id` come from the matched
  `modelInfo`, so `q/b/c` stays intact; `variant` is omitted when empty.
- **Pi lifetime.** `checkUtility`'s deferred `ag.stop` ends the RPC process
  before the prompt run starts. On success `Read` returns only after the
  process has exited. On cancellation `p.Stop` closes stdin and kills after
  3 s, and the reading goroutine sends into a buffered channel, so it does not
  leak. The title prompt is at most 4,000 runes plus a fixed preamble, well
  under a pipe buffer, so the stdin write before the `select` cannot block.
- **Pi parsing and result.** Only `message_end` with role `assistant` decodes
  content blocks, so string content elsewhere is skipped. The latest assistant
  message wins, which covers retries. Order of results: `stop` → text; else
  `errorMessage`; else the stop reason; else the last plain line joined to
  `Read`'s reason; else `Read`'s reason, which is never nil.
- **Docs.** `configuration.md` and `architecture.md` changes match the plan
  and current behavior.

### Verification

- Reused, from the work log, against this working tree: `gofmt -l plugins`,
  `go vet`, `go test ./plugins/opencode ./plugins/pi`, `make check` (runs
  `go test -race ./...`), the live `generate` successes and refusals for both
  harnesses, and the live dev-session title check for both. The log records
  the commands and results; I did not repeat them.
- Run: `go test -race -count=1 -run Generate ./plugins/opencode ./plugins/pi`
  on this tree: `ok` for both. Reason: the new tests start goroutines and fake
  processes; this confirms them under the race detector on the exact tree I
  reviewed, at low cost.

### Notes (no change required)

- `plugins/pi/generate.go`: `fmt.Errorf("%s", o.failure)` could be
  `errors.New(o.failure)`.
- `plugins/pi/generate_test.go`: the local variable `print` shadows the
  built-in.

### Limitations

- No commit anchor exists; this review covers the fingerprinted tree above.
  Any change after it needs a diff against that fingerprint.
- Linux not examined. I did not rerun live model calls.

### Execution decision

- Author: coordinator, Hypercube session 12efdd39 (Claude Code, Opus 5.5);
  native conversation `db73ed8b-a2fe-4d6b-8cfc-a41de1a7f3a3`.
- Findings 1–4: resolved in Repair 1 of the work log. The coordinator checked
  each against the review: the OpenCode rejection cases assert their messages;
  the proxy helper is shared as `owned`; the Pi table has one assertion rule;
  the docs table has two columns followed by the OpenCode and Pi sentences.
- Evidence (reused): `go test -count=1 ./plugins/opencode ./plugins/pi` ok;
  `make check` exit 0, on the repaired tree.
- The user asked for no further review rounds; there is no second
  independent round.
- Decision: accept.
