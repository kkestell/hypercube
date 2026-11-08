# Separate backend, MCP bridge, and holder responsibilities

## Goal

Give the backend, the stdio MCP bridge, and the process holder their own
executables and explicit dependencies. Untangle the packages that currently
make a plugin, a bridge, or a test binary depend on unrelated application
responsibilities. Keep the resulting code straightforward to read and change.

The completed implementation has three production entry points:

| Executable | Responsibility |
| --- | --- |
| `hypercube` | The existing `ui [port]` backend and `version` command. The backend owns SQLite, the session runtime, browser routes, backend MCP tools, and the desktop connection. |
| `hypercube-mcp` | Forward stdio MCP messages to the backend with the calling session's identity. |
| `hypercube-hold` | Own one child process and its input, output, terminal when present, and exit record independently of the backend. |

Keep `hypercube mcp` and `hypercube hold` as small compatibility commands that
replace themselves with their respective helpers. New launches use the
helpers directly. Old native processes still invoke the former; an old
backend can invoke the latter while its executable is being replaced before
the backend restarts. Neither forwarding command contains the helper's
implementation.

This plan covers the connected source, test, build, installation, development,
and documentation changes. It does not implement them. The parent records its
independent architectural review in
`agents/reviews/2026-11-07-007-runtime-boundaries.md` before implementation.

## Findings from the current implementation

The process responsibilities already exist. The defect is how their code and
launch dependencies are connected, rather than a missing process boundary.

1. `cmd/hypercube/main.go` dispatches all three roles. It also contains backend
   setup, configuration upgrades, database lifetime, runtime restoration, HTTP
   startup, notification preferences, and signal handling. A holder or bridge
   entry point consequently shares a package and imports with the whole
   backend.
2. `internal/hold/hold.go:Start` calls `os.Executable()` and prepends `hold`.
   Its caller's program must implement Hypercube's command dispatch. The
   holder package otherwise has no application-package imports, and its files
   already distinguish client attachment, output reading, and holder execution.
   Its file/socket protocol is an appropriate existing boundary.
3. `internal/hold/holdtest/holdtest.go` makes unrelated test binaries run
   `hold.Main`. Command, session, MCP, terminal, and browser tests use that
   dispatcher through `TestMain`; hold's own `TestMain` duplicates it. This
   machinery follows from the production self-execution assumption.
4. `internal/mcp/mcp.go` contains both the stdio-to-HTTP bridge and backend
   tool registration/handlers. The bridge imports the session runtime and
   store through its package, even though forwarding needs neither. Bridge
   tests share fixtures with tool tests and use their database-backed server.
5. `internal/harness/protocol.go:StartArgs.Hypercube` names the backend
   executable. `internal/session/launch.go:startArgs` obtains it through
   `os.Executable()`, and all four plugins append `mcp`. Their real dependency
   is the MCP command and its environment.
6. `internal/harness` mixes shared command/event/catalog data and validation,
   backend plugin discovery/execution, a holder-backed plugin client, and all
   application preferences. A plugin importing its protocol types acquires
   dependencies on holder execution and preferences. `store` uses harness
   data such as usage and limits, which unnecessarily pulls those same
   responsibilities into the storage dependency graph.
7. `internal/ui/ui.go:Handler` composes browser routes, `/mcp`, `/desktop`,
   `/version`, and the common host/origin/frame protections. A browser package
   is therefore the assembly point for backend services that do not belong to
   the browser. Its route-protection tests exercise all three clients.
8. Distribution already supports sibling executable plugins. `Makefile`,
   `make/macos.mk`, and `make/linux.mk` build/package those programs; macOS signs
   them and gives the backend its existing privacy-permission identifier.
   `make screenshots` separately builds just the backend and scripted plugin.
   Both construction paths need the new helpers.
9. Linux replaces the installed bundle as a complete directory; macOS removes
   and copies its bundle. Existing plugins and holders survive installation
   with the code they started from. The service retains its backend command
   and restarts it with SIGTERM. `scripts/dev` uses the normal binaries target,
   but its crash cleanup recognizes only the old `hypercube hold` process name.
