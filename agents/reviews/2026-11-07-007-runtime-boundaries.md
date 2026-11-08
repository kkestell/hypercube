# Runtime boundaries: architecture review and plan assessment

## Outcome and scope

The executable split is justified, provided the work separates dependencies
and ownership as well as entry points. The strongest existing defect is the
holder library's assumption that its caller implements the `hold` command.
Related coupling joins MCP transport to backend tools and shared plugin data
to backend execution and application configuration.

This review examines the architecture at `c05f253`, including the three
command roles, backend startup and shutdown, plugin launch and discovery,
MCP forwarding and tools, session persistence and reattachment, panel shells,
native MCP configuration in all four plugins, testing seams, development,
screenshots, and macOS/Linux packaging and installation. It supports the
[implementation plan](../plans/2026-11-07-005-runtime-boundaries.md).

The review is comprehensive for these boundaries. It is not a fresh
correctness audit of every native event translation or browser component.
Source inspection establishes the findings below; it does not establish
performance savings or a history of runtime failures caused by the combined
binary. No product code changes or code checks belong to this planning task.

## Findings in the current code

All findings below concern existing architecture. Their remedies are planned,
not implemented.

| ID | Importance | Finding |
| --- | --- | --- |
| F1 | Medium | A library requires its host executable to implement another role |
| F2 | Medium | MCP forwarding and backend tool execution share a package |
| F3 | Medium | The plugin launch contract exposes the backend command line |
| F4 | Medium | Shared harness data imports backend process machinery and owns application preferences |
| F5 | Medium | Browser HTTP code owns backend-wide composition and access rules |
| F6 | Medium | Tests reproduce the combined executable assumption |

### F1. Holder launch depends on its host's command line

In [hold.Start](../../internal/hold/hold.go), the launcher obtains
`os.Executable()` and executes it with `hold` arguments. Its public arguments
describe the holder directory, child command, environment, working directory,
and terminal size, but omit the executable that actually performs the work.

The function therefore has an extra precondition: its host program must
dispatch the Hypercube holder command. This precondition is unrelated to
managing the child or to the holder's on-disk protocol. It prevents ordinary
reuse from another executable and forces test programs to impersonate the
production command.

Pass an explicit holder executable through backend construction, the plugin
client, and the terminal set. The holder entry point should call the existing
holder implementation. Executable discovery belongs at application startup;
the holder library should execute the dependency it is given.

The holder itself already has a useful boundary: its production package has
no imports of another Hypercube package. Pipes and terminals share the same
lock, input socket, output segments, and exit record. Keeping their client
and holder code in this cohesive package is proportionate. Splitting that
package or replacing its transport is unnecessary.

### F2. The MCP package owns both sides of the process boundary

[mcp.go](../../internal/mcp/mcp.go) contains session-specific tool registration,
database reads, backend HTTP handling, stdio forwarding, session headers, and
HTTP retry behavior. The neighboring files implement backend tools.

`Bridge` only needs streams, the backend address, and session identity.
Its package nevertheless imports `session` and `store`, reaching terminals,
holders, workspaces, and SQLite transitively. Creating another `main` that
imports this package would retain these dependencies.

Move transport forwarding and its retry behavior into `internal/mcpbridge`.
Keep tools and their HTTP handler in the backend-owned `internal/mcp` package.
Sharing the MCP SDK is appropriate; sharing application state with the
forwarder is unnecessary. A stable header literal at both endpoints does not
justify a separate package containing only that constant.

### F3. Plugins receive the application executable instead of the MCP command

[startArgs](../../internal/session/launch.go) obtains the current executable
and records it as `StartArgs.Hypercube`. Claude, Codex, OpenCode, Pi, and the
scripted plugin then append `mcp` themselves.

This makes every integration depend on the backend's command dispatch. It
also makes a launch dependency implicit in the process constructing the
command. Supply the MCP command explicitly in the start data. An argument
vector is justified by the two actual command forms: a direct helper for new
launches and `hypercube mcp` in saved starts from the previous version.

Preserve the session environment and each plugin's native configuration
format. A shared plugin-side adapter can read an old recorded start; native
adapters should continue to own rendering the resulting command into their
own configuration.

### F4. Shared protocol data and backend operations have the same owner

`internal/harness` currently contains:

- [Protocol data and validation](../../internal/harness/protocol.go), used by
  plugins, storage, sessions, and browser handlers.
- [A plugin client](../../internal/harness/harness.go), which imports and
  starts holders.
