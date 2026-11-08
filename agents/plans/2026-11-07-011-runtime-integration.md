# Ship the MCP bridge and holder executables

## Goal

After the earlier plans, the backend finds `hypercube-mcp` and
`hypercube-hold` beside its real executable, but only tests build them. This
task builds and ships them everywhere `hypercube` is built: `make binaries`
(and through it `make dev`, both bundles, and releases) and the screenshot
build. macOS signs them. Development crash cleanup recognizes the new holder
command. The Linux bundle, install, and release tests check the helpers, and
the architecture, glossary, and development guides describe the separate
executables.

The helpers are private to the bundle: `bin/` and `~/.local/bin` keep links
to `hypercube` and the plugins only.

Prerequisites: the [configuration](2026-11-07-006-application-config.md),
[protocol](2026-11-07-007-harness-protocol-boundary.md),
[MCP bridge](2026-11-07-008-mcp-bridge-executable.md),
[holder](2026-11-07-009-holder-executable.md), and
[backend composition](2026-11-07-010-backend-composition.md) plans.

## Related code

- `Makefile`:
  - `binaries` — `go build -o $(OUT)/hypercube ./cmd/hypercube` and the
    plugin loop. `make/macos.mk` and `make/linux.mk` call it for the
    bundles; `scripts/dev` calls it with `OUT=bin/dev` (line 144).
  - `screenshots` — builds `$$tmp/hypercube` and the scripted plugin
    directly, without `binaries`.