10. Start command JSON lines are saved in SQLite by
    `internal/session/commands.go`, resent verbatim in ID order, and deleted
    with the transaction applying their replies. A backend restart can meet
    an old plugin that has or has not accepted its start command. This is
    concrete compatibility state, not merely an old CLI spelling.

The native MCP configuration does not require a storage migration. Claude's
plugin supplies `--mcp-config` on each launch; Codex supplies thread start,
resume, and fork configuration; OpenCode configures its private server through
`OPENCODE_CONFIG_CONTENT`; Pi supplies `HYPERCUBE_MCP` to its process. Each new
plugin launch supplies current MCP settings while resuming the saved native
conversation. An already running native process keeps its old command and can
start that command again after installation.

## Design decisions

### Compare the complete designs

A package-only refactor could move forwarding, preferences, and plugin client
code, then inject an explicit command such as `{hypercube, "hold"}` into the
holder launcher. That would remove the most harmful hidden coupling with less
packaging work. A single executable is not inherently a bad design, and the
three roles are already separate processes.

Choose separate executables here because the three responsibilities are
independent, have small established communication boundaries, and need no
shared backend startup. A narrow executable makes the allowed production
dependencies inspectable, lets tests launch the real holder without pretending
to be the application, and stops new plugins from receiving an unrelated
backend command. The existing sibling-plugin packaging model makes the added
distribution cost manageable. The package-only work is still necessary: three
`main` functions importing today's mixed packages would leave the problem
largely intact.

The additional costs are two built/signed/shipped files, explicit launch paths,
test construction of a real holder, and a narrow transition for live agents.
Keep the helpers private to the bundle to avoid expanding the installed public
CLI surface or adding more symlink/uninstall responsibilities.

### Package ownership and allowed dependencies

| Package | Final ownership and dependency direction |
| --- | --- |
| `cmd/hypercube` | CLI parsing, build identity, canonical executable-directory discovery, compatibility execs, and backend composition/lifetime. Keep startup/shutdown, HTTP composition, and alerts in focused files. Never imports `mcpbridge` or calls `hold.Main` in the final implementation. |
| `cmd/hypercube-mcp` | Read `HYPERCUBE_URL` and `HYPERCUBE_SESSION_ID`, report missing inputs, and call `mcpbridge.Run` with explicit values and stdio. |
| `cmd/hypercube-hold` | Call `hold.Main` with its arguments and report errors under its own program name. |
| `internal/mcpbridge` | MCP transport forwarding and the existing connection-refused retry policy. Imports the MCP SDK and standard library, with no local backend, store, session, harness, hold, terminal, desktop, or UI dependencies. |
| `internal/mcp` | Backend MCP tool schemas, permissions/scopes, registration, HTTP handler, and adapters to `session` and `store`. No stdio transport or environment discovery. |
| `internal/harness` | Shared harness/plugin data and validation: commands, replies, events, transcript items, selections, settings, descriptions, catalogs, image validation, and record decoding. No executable discovery, process management, holder, configuration-file, session, or store dependency. |
| `internal/harnessclient` | The backend's holder-backed plugin connection and one-shot executable discovery/model queries. Imports `harness` and `hold`, never the session runtime or application preferences. |
| `internal/config` | The existing application preference document, defaults, validation, atomic writes, and upgrades. May use pure `harness.Settings`; no process execution or backend startup. |
| `internal/hold` | The existing holder client, output reader, and holder implementation, organized by their current files. Starts a supplied helper path. No knowledge of the backend executable. |
| `internal/session` | Existing coordinated lifecycle, transactions, command replay, transcript application, and terminal ownership. Receives concrete executable paths; uses `harnessclient` for backend process operations and `harness` for shared data. |
| `internal/terminal` | Existing panel shell set and holder attachment. Receives the holder executable explicitly. |
| `internal/ui` | Browser assets, data socket, browser-facing routes, and their adapters to existing backend services. Does not import backend MCP tools or desktop connection code. |
| `plugins/internal/plugin` and native plugins | Native process ownership and translation remain together as today. Import pure `harness` and plugin helpers, not backend process clients, holder code, preferences, or backend packages. |