- [Discovery types and process execution](../../internal/harness/discovery.go).
- [Application configuration](../../internal/harness/config.go), including
  theme, terminal font size, notifications, worktree defaults, and migrations.

Consequently, importing protocol types brings a dependency on holder
machinery. Even storage depends on this mixed package through `Usage` and
`Limit`. Preferences have an owner whose name describes only one part of
their contents.

Keep `internal/harness` for shared data and pure validation. Move the client
and discovery execution to `internal/harnessclient`, and preferences to
`internal/config`. This removes inappropriate imports without renaming the
shared types throughout every integration. Export the existing pure record
decoder for the moved client; do not duplicate record validation.

Storage can continue to use shared protocol values. Creating a parallel
set of usage, image, transcript, and catalog types with conversion functions
would add work without resolving another ownership conflict.

### F5. Browser handlers compose unrelated backend endpoints

[ui.Handler](../../internal/ui/ui.go) registers browser routes, the agent MCP
route, the desktop connection, build identity, and access protections. Its
origin check knows that `/mcp` has different callers. Meanwhile,
[cmd/hypercube/main.go](../../cmd/hypercube/main.go) owns configuration
upgrades, database opening, harness scanning, runtime restoration, HTTP
startup, notification preferences, and shutdown.

Move backend-wide construction, route composition, and access protections to
`internal/backend`. Browser routes can remain in `internal/ui`. A thin
command entry point should own command arguments and executable location;
`backend.Serve` should own its signal subscription and the resources it starts.

Keep host, origin, and framing protection outside the composed routes. The
existing MCP exception must remain specific to that endpoint. This is a
responsibility move, not permission to broaden access or expose another
listener. Move tests of these protections to the composition boundary.

### F6. Tests encode the packaging decision

[holdtest.Main](../../internal/hold/holdtest/holdtest.go) exists because
`hold.Start` executes the test binary. Session, terminal, MCP, browser, and
command test suites all dispatch holder mode in `TestMain`. Holder tests also
contain their own holder dispatcher.

These tests exercise useful process behavior, but their setup hides the
production executable dependency. Use the real narrow holder command through
an explicit path, built once per test process by a test-only helper. Preserve
race instrumentation for the subprocess checks. Delete the fake production
dispatchers rather than leaving two setup mechanisms.

The native harness stand-in is different: it intentionally implements the
plugin protocol to exercise session behavior without real model calls. Keep
that seam. The split does not justify replacing the stand-in framework or
duplicating every lifecycle test at every executable boundary.

## Boundaries that should remain together

The current process model already separates lifetimes. A shared executable
does not mean a shared process or a second database owner.

- The backend owns session state and saved changes. Browser actions and MCP
  tools reach the same runtime. Tool handlers belong with that backend.
- `session.Runtime` coordinates launch decisions, command recording, input
  acceptance, transcript checkpoints, pending items, and family cleanup.
  Those operations share real transactions and lock ownership. Moving them
  into independently mutable services would make this change harder to
  understand and preserve.
- A holder owns a child and retained I/O across backend restarts. The backend
  adopts it through existing files and sockets. Keep that protocol intact.
- A plugin owns the native harness protocol and process. Its `Process` helper
  handles native pipes and final output; the holder handles detachment and
  durable I/O. These are distinct responsibilities. Combining their process
  implementations would obscure that distinction.
- Shell admission belongs with session and workspace cleanup so a delete or
  archive cannot race a newly admitted writer. Terminal mechanics belong in
  `terminal` and `hold`.
- The Electron client, Git workspace operations, store, and report formatting
  have no evidenced need for another service or generic interface here.

The cleanup should leave these relationships easier to see, with existing
semantics for SIGINT, SIGTERM, parking, restart, delivery acceptance, command
IDs, and output positions.

## Upgrade and installation constraints

### Old agents still need their recorded MCP command

All four integrations supply MCP configuration per native launch or resume:
Claude through `--mcp-config`, Codex through thread configuration, OpenCode
through its private server environment, and Pi through `HYPERCUBE_MCP`.
No edit to a user's global or project native configuration is needed.

Surviving old plugins and native harnesses retain `<hypercube> mcp`, however,
and may spawn that command again after a bundle replacement. Keep this one
backend command as a tiny process replacement into the sibling bridge helper.
It should preserve streams, environment, signals, and exit status without
importing the bridge implementation into the backend command.

Saved start commands are also real compatibility input:
[record](../../internal/session/commands.go) stores complete command JSON,
and reattachment resends unanswered lines. Preserve those lines and IDs.
The narrow legacy start adapter handles new plugins reading old starts;
the forwarding command handles old processes launching their existing MCP
configuration. This avoids a database migration or rewriting in-flight
commands.