- `make/macos.mk`:
  - `bundle` comment: "with hypercube and the harness plugins side by side
    in Contents/MacOS".
  - Signing: a loop signs each `hypercube-harness-*`; `codesign --deep`
    signs the app; `hypercube` is signed with `--identifier $(APP_ID)`
    ("The service is the process macOS asks for privacy permissions, which it
    grants to the app's identifier"); then the app is signed again.
  - `build` and `place` link `hypercube` and the plugins only.
- `make/linux.mk` — `bundle` comment: "with hypercube and the harness plugins
  side by side at the top"; `build` links `hypercube` and the plugins.
- `scripts/linux-install` — links `$app/hypercube` and
  `$app/hypercube-harness-*` into `$BINDIR`; `scripts/linux-uninstall`
  removes those links. Neither needs a change.
- `scripts/dev` `stop_holders` (line 72) — for each `run/*/pid` in the dev
  state, kills the process when its command contains `hypercube hold `.
- `scripts/screenshots/main.js:6` — "The folder holds hypercube and the
  scripted harness plugin".
- `scripts/linux/test-bundle` — line 22 checks the bundle's executables;
  line 69 starts the backend as `"$app/hypercube" ui` and checks
  `/harnesses`.
- `scripts/linux/test-install` — line 78 checks installed executables;
  lines 99-112 start a scripted agent and check its holder PID survives
  restarts and reinstalls.
- `scripts/linux/test-release` — line 66 checks the plugin links after
  installing the extracted archive.
- `agents/architecture.md:55` (`hypercube hold`), `:76` (`hypercube mcp`);
  `agents/glossary.md:52` (holder); `agents/development.md:180-181` (which
  changes reach running sessions only after a restart).

## Decisions

### Building

```make
# binaries builds hypercube, the MCP bridge and holder it runs agents with,
# and the harness plugins side by side in $(OUT).
binaries:
	for c in hypercube hypercube-mcp hypercube-hold; do go build -o $(OUT)/$$c ./cmd/$$c || exit 1; done
	for p in $(PLUGINS); do go build -o $(OUT)/hypercube-harness-$$p ./plugins/$$p || exit 1; done
```

`screenshots` builds the same three commands into `$$tmp` with the same
loop, before the scripted plugin. `check` already builds `./cmd/...`.

### Signing on macOS

The helpers are signed before the `--deep` app signature, in the loop that
signs the plugins:

```make
	for f in hypercube-mcp $(PLUGINS:%=hypercube-harness-%); do codesign --force --sign "$(SIGN_IDENTITY)" $(MACOS)/$$f || exit 1; done
```

`hypercube-hold` is signed with the backend's identifier, after the `--deep`
signature, next to `hypercube`:

```make
	# The service is the process macOS asks for privacy permissions, which it
	# grants to the app's identifier. Holders run the agents and shells, so
	# they carry the same identifier.
	codesign --force --sign "$(SIGN_IDENTITY)" --identifier $(APP_ID) $(MACOS)/hypercube
	codesign --force --sign "$(SIGN_IDENTITY)" --identifier $(APP_ID) $(MACOS)/hypercube-hold
	codesign --force --sign "$(SIGN_IDENTITY)" $(APP)
```

Until now every holder was the `hypercube` executable, signed with
`$(APP_ID)`. Signing `hypercube-hold` the same way keeps the code identity
of the process that runs agents and panel shells, so privacy permissions
granted to Hypercube keep applying to them.

### Development cleanup

`stop_holders` matches both command forms, because holders left by a dev
backend built before this change run `hypercube hold`:

```bash
		case "$(ps -p "$pid" -o command=)" in
			*"hypercube-hold "*|*"hypercube hold "*) kill -TERM "$pid" 2>/dev/null || true; pids+=("$pid");;
		esac
```

### Installing this change

This change alters how agents and shells launch. Running sessions keep their
current holder and MCP command until they restart, and keep working. When
offering `make install-service`, say that sessions pick up the new launch
when they next start.

## Test plan

- `make binaries OUT=/tmp/hc-bin` writes `hypercube`, `hypercube-mcp`,
  `hypercube-hold`, and the four plugins.
- macOS, `make build`:
  - `bin/Hypercube.app/Contents/MacOS` has the three executables and the
    plugins; `bin/` has links to `hypercube` and the plugins only.
  - `codesign -dv` reports `Identifier=com.github.kkestell.hypercube` for
    `hypercube` and `hypercube-hold`, and `codesign --verify --deep --strict
    bin/Hypercube.app` passes.
  - `make dev` starts the dev backend from `bin/dev`. Opening a workspace's
    panel shell in the dev page starts a holder whose command starts with
    `<repo>/bin/dev/hypercube-hold`. Changing a Go file's contents rebuilds
    and restarts the backend, and the shell keeps its PID and scrollback.
    `scripts/dev stop` ends both. If an existing dev server has unknown
    ownership, use a disposable source copy, state directory, and port so
    its watcher and workspaces are untouched. The watcher hashes contents,
    so touching a file alone does not trigger a rebuild.
  - With temporary dev state and HOME, `scripts/dev clean` stops actual
    holders launched under both `hypercube hold` and `hypercube-hold`.
- Linux, in the container:
  - `scripts/linux/test-bundle` checks `hypercube-mcp` and `hypercube-hold`
    in the bundle's executable list (line 22) and starts the backend through
    the `bin/hypercube` link instead of `$app/hypercube`, so `/harnesses`
    listing the plugins shows that a symlinked backend finds the bundle.
  - `scripts/linux/test-install` checks both helpers in the installed
    executable list (line 78), and after the first turn checks that
    `ps -o command= -p "$holder"` starts with `$app/hypercube-hold`. The
    existing restart and reinstall checks keep that holder alive.
  - `scripts/linux/test-release` checks `$app/hypercube-mcp` and
    `$app/hypercube-hold` are executable after installing the archive.
  - Run `scripts/linux/run scripts/linux/test-bundle`,
    `scripts/linux/run scripts/linux/test-install`, and
    `scripts/linux/run scripts/linux/test-release` on the Mac. CI runs them
    across the distributions.
- `make screenshots` is not rerun; read the recipe and confirm it builds the
  three commands.
- `make check` passes.
- Final dependency check: `go list -deps` of `./cmd/hypercube-mcp` lists only
  `internal/mcpbridge` from this module, `./cmd/hypercube-hold` only
  `internal/hold`, and the four production plugins no `hold`,
  `harnessclient`, `config`, `session`, `store`, or `ui`.
- Whether macOS applies the backend's privacy permissions to agents under
  the newly signed holder shows only in an installed app. Report it as
  unverified unless the user installs the build and checks an agent's access
  to a protected folder such as `~/Documents`.

## Implementation plan

1. Update `binaries` and `screenshots` in the `Makefile`, and the comment in
   `scripts/screenshots/main.js`.
2. Update `make/macos.mk`: the `bundle` comment and the signing lines above.
   Update the `bundle` comment in `make/linux.mk`.
3. Update `stop_holders` in `scripts/dev`.
4. Update `scripts/linux/test-bundle`, `test-install`, and `test-release`.
5. Update the documentation.
6. Run the test plan.

## Documentation updates

- `agents/architecture.md`:
  - Line 55: "An agent plugin runs under a `hypercube-hold` process that owns
    its input and output independently of the app."
  - Line 76: "Each harness starts `hypercube-mcp` as a stdio MCP server."
  - In the backend section, one sentence: the backend finds its plugins,
    `hypercube-mcp`, and `hypercube-hold` beside its real executable, and
    `hypercube mcp` and `hypercube hold` run the same code for processes
    started by earlier versions.
- `agents/glossary.md:52`: "**holder**: A detached `hypercube-hold` process
  that owns ...".
- `agents/development.md:180-181`: "A change to the harness plugins,
  `hypercube-hold`, `hypercube-mcp`, the plugin protocol, or how agents and
  shells launch reaches an active session only after that session restarts."