Keep `store`, `report`, `workspace`, `desktop`, and the React state model in
their existing roles. Shared harness data in store or browser responses is not
itself an ownership defect once the shared package is independent. MCP and
browser adapters may read the store directly where they already do; do not
insert pass-through runtime methods or repository/service interfaces merely to
redraw the dependency graph.

The holder client and holder implementation can remain in one coherent
package: they share the small on-disk protocol, and neither depends on the
application. Splitting that package into client/server/wire packages would add
exported surface and movement without removing the harmful application
coupling. Preserve the existing distinction between `hold.go`, `output.go`,
and `holder.go`.

### Concrete launch dependencies

Use a plain `session.Programs` value containing `PluginDir`, `Holder`, and
`MCP` paths. Change `session.New` to receive it, save it privately in the
runtime, and remove the public `Runtime.PluginDir` configuration path. This is
concrete launch configuration, not a provider interface or helper registry.

At `cmd/hypercube`'s composition boundary, obtain `os.Executable()`, resolve
symlinks with `filepath.EvalSymlinks`, and take its real parent directory.
Construct `session.Programs` from the known siblings in that directory and
pass it to backend startup in `cmd/hypercube`. Keep the existing private
startup function, adapted to accept explicit programs for focused tests;
there is no separate backend package or exported startup API. Neither session,
terminal, harness client, nor holder libraries call `os.Executable()` or search
`PATH` for Hypercube-owned programs. Native harness CLI discovery continues to
use the user's existing `PATH` as it does now.

Change `hold.Start` to take the holder executable as its first argument, then
the current holder directory, child argv, environment, working directory, and
optional terminal size. Execute that helper directly, without a `hold`
subcommand. Change `harnessclient.Launch` to receive/pass the holder path.
Give `terminal.Set` a `Holder` path and populate it from the same runtime
programs. `hold.Open`, `harnessclient.Attach`, and `terminal.Set.Adopt` attach
to existing holders without an executable. `terminal.Set.Open` uses the
configured `Holder` path whenever it needs to start a shell. A missing helper
must yield a contextual launch error; do not fall back to the current
executable, another installation, or `PATH`.

Introduce `StartArgs.MCP string` with JSON name `mcp`, containing the absolute
`hypercube-mcp` executable path. New starts need no arguments for this helper.
Keep the existing `env` wire field and document it as the MCP process's
environment, including the current session ID and backend URL.

Keep argument construction inside the plugin adapters. One shared plugin
helper returns the direct executable or the legacy `{Hypercube, "mcp"}` argv
when reading an old recorded start, as described below. The wire contract
needs only the executable path; do not add a general argv field, executable
options system, helper-selection environment variables, or remote command
description.

Expose `harness.DecodeRecord` when moving the backend client out of its
package; it owns the current record validation, including the private event
checks. Do not duplicate validation or export private helpers solely to reach
across the new boundary.

### HTTP composition and backend lifetime

Keep the backend in `cmd/hypercube`, with focused `backend.go`, `http.go`, and
`alerts.go` files for startup/shutdown, HTTP composition, and alert delivery.
`main.go` retains command dispatch, argument parsing, and executable location.
Once the other process implementations leave this command, a separate
`internal/backend` package would add an exported entry point and a test
constructor for a single caller without establishing another needed boundary.

The backend's top-level mux mounts `POST /mcp`, `GET /desktop`, and
`GET /version`, then delegates browser routes to `ui.Handler(db, rt)`. Move
`localOnly` with this composition and apply it once around the complete
handler. Preserve every current route's method and semantics, the `/mcp`
origin exception, known-session validation, localhost checks, origin checks
for browser/desktop sockets and actions, and `SAMEORIGIN` frame header.
The desktop client still connects to the same endpoint and the browser sees
the same API and assets.

Retain startup ordering: upgrade preferences, open the store, construct the
runtime, scan plugins, register saved launches, begin HTTP service, then adopt
readers and deliver queued work. In particular, an attaching agent can call a
backend MCP tool while restoration is proceeding.

