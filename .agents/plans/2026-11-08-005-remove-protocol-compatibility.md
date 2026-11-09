# Remove obsolete plugin and runtime protocol paths

- Task ID: `2026-11-08-005-remove-protocol-compatibility`
- Status: ready; awaiting independent plan review
- Source HEAD: `ae022eeebf9dc90c75a61527267db0682b194057`
- Author: `compat-plan`, Hypercube `d36e9339`, native
  `ses_e41a1172bffeqvkaFBnCzmGxvc`; OpenCode / `openai/gpt-6.1-sol` / `xhigh`
- Artifact path (repository-relative): `.agents/plans/2026-11-08-005-remove-protocol-compatibility.md`
- Shared guidance (repository-relative): `.agents/designs/2026-11-08-003-remove-backwards-compatibility.md`,
  linked [here](../designs/2026-11-08-003-remove-backwards-compatibility.md).
- Technical prerequisites: none. Administrative order follows 004; preserve 003's
  earlier edits to the app integration test.
- Request: remove old command/start/CLI and saved-runtime exceptions while keeping
  current launches, answers, replay, and supported session placement working.

## Outcome and supported contract

There is one start encoding (`mcp` bridge executable), one answer encoding
(`choices` array plus optional free text), and dedicated `hypercube-mcp` /
`hypercube-hold` executables. The main executable supports version/ui only.
Delete historical aliases rather than converting saved command rows or inventing
protocol versions. Current producers already emit this contract; no holder,
recorded command, MCP transport or native harness protocol changes are needed.

Do not stop/restart installed agents. Existing latest holders/plugins remain
usable: commands and helper locations do not change. Current replay IDs/order,
acceptance transactions, duplicate suppression, pending questions, grants,
multiselect/free text, holder adoption and SIGTERM detachment must remain intact.
No browser changes, installation, state reset, push or merge.

## Existing paths and chosen simplification

`internal/harness/protocol.go` retains Command.Choice and StartArgs.Hypercube
solely for old saved lines. `plugins/internal/plugin/serve.go:handle` converts
Choice into Choices. `plugin.go:MCPCommand` chooses bridge versus backend `mcp`,
and Claude/OpenCode/Pi/scripted callers carry its executable/argument tuple.
Remove both fields, the conversion, the helper and obsolete tuple wiring.

Native plugin starts are invoked through shared Serve. Preserve the existing
missing-MCP failure at that external boundary: after checking nonnil Start,
reject empty `Start.MCP` before calling the native start function. Tests' shared
fake start args need an explicit bridge path. Native start functions use `a.MCP`
directly; Claude/Pi MCP JSON retains `args: []` (not null), OpenCode command is
`[]string{a.MCP}`, and scripted `createSessions` runs `exec.Command(start.MCP)`.
The scripted plugin has its own command loop, so keep an explicit empty-MCP
check where createSessions actually needs the bridge; scene-only starts without
tool calls need not gain a new requirement. No new shared getter/tuple helper.

`cmd/hypercube/main.go` invokes MCP/holder packages for old subcommands, while
the dedicated command packages already own current execution. Delete those
dispatch cases/imports and update usage/comments. Keep dedicated executables
unchanged. Update `agents/architecture.md` to remove the obsolete commands'
promise, not the current process architecture.

Two runtime exceptions need actual behavior changes. In `offers.go:answerOffer`,
validate the chosen answer against the saved item's choices (reuse the ordinary
answer validation or an equivalent membership check); do not grant Dismiss when
absent from an old two-choice offer. Today's offers still save all three choices,
and all three answers retain their existing messages/actions. In
`child_options.go`, reject a nonempty directory for every fork rather than
resolving/accepting one that happens to name the parent's directory. Fork placement
then needs parent/name only: checkout/non-Git parents share their folder, while
a parent with Base gets a new worktree/child branch from its HEAD. Keep normal
nonfork explicit directories, settings checks, and failure cleanup.

Keep exact-input workspace identity/reclassification in `sessions.go`: today's
plain subfolder can become part of a Git checkout later. Existing tests protect
that current state transition. Also keep OpenCode's default→empty effort for a
model without variants. Current V2 model semantics distinguish provider default
from unavailable effort controls; there is no old-version branch to delete.

## Execution sequence

1. Remove the protocol fields, shared scalar conversion, helper and four callers'
   tuple wiring. Centralize only the already-existing required bridge validation
   in shared Serve as described, with scripted validation at its actual call.
2. Adjust `internal/harness/harnesstest/harnesstest.go:answer` to compute a local
   display string from Choices instead of mutating the deleted Command.Choice.
   Its log Entry.Choice may remain as a derived test observation; it is not a
   protocol input. Preserve current scalar assertions or use Choices where
   clearer, without a test-log format redesign.
3. Delete `TestMCPCommand` and Claude's legacy-MCP-start test. Convert shared
   Serve's answer fixture to Choices and retain current multi/free-answer coverage.
   Add the missing-MCP boundary case. Existing native argument/config tests must
   still verify direct dedicated-bridge configuration and empty argument arrays.
4. Remove main CLI aliases. In `cmd/hypercube/integration_test.go`, simplify the
   bridge helper to the dedicated binary only; run both create_session and
   list_sessions through it. Preserve current app/session lifecycle assertions,
   startup config fixture from 003, shell survival, and helper discovery.
   Add usage-failure coverage for the two removed main subcommands.
5. Simplify fork placement and offer validation. Retain the current Dismiss test
   body but delete its legacy two-choice variant/setup. Change the same-directory
   fork acceptance assertion to rejection, and keep placement/settings checks.
   Update only directly affected current architecture prose.

## Verification

Required `make check` covers all packages, race checks, current native plugin
configuration and the process integration test before the code commit. Focused
Serve, children/offers, and app-process tests are useful only if iteration reveals
a specific uncertainty; do not mandate duplicate broad runs.

Inspect coverage of one/multiple selected answers, free-text-only answers,
missing Start/MCP, unchanged input/configure ordering, and current start JSON
containing only the direct bridge path. Keep existing `internal/session/restart_test.go`
replay/adoption scenarios and the app process test: latest current commands and
in-flight holder state must survive detach/reconnect without another start or
lost accepted input. No new mixed-version test harness is appropriate.

For offers, unknown answers fail and all current answers work; Dismiss queues
the existing no-work-authorized report and creates no child. For forks, both `.`
and a different nonempty dir fail without creating a child/worktree; empty dir
retains original-checkout/non-Git sharing and worktree-parent separate placement.
Retain workspace identity/reclassification and no-effort OpenCode tests unchanged.

Inspect live code for remaining Command.Choice, StartArgs.Hypercube, MCPCommand,
main mcp/hold dispatch, and special old fork acceptance. HTTP form field `choice`,
derived test log Choice, current SDK negotiation, and generic JSON unknown-field
tolerance are not obsolete protocol aliases. Record exact verification results
and code anchors in this unit's work log. No real session restart or install.

## Handoff and normalization

Ready; no open contract decisions. One current-protocol retirement crosses the
existing shared boundary, native wiring and isolated runtime exceptions; no
transport or lifecycle is redesigned. Its test burden exceeds a callback brief
but is smaller than a new integration mechanism. Platform cleanup/verification
belongs to 006. Use the experimental 600–1,500-word band; no curated manifest
was supplied. Planner writes only records; parent commits and requests review.
