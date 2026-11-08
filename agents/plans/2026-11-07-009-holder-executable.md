# Launch a separate holder through explicit paths

## Goal and prerequisites

Make `hold.Start` launch a supplied `hypercube-hold` executable, removing the
requirement that its caller implement Hypercube's `hold` command. Agent and
panel-shell launch use that path explicitly. Process tests run the real helper
instead of making their own binaries act as the holder.

Prerequisite: [MCP bridge executable](2026-11-07-008-mcp-bridge-executable.md),
which introduces canonical sibling resolution and `session.Programs` after
the configuration/protocol extractions. This plan completes review findings
F1 and F6. Normal build recipes and bundles remain incomplete until
[runtime integration](2026-11-07-011-runtime-integration.md); this source task
builds the helper directly for tests and does not add shipping fallbacks.

## Related code

- `internal/hold/hold.go:Start` creates/locks holder files, creates the Unix
  listener, and invokes its caller's executable with `hold` and descriptors
  3/4. `holder.go:Main` owns the child; `output.go` owns retained-output reads.
- `internal/harnessclient` launches plugins under holders and attaches to
  existing ones. `internal/session/launch.go` calls that client.
- `internal/terminal/terminal.go:Set.Open` adopts a live shell or starts a
  terminal holder. `Set.Adopt` only attaches.
- `internal/hold/holdtest/holdtest.go` dispatches the test binary as holder.
  Command, session, terminal, backend MCP, and browser suites call it from
  `TestMain`. Hold's own `TestMain` duplicates that dispatch and also has an
  intentional launcher-parent survival mode.
- `cmd/hypercube/main.go` still dispatches `hold.Main`; the MCP plan's command
  location/exec code already resolves canonical sibling programs.

## Decisions

Add `cmd/hypercube-hold` as a narrow caller of `hold.Main`, passing its
arguments and reporting errors under its own program name. Keep holder
client and implementation in the same package: their shared on-disk protocol
is small and coherent. Preserve the existing file split by concern instead
of adding client/server/wire packages.

Change `hold.Start` to take a helper executable path first, followed by its
current directory, argv, environment, working directory, and optional terminal
size. Execute the helper directly without prepending `hold`. Add `Holder`
to `session.Programs`, set it from the canonical sibling directory, pass it
through `harnessclient.Launch`, and configure `terminal.Set.Holder` from the
same value. Update every constructor and fixture that can start a holder.
No library finds Hypercube-owned helpers through its executable or `PATH`.
A missing helper produces a contextual launch error without fallback.

`hold.Open`, `harnessclient.Attach`, and `terminal.Set.Adopt` need no helper
path. `Set.Open` does need it when no live holder can be adopted. Keep those
responsibilities clear rather than making attachment depend on executable
availability.

Replace production `hypercube hold` with a tiny `syscall.Exec` into the
sibling helper. Preserve original arguments, environment, stdio, and inherited
lock/listener descriptors 3 and 4. Old backends still use that command and can
launch during macOS bundle replacement or a development rebuild before the
backend restarts. Retaining exec compatibility avoids a new command mismatch;
it does not guarantee launches through existing replacement gaps or change
Linux's behavior when the running executable's directory is moved. Keep the
already extracted MCP command unchanged.

Do not alter holder layout, socket length limit, process groups, environment,
working directory, terminals, output offsets/retention, exit-record ordering,
or stop semantics. Existing holders and shells remain adoptable without
migration or forced restart. Preserve SIGTERM detachment, SIGINT deliberate
termination, and desktop disconnection behavior. No new supervision,
durability, version negotiation, or delivery guarantees belong here.

## Implementation plan

1. Add the narrow command and change `hold.Start` to explicit helper launch.
   Update helper usage text and errors to the actual executable. Reuse the
   command's sibling discovery for production `Programs.Holder` and the
   legacy exec; remove `hold.Main` dispatch from the backend command.
2. Wire `Holder` through session launch, `harnessclient`, and terminal setup.
   Update all direct `hold.Start`, `terminal.Set`, runtime, and backend
   constructor callers. Preserve existing adoption paths and lifecycle code.
3. Replace `holdtest`'s dispatcher with test-only construction of the real
   helper once per test process. The builder must not import `hold`, allowing
   hold's own tests to use it without a cycle. Return/provide its explicit path
   to fixtures and remove its temporary executable directory when the suite
   ends. Avoid per-test compilation and production environment overrides.
4. Use two small files selected by `//go:build race` and `//go:build !race` to
   choose whether the test helper's `go build` passes `-race`. Ordinary focused
   tests should not gain an extra C-toolchain requirement. Preserve inherited
   `GORACE=atexit_sleep_ms=0` before launching race-enabled test holders so
   every subprocess exit does not add a second.
5. Wire real-helper setup into every affected suite in this task and delete
   their fake holder dispatch. Retain hold's intentional `parent` test mode,
   passing the already built helper path to that launcher subprocess rather
   than recompiling or redispatching holder behavior. Keep the native plugin
   stand-in protocol; it is a distinct, useful test seam.
6. Add one backend-command compatibility test with actual inherited lock and
   listener files. Existing command process integration builds the holder
   beside its backend/MCP executables directly. Remove replaced dispatch
   machinery, stale self-exec comments, and unused imports as part of this
   source task rather than deferring them as general cleanup.

## Acceptance and verification

- Existing holder tests run the real helper and preserve observable pipe and
  terminal I/O, positions, reattachment, retention/discard, final output/exit,
  process groups, stop/lost-holder behavior, and launcher-parent survival.
  A nonexistent helper path fails clearly instead of starting the caller.
- Terminal and session tests cover new helper launch and existing-holder
  adoption. Adoption itself must not invoke a helper. Backend process tests
  retain agent and panel-shell PIDs, transcript, and continued input over
  SIGTERM/restart and terminate them on SIGINT.
- One `hypercube hold` forwarding test passes descriptors 3 and 4 through the
  command and verifies child input and retained output. This protects the
  old-backend call shape without an old application binary or install-race
  simulator. Main holder tests launch the helper directly.
- Inspect dependencies: `cmd/hypercube-hold` has no local package except
  `internal/hold`; backend production code does not call `hold.Main`.
  Remaining `os.Executable()` uses for Hypercube program paths belong to
  command composition or intentional test orchestration, not libraries.
- Run focused holder, terminal, session, and command process tests, including
  a race-enabled run that uses the matching helper. Run `make check` before
  a code commit. Record results/omissions in the work log and stage for fresh
  code review. Packaging, signing, development cleanup, Linux installs, and
  screenshot runtime construction are verified once in the final integration
  plan; they are not source-test prerequisites here.

Before implementation, explain that surviving agents and shells keep their
existing holder/plugin code; affected sessions or shells restart to use new
launch behavior. Current operational documentation is updated with the final
integration task.

Next plan: [Backend composition](2026-11-07-010-backend-composition.md).