The backend remains the owner of the server and notification-watch goroutines.
Give the alert watch a completion signal. Preserve the runtime's `Quit` versus
`Detach` decision, cancel the watch, close the desktop connection and HTTP
server, then wait for the watch before closing the database. Closing the
desktop connection must precede that wait: `desktop.Alert` currently writes
with `context.Background()`, so context cancellation alone cannot release a
blocked notification write. Retain the existing connection shutdown mechanism;
do not introduce a new notification timeout or delivery guarantee. This is
local lifetime ownership, not a new draining or recovery scheme. Leave the
runtime's transaction/locking model together. Keep signal registration/cleanup
visibly owned by `serveUI`. Review the cancellation and close-before-join
ordering directly; retain existing desktop shutdown coverage without adding
a connected-peer test merely for the moved startup code.

### State and live-process transition

No database schema, preference schema, holder layout, or native conversation
migration is needed. Preserve the recorded JSON command bytes and command IDs.

| State at installation/restart | Required behavior |
| --- | --- |
| A holder or panel shell is running old code | New backend attaches using the unchanged lock, socket, output, terminal, and exit files. No forced restart. |
| An old plugin/native harness is running | It continues with its original protocol and native conversation. If it starts or restarts its MCP server, its saved `hypercube mcp` command still works through the small compatibility exec. |
| An old start command is pending in SQLite for an old plugin | Resend the original command line unchanged. The old plugin still sees `hypercube`; do not rewrite it to a field that old code cannot read. |
| An old saved start is used when a new plugin must be launched | The new plugin understands the old field through one shared start adapter, then supplies the recorded backend-plus-`mcp` command. |
| An old backend starts an agent or shell after its executable is replaced | Its `hypercube hold` call forwards to the sibling helper with the original arguments and inherited lock/listener files. New backends launch the helper directly. |
| A new launch is recorded | Write the helper path in `mcp` and the current environment. Do not write the legacy backend field. Launch the separate holder directly. |
| A saved native conversation is resumed or forked | The current plugin supplies its normal per-launch MCP config using the new command. Keep the native ID, fork behavior, instructions, and working directory. |

Retain `StartArgs.Hypercube` with `omitempty` and a comment limiting it to old
recorded start commands. Add a shared `plugin.MCPCommand` helper that accepts
`StartArgs` and returns the native adapter's executable/arguments, or a clear
error if neither executable field is usable. Prefer nonempty `MCP`, returning
that path without arguments; otherwise return `{Hypercube, "mcp"}`. Each
native config builder calls this one helper and retains its native format and
the existing environment. The fallback is local compatibility code, not
another field in the shared wire contract.

The scripted plugin has its own serve loop. Update `createSessions` to obtain
its command through the same helper and retain its existing environment.
Verify the actual tool call in a scene with an offer, child, or suggestion;
merely building the helper files would not exercise this path.

Both compatibility commands resolve their sibling helper using the canonical
executable directory and call `syscall.Exec` with unchanged environment and
stdio. The `hold` command also preserves its argument vector and inherited
lock/listener file descriptors 3 and 4. Neither imports the role's
implementation, starts backend services, opens state, or adds a process that
needs supervision.

The holder itself never re-executes `hypercube hold`, but that does not make
the old command unused: macOS installation replaces the bundle before
stopping the old backend, and development rebuilds replace executables before
restarting it. An old backend can launch a new agent or shell in that interval.
Retain the small `hold` forwarding command to prevent the new command mismatch.
This does not promise launches through an existing file-replacement gap or
change Linux's existing behavior when the running executable's directory is
moved. Keep installer ordering unchanged. Remove compatibility commands only
when supported callers no longer need them, not after an arbitrary release
count.

Keep the holder's inherited lock/listener file descriptors, Unix socket path
limit, process groups, environment, working directory, terminal handling,
output offsets/retention, exit-record ordering, and stop behavior unchanged.
Keep command acceptance, replay order, queued-message deletion, quick-exit
handling, parked sessions, and transcript application unchanged. SIGTERM
detaches the backend; SIGINT deliberately ends agents and shells; closing the
desktop app ends neither. Do not add version negotiation, stronger durability,
restart delivery guarantees, new IPC, or a process supervision subsystem.

### Packaging and operational paths

