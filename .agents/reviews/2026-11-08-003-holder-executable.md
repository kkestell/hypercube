# Holder executable review

## Scope and coverage

Reviewed the staged implementation of
[plan 009](../plans/2026-11-07-009-holder-executable.md), its
[work log](../work/2026-11-08-001-holder-executable.md), and every changed
production and test file as a whole. Traced executable discovery, runtime
construction, agent launch and error reporting, terminal creation and
adoption, holder startup and inherited descriptors, retained output,
restoration, and harness stand-in dispatch. Checked the original runtime
boundary findings F1 and F6 and the parent plan assessment.

The review covers source behavior and isolated process tests. Packaging,
development and screenshot builds, installation, signing, and live native
harness verification belong to plan 011 and were not exercised. Legacy
`hypercube hold` compatibility was checked by tracing its unchanged direct
dispatch and argument/descriptor contract; there is no new process test of
an older backend replacing its executable.

## Summary

No findings. No production or test fixes were needed.

The holder executable imports only the existing holder implementation.
`hold.Start` executes the supplied path without the old `hold` argument;
socket creation, lock/listener inheritance, detachment, environment, working
directory, and failed-start socket cleanup retain their existing ownership.
The runtime supplies the path to agent and shell launches. Attachment and
adoption do not require that path. The process test builds all three
commands, launches the backend through a symlink, checks the holders'
command paths, and preserves the existing restart and shutdown assertions.

The TestMain adjustment is justified. `harnesstest.Run` returns immediately
without its stand-in environment marker, and serves the protocol and exits
when the marker is present. Dispatching it before `holdtest.Main` therefore
avoids rebuilding a holder in each plugin subprocess. Removing the redundant
`TestStandIn` functions is safe: TestMain handles those invocations before
test selection. The command package retains its stand-in test because its
TestMain was removed. The helper builds once per normal test process,
matches race instrumentation, retains the race-detector exit setting, and
removes its temporary executable after tests finish.

## Checks run

- `go test -race ./internal/hold/... ./internal/terminal` — passed from the
  test cache, confirming the unchanged reviewed inputs match passing tests.
- `go list -deps ./cmd/hypercube-hold` — completed successfully; its only
  Hypercube packages are the command and `internal/hold`. Go emitted a
  sandbox warning about writing module stat-cache metadata, which did not
  prevent dependency enumeration.
- Inspected all `hold.Start`, `harnessclient.Launch`, `terminal.Set`, and
  `session.Programs` construction sites. The remaining empty program sets
  do not launch agents or shells. The `os.Executable` search under
  `internal` and `plugins` contains only tests and the harness test helper.
- Reused the implementation's reported final passing `make check` and
  Linux race tests for holder, terminal, session, MCP, UI, and backend,
  recorded in the work log. No code changed during review, so repeating
  those full checks was unnecessary.
- `git diff --cached --check` — passed before the review was added and
  again with the staged review.

## Verdict

Pass for plan 009. The explicit executable boundary resolves F1 and F6
without changing holder state or transport. No confirmed medium or high
severity issues remain. The intermediate checkout still requires plan 011
before installation or release.