An already running holder does not invoke the holder command again. Its
lock, PID, terminal, socket, segments, and exit record provide adoption.
There is no equivalent persistence reason to retain `hypercube hold`.

### Every build path must supply the helpers

The main `Makefile` builds backend and plugins, but the screenshot target also
builds a temporary installation directly. Both must include the new helpers.
The regular development build and both bundles must contain the same required
executables. macOS signing must include the helpers and preserve the
backend's existing privacy identity.

Keep helpers private to the bundle. Resolve the real backend executable once
so `bin/hypercube` and installed CLI symlinks find helpers in the bundle;
otherwise private helpers would accidentally require additional public
symlinks. Cover a symlink launch and a path containing spaces.

Service names, the `hypercube ui` command, the desktop executable, and service
shutdown behavior can stay stable. Existing Linux `KillMode=process` and
macOS SIGTERM replacement behavior already allow adoption. Preserve them.

`scripts/dev` identifies orphan holders by command text. It must recognize
the old and new holder names for PIDs read from its own isolated state.
Do not expand cleanup into a system-wide process sweep. Existing installers'
replacement behavior is not a request for a new update transaction system.

## Simpler baseline and design assessment

A package-only cleanup would fix the shared imports and could replace
self-execution with an explicitly supplied command vector. It would retain
one artifact, one dispatcher, and all backend code in the executable used for
every bridge and holder. That is a viable smaller alternative.

The chosen three-executable design also removes the host-dispatch requirement
and expresses the already separate process roles in the distributed programs.
The extra cost is two packaged and signed binaries, explicit paths, real
helper setup in tests, and the narrow legacy MCP forwarding path. The
existing multi-executable plugin bundle makes that cost proportionate.

Separate modules, plugin loading frameworks, dependency-injection containers,
helper registries, transport replacement, version negotiation, and new
durability guarantees are unnecessary. Protocol and application preference
package moves, explicit launch dependencies, and backend composition address
the evidenced problems directly.

## Plan assessment

The parent assessed the fresh planning agent's draft independently against
the source and the package-only baseline. The plan is ready for implementation
after the following corrections, now incorporated into it:

1. **Include the scripted plugin's actual MCP call.** Its `createSessions`
   function executes the old backend command, and its independent serve loop
   does not receive the shared native adapter. The plan now names that code
   change and a non-rendering scene regression. An offer can exercise the
   callback without changing the existing Linux test's agent PID count.
2. **Release desktop writes before joining the watcher.** The draft proposed
   waiting for alert-watch completion before closing the desktop connection.
   `Desktop.Alert` uses `context.Background()` for its WebSocket write, so
   cancelling the watch cannot release a blocked write. The revised shutdown
   closes the connection before joining the watcher and closes SQLite after
   the watcher finishes. This uses the current connection shutdown mechanism.
3. **Make intermediate steps usable.** Real helper construction in fixtures
   must accompany removal of test-binary holder dispatch. The existing MCP
   command temporarily calls the extracted bridge until it is replaced by
   the compatibility exec. Neither step leaves a missing dependency hidden
   until the final verification phase.
4. **Verify the environment that really exists.** Normal `scripts/dev` strips
   external scene variables, so the plan no longer assumes that exporting
   `HYPERCUBE_SCENE` supplies a scripted development agent. Isolated dev
   startup/rebuild and the existing stand-in process tests cover their proper
   boundaries. Callback/restart tests retain the same backend address that
   surviving processes were configured to use.

The final design addresses F1 through explicit program paths; F2 through an
independent bridge package; F3 through MCP argv and narrow legacy handling;
F4 through shared data, client, and preferences ownership; F5 through backend
composition; and F6 through actual helper executables in tests. Its inventory
includes native and scripted integrations, private helper packaging/signing,
symlink invocation, development cleanup, archive contents, current references,
and deletion of replaced dispatch and forwarding code.

No material design question remains unresolved. This verdict approves the
plan's coherence and scope, not an implementation. The implementation must
still receive the fresh work and code-review agents and the validation required
by `agents/development.md`. Architecture findings above remain planned until
that work is complete.

## Verification of this review and plan

Inspected the source and command/configuration call sites described above,
the completed plan, its revisions, and the final document diff and local
links. No builds, tests, model-backed sessions, installs, service changes,
or screenshot captures were run. The artifacts are planning documents;
implementation checks are specified in the plan.