Build `hypercube`, both helpers, and the current harness plugins together in
`make binaries`. Ship helpers beside the backend inside both platform bundles.
Sign both helper executables explicitly in the macOS executable-signing step;
retain the backend's app identifier and existing outer-bundle signing order.
Do not give the helpers independent application identities or change native
privacy permissions.

Keep helpers bundle-private: existing `bin/hypercube` and installed CLI/plugin
links remain as they are. Canonical executable discovery must therefore work
when the backend is invoked through those links. Linux installation and
uninstallation continue to operate on the whole bundle and their existing
public links; add assertions for helper presence, rather than inventing extra
links and later cleanup. Release archives inherit the complete binaries target.

`make screenshots` must build the real two helpers in its temporary executable
directory beside the backend and scripted plugin. Keep the scripted plugin
out of release bundles. `scripts/dev` obtains helpers from `make binaries` or
the normal bundle target. Update its holder crash cleanup to recognize both
old `hypercube hold` and new `hypercube-hold` commands only for PIDs found under
that development state's run directory. No global process scanning/killing.

Service names, ports, `hypercube ui`, build identity output, backend log path,
login-shell environment, desktop service setup, installer replacement rules,
and user-facing browser behavior stay the same.

## Related code and expected file changes

| Files | Expected work |
| --- | --- |
| `cmd/hypercube/main.go`, new `backend.go`, `http.go`, `alerts.go`, and associated tests | Separate dispatch, startup/shutdown, HTTP composition, and alerts within the existing backend command. Add canonical sibling discovery and compatibility execs; move HTTP protection/version tests here and retain the real process integration. Remove holder implementation dispatch from test entry points. |
| New `cmd/hypercube-mcp/main.go`, `cmd/hypercube-hold/main.go` | Narrow executable entry points. |
| `internal/mcp/mcp.go`, `mcp_test.go`, `testmain_test.go`; new `internal/mcpbridge/` | Move stdio forwarding/retry and bridge tests out; keep schemas/backend tools and their current tool-specific files in `mcp`. |
| `internal/harness/{harness,discovery,protocol,image,config}.go` and tests; new `internal/harnessclient/`, `internal/config/` | Extract client/executable discovery and application prefs; keep pure shared data/validation; expose record decoding. Move tests with ownership and delete obsolete source files. |
| `internal/session/{runtime,launch,selection,sessions,children}.go`, fixtures and constructor callers | Receive explicit programs, call harness client/config owners, and record the direct MCP command. Preserve runtime state transitions. Search remaining callers rather than assuming this list exhausts them. |
| `internal/terminal/terminal.go` and tests | Explicit holder executable for new shells; existing adoption unchanged. |
| `internal/hold/{hold,holder,output}.go`, tests, and `holdtest/` | Explicit launch path and new command usage; replace test-binary holder dispatch with a real helper build. Preserve client/holder protocol and behavior. |
| `plugins/{claude,codex,opencode,pi}/main.go`, plugin tests; new `plugins/internal/plugin/mcp.go` and focused tests | Consume the executable path through one shared command adapter with legacy fallback; retain native configuration ownership. Check other fixtures constructing `StartArgs`. |
| `internal/ui/{ui,config}.go` and Go tests | Return only browser routes; import prefs from config; relocate composition/protection tests. Update runtime construction. No React redesign. |
| `plugins/scripted/main.go`, relevant integration scene | Update `createSessions` to obtain the MCP command from the shared adapter and retain its environment. Exercise a scene tool call without rendering screenshots. |
| `internal/store`, `internal/report`, `internal/desktop`, `internal/harness/harnesstest` | Inspect their final dependencies and update only constructor/type/comment/fixture references required by the extraction. Avoid unrelated behavioral changes. |
| `Makefile`, `make/{macos,linux}.mk`, `scripts/dev`, `scripts/screenshots/main.js` comments | Build/package/sign helpers; construct complete screenshot runtime; recognize both development holder process names. |
| `scripts/linux/test-{bundle,install,release}` | Verify helper layout and execution, symlink-backed startup, reinstall/restart survival, and bundle removal. Existing desktop tests continue to cover the unchanged UI/service flow. |
| `agents/{development,architecture,glossary}.md`, `CONTRIBUTING.md` | Describe actual command/package roles, private helper layout, build setup, and session restart requirements. See documentation scope below. |

The expected inventory is broad because the current boundaries are shared by
tests and every plugin. Moving unchanged code and changing imports is part of
the extraction; it does not authorize rewriting those unrelated concerns.

## Implementation plan

1. **Extract the existing package responsibilities.** Move preferences and
   their upgrades/tests to `internal/config`. Split discovery data from
   execution and move the latter plus the holder-backed client to
   `internal/harnessclient`. Export `harness.DecodeRecord`; keep validation
   identical. Update backend/session/browser callers. Confirm the shared
   harness package and production native plugins no longer depend on hold,
   config, session, store, or UI through the shared contract. Do not preserve
   forwarding aliases in the old package.
2. **Separate bridge transport from backend tools.** Move forwarding,
   identity-header injection, and refused-connection retry to
   `internal/mcpbridge`. Give it
   `Run(ctx, baseURL, sessionID, stdin, stdout)` with explicit inputs; the
   command owns environment lookup. Keep `internal/mcp.Handler` and tools in
   the backend. Use the stable `X-Hypercube-Session` literal at the two ends,
   documented as their wire contract; do not add a shared package just for
   this constant. Backend tool test clients add the header themselves and
   do not import the bridge to obtain a retrying transport. Temporarily route
   the existing main command's `mcp` case to the extracted bridge with explicit
   inputs; step 4 replaces that call with exec. Do not leave a forwarding alias
   in the backend tool package.
3. **Introduce explicit program paths and real helper entry points together.**
   Add both helper commands, change `hold.Start`/harness client/terminal shell
   launch to require the holder executable, and pass `session.Programs` from
   backend construction through runtime fixtures. Remove all self-execution
   defaults from those libraries. Add both helpers to `make binaries` and the
   screenshot construction path in this step so the intermediate tree can
   still start sessions and shells. Replace production `hypercube hold`
   implementation dispatch with the sibling-helper exec, preserving its
   arguments, environment, stdio, and inherited descriptors 3 and 4.

   Create the test-only helper here: build the real holder once per test
   process, without importing `hold` so hold's own tests can use it without a
   cycle. Give fixtures its explicit path and remove its executable directory
   when the test process ends. Two small files selected by `//go:build race`
   and `//go:build !race` choose whether its `go build` passes `-race`; ordinary
   focused tests gain no additional C-toolchain requirement. Preserve the
   current test-only `GORACE=atexit_sleep_ms=0` environment before launching
   holders so race-enabled subprocess exits do not each wait a second.

   Wire these fixtures and remove their fake production `hold` dispatch in
   this same step; retain intentional test orchestration for the
   parent-survival case. Adjust helper error messages/usage. The main command
   can still call separated bridge code until the next step switches it to
   exec. Step 7 only removes any replaced machinery left after this change.
4. **Switch plugin starts to the actual MCP command with the required
   transition.** Add the plain `MCP` executable field and shared plugin command
   adapter, update all four native config builders and the scripted plugin's
   `createSessions`, and write new starts using only the helper path and
   existing environment. Replace `hypercube mcp` implementation with the
   sibling-helper exec. Keep recorded command lines untouched. Test both wire
   forms before removing the old
   application-executable field from any newly created fixture/command. Do not
   remove its compatibility decoding.
5. **Separate backend concerns within the existing command.** Move startup,
   shutdown, and notifications out of `main.go` into focused files in
   `cmd/hypercube`, using the explicit programs already introduced. Move
   `/mcp`, `/desktop`, `/version`, and the single outer host/origin/frame
   wrapper from UI to the command's `http.go`. Leave browser routes and
   assets in UI. Move version/protection tests with their real owner and keep
   CLI process integration in the same package. Make alert cancellation and
   completion ownership visible; review the shutdown ordering without adding
   a connected-peer test or changing runtime restart behavior.
6. **Complete operational integration.** Sign/package the helpers on macOS,
   verify Linux bundle/install/release expectations, and update development
   cleanup for old and new holder names. Keep helpers out of public symlink
   lists. Confirm the real-executable-directory resolution supports normal
   installed and `bin/` symlinks. Inspect the release scripts and desktop
   setup flows; change only assumptions invalidated by helper packaging.
7. **Delete remaining replaced machinery.** Remove unused dispatch helpers,
   duplicate test setup, stale comments, and old fixture defaults left after
   step 3. The real-holder builder, race flag, environment, cleanup, and fixture
   wiring already exist; this step introduces no prerequisite for an earlier
   step. Retain hold's intentional parent-process survival test and
   harnesstest's native stand-in protocol. Avoid per-test compiler invocations,
   production helper environment overrides, or a new test framework.
8. **Update current references, validate, and prepare review.** Inspect whole
   touched files for remaining mixed concerns and remove dead imports,
   obsolete forwarding functions, old discovery defaults, and old self-exec
   comments. Update current architecture/development/contributor references,
   run the verification below, and record results and limitations in the work
   log. Hand the staged implementation to the fresh reviewer required by the
   development workflow before the parent commits it.

Each dependency-changing step includes its necessary fixture/import updates;
the final test step consolidates verification and removes remaining obsolete
machinery. Do not leave an intermediate commit that builds an executable but
cannot construct its required sibling helper.

## Acceptance criteria and verification

Use existing behavioral boundaries and a few focused regressions. This is an
architectural refactor with consequential process-lifetime and installation
behavior, so command/process and bundle tests are required; repeated tests of
unchanged internals are not.

### Static ownership checks

- Inspect `go list -deps` for each production helper and native plugin.
  `hypercube-mcp` has no local application dependencies except `mcpbridge`;
  `hypercube-hold` has no local dependency except `hold`; native plugins and
  shared harness data do not acquire hold, harnessclient, config,
  session, store, terminal, or UI through protocol imports.
- The backend command does not import `mcpbridge` or invoke `hold.Main`.
  UI does not import `mcp` or desktop; backend composes them. No library
  silently derives Hypercube helper/plugin paths from its own executable.
- Search for `os.Executable`, `hypercube hold`, `hypercube mcp`,
  `StartArgs.Hypercube`, and the old preference/discovery entry points.
  Remaining uses must be command-boundary resolution, intentional test
  orchestration, legacy start compatibility, or accurate current/history docs.
  No architecture-test framework or import parser is needed for these checks.

### Focused behavior

- Move current bridge coverage to `mcpbridge`, using a small HTTP MCP response
  fixture rather than importing backend packages. Cover forwarding a request
  and its matching response, initialization/notification behavior, identity
  header, EOF, backend errors, and the existing delayed-backend retry case.
  Command tests cover missing environment. Preserve current retry limits and
  message ordering rather than introducing concurrency or recovery behavior.
- Keep backend MCP scope/schema/tool assertions in `internal/mcp` and their
  tool-specific tests. Neither helper opens the application database. The
  holder continues to write its existing run files.
- Shared plugin command-adapter tests cover the new executable path taking
  precedence, old recorded-field fallback, and an unusable command's clear
  error. Existing native stand-ins assert each integration supplies the actual
  helper executable without arguments and retains the existing environment,
  without editing user settings or `HOME`.
- Exercise the scripted plugin's separate command transport in an existing
  backend/bundle integration scene containing an offer, child, or suggestion.
  Assert the resulting backend state. Prefer extending the Linux install
  scene with an offer, which exercises MCP without adding another agent
  process to its current PID assertions. No screenshot capture is needed.
- Keep holder behavior coverage for pipe and terminal input/output,
  reattachment/positions, retention/discard, final output/exit status,
  process groups, stopping, lost holder, and survival of the launcher parent.
  Run the actual `hypercube-hold`; a test program never acts as that helper.
  An invalid helper path fails rather than starting the test/backend binary.
- Extend the existing CLI process integration to build a complete sibling
  layout and start the backend through an external symlink to a bundle path
  containing spaces. Exercise direct `hypercube-mcp` and the legacy
  `hypercube mcp` path against it; both reach the same backend/session.
  SIGTERM and a replacement backend preserve the agent and panel-shell PIDs,
  state, transcript, and continued input. SIGINT ends them. Use the same
  backend address across this callback/restart check,
  because surviving native processes retain that address. Exercise the old
  configured `hypercube mcp` command after replacement as well as the direct
  helper. Keep existing native-conversation and queued-message assertions.
- Exercise `hypercube hold` forwarding in one process integration case with
  the real inherited lock/listener files, verifying the child can receive input
  and its retained output can be read. This covers the old backend's launch
  command after executable replacement without an old application binary or
  an install-race simulator. The main holder suite continues to run the helper
  directly.
- Add a narrow restart regression for an old serialized start command,
  preserving its original bytes/ID. Cover replay to an old-compatible
  stand-in and consumption by the new plugin start adapter. The existing
  restore tests plus unchanged holder-format assertions establish adoption;
  do not add a checked-in old application binary or general version matrix.
- Backend composition tests retain foreign host/origin rejection, `/mcp`'s
  origin exception with known session checks, browser/terminal/desktop socket
  protection, frame protection, route methods, and `/version` behavior.
  Backend shutdown cancels the alert watch and closes the desktop connection
  before joining it, then closes the database. Review this ordering directly
  and retain the existing desktop shutdown tests; add no connected-peer or
  backpressure test solely for this code movement.

### Required commands and smoke tests

1. Use focused Go tests while extracting each boundary. Existing browser tests
   remain necessary where their server constructor/routes change; no new
   React tests are needed for unchanged browser behavior.
2. Run `make check` once the implementation is complete, as required before
   committing code. It builds all `cmd/...` and `plugins/...`, checks generated
   assets, validates Go/TypeScript/desktop behavior, and runs Go tests under
   the race detector. Fix failures and repeat only affected checks until a
   final successful required check is recorded.
3. Build the macOS bundle with the existing icon outputs when available;
   confirm both helpers are executable, independently signed, and covered by
   `codesign --verify --deep --strict`. Smoke isolated `make dev` startup and
   a rebuild with both helpers present; the command integration tests already
   exercise agent and shell survival with stand-ins. Normal `scripts/dev`
   intentionally strips external scene variables, so do not assume exporting
   `HYPERCUBE_SCENE` makes it run the scripted plugin. Clean up what the test
   starts. Do not install into the user's running application merely to
   validate this refactor.
4. Run `scripts/linux/run scripts/linux/test-bundle` and
   `scripts/linux/run scripts/linux/test-install`. Extend their normal checks
   for private helper presence, symlink invocation, and agent survival across
   service restart/reinstall. Run
   `scripts/linux/run scripts/linux/test-release` because a missing helper in
   an extracted archive is an installation failure. The current desktop suite
   remains in normal CI; run it locally only if desktop/installer behavior
   changes beyond bundle contents. Use the existing distribution matrix in CI
   rather than repeating every distribution locally without a failure to
   investigate.
5. Inspect the temporary screenshot build recipe and verify its complete
   executable layout. Do not regenerate screenshot images: this change has no
   visible UI change. Do not run tagged release publishing or app installation.

If the host cannot perform a required platform check, state that omission and
its cause explicitly in the work log/review; do not claim verification from
source inspection. No tests or builds are required for this planning document.

## Documentation updates and completion

Read `agents/documentation.md` before updating current guides. Update the
architecture's process descriptions and protocol links, development's code
layout and restart guidance, the glossary's holder definition, and the
contributor build/layout explanation. Keep service commands and public usage
unchanged. Update current source/script comments to name the actual helper
roles. Historical plans/reviews/work logs remain historical and need no
mass rewrite. The existing architecture image already shows separate runtime
roles; only regenerate it if its actual labels become inaccurate.

Before implementation starts, warn that installed, already running agents and
shells continue using the code they started with. They survive the update;
loading new plugin/helper behavior requires restarting the affected session
or shell. Repeat that consequence when offering an installation after the
implementation is complete. No restart, installation, or implementation is
authorized merely by writing this plan.

Stop when the three roles have the stated executables and package ownership,
all required behavior and transitions pass the proportionate checks, obsolete
coupling is deleted, and touched files are easy to follow. Do not expand this
work into session-runtime decomposition, native harness integration rewrites,
new guarantees, or unrelated frontend/storage cleanup.
