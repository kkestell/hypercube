# MCP bridge executable

## Goal

Each agent's harness runs `hypercube mcp`, a stdio MCP server that forwards
requests to the backend's `/mcp` route. Two problems follow from how that is
built today:

- The forwarding code lives in `internal/mcp` beside the backend's tool
  handlers, so the bridge imports `session` and `store` and, through them,
  SQLite, workspaces, terminals, and holders.
- The backend sends plugins its own executable path (`StartArgs.Hypercube`),
  found with `os.Executable()` inside `session`, and every plugin appends
  `mcp`. Plugins depend on the backend's command line, and a library finds a
  program by asking which program it is running in.

When this task is done:

- `internal/mcpbridge` forwards stdio to HTTP and imports no Hypercube
  package. `cmd/hypercube-mcp` runs it.
- `internal/mcp` holds the tools and the `/mcp` handler.
- The backend command finds its plugins and `hypercube-mcp` beside its real
  executable and passes them to `session.New` as `session.Programs`.
- New start commands carry `StartArgs.MCP`, the bridge's path. Plugins build
  the harness's MCP configuration through one shared function that also
  accepts starts saved by earlier versions.
- `hypercube mcp` still works, calling the same bridge code, for native
  harnesses that were configured with it before an upgrade.

Prerequisites: [application configuration](2026-11-07-006-application-config.md)
and [harness protocol boundary](2026-11-07-007-harness-protocol-boundary.md).
Addresses review findings F2 and F3. Building `hypercube-mcp` into bundles
and development builds is part of
[runtime integration](2026-11-07-011-runtime-integration.md); until then,
tests build it themselves.

## Related code

- `internal/mcp/mcp.go`:
  - `newServer`, `Handler`, `listSessions`, `sendMessage` — tools; stay.
  - `SessionHeader` — the `X-Hypercube-Session` header `Handler` reads; stays.
  - `Bridge(stdin, stdout)` — reads `HYPERCUBE_SESSION_ID` and
    `HYPERCUBE_URL`, connects an `mcp.StreamableClientTransport` to
    `<url>/mcp` and an `mcp.IOTransport` to stdio, and forwards one request
    at a time; a failed write of a call answers that call with a JSON-RPC
    internal error.
  - `identity` — a `RoundTripper` that sets `SessionHeader` and retries a
    refused connection every 250 ms for 30 s, so a bridge survives a backend
    restart.
  - `nopCloser`.
- `internal/mcp/mcp_test.go`:
  - `connect` (line 92) uses `identity(id)` as the test client's transport.
  - `TestBridge` (213) and `TestBridgeWaitsForRestartingApp` (580) run
    `Bridge` against the real tools.
  - `family` (30) returns `session.New(db, "")`; `started` (284) sets
    `rt.PluginDir` to a directory with stand-ins `one` and `two`.
- `cmd/hypercube/main.go` — `case ... "mcp": err = mcp.Bridge(os.Stdin,
  os.Stdout)`; `startApp(listener, plugins string, desktop)` sets
  `rt.PluginDir = plugins`; `serveUI` passes `""`.
- `internal/session/runtime.go` — exported `PluginDir` ("Empty means beside
  the running executable"), `New(db, url)`.
- `internal/session/launch.go:209,217-231` — `pluginPath` and `startArgs`,
  which calls `os.Executable()` and sets `StartArgs.Hypercube`.
- `internal/session/sessions.go:64`, `selection.go:16,41` — pass
  `r.PluginDir` to `harnessclient`.
- `internal/harnessclient/discovery.go` — `pluginDir` falls back to
  `os.Executable()` when the directory is empty.
- `internal/harness/protocol.go:73-76` — `StartArgs.Hypercube` (`json:"hypercube"`)
  and `Env`.
- `internal/session/commands.go` — saves each command's complete JSON line and
  resends unanswered lines when it reattaches. Starts saved before this
  change contain `"hypercube"`.
- Plugins that build the MCP command:
  - `plugins/claude/main.go:71` — `--mcp-config` JSON:
    `{"command": a.Hypercube, "args": ["mcp"], "env": a.Env}`.
  - `plugins/codex/main.go:158` — `mcp_servers.hypercube` in thread config,
    same shape plus `default_tools_approval_mode`.
  - `plugins/opencode/main.go:89` — `"command": [a.Hypercube, "mcp"]`,
    `"environment": a.Env`.
  - `plugins/pi/main.go:112` — `HYPERCUBE_MCP` JSON, same shape as Claude
    plus `"exposure": "direct"`.
  - `plugins/scripted/main.go:276` — `exec.Command(start.Hypercube, "mcp")`
    in `createSessions`.
- Plugin tests with `Hypercube:` start arguments and expected configuration:
  `plugins/claude/main_test.go:616`, `plugins/codex/main_test.go:1166,1179`,
  `plugins/pi/main_test.go:691,742`, `plugins/opencode/native_test.go:135`.
- `internal/harness/harnesstest/harnesstest.go:80,282` — the stand-in logs
  each start as an `Entry`.
- Runtime fixtures: `internal/session/fixture_test.go:72`
  (`newRuntime`), `internal/ui/ui_test.go:56,277`,
  `internal/ui/terminal_test.go:39`, `internal/mcp/mcp_test.go:56,290`,
  `cmd/hypercube/integration_test.go:244`.
- `cmd/hypercube/integration_test.go` `TestAppProcess` — builds the backend
  into the plugin directory with `go build -o <plugins>/hypercube .`, starts
  it directly, and calls tools through `hypercube mcp`.

## Decisions

### `internal/mcpbridge`

`internal/mcpbridge/bridge.go`:

```go
// Package mcpbridge is the stdio MCP server each agent's harness runs. It
// forwards every message to the Hypercube backend's /mcp route with the
// agent's session ID and writes the backend's answers back. The SDK has no
// stdio-to-HTTP forwarder, so Run moves messages between the SDK's stdio and
// streamable HTTP transports itself, one request at a time.
package mcpbridge

// sessionHeader names the session a request comes from. The backend's /mcp
// handler (internal/mcp.SessionHeader) reads the same header.
const sessionHeader = "X-Hypercube-Session"

// Main runs the bridge for the session in HYPERCUBE_SESSION_ID against the
// backend at HYPERCUBE_URL until stdin closes.
func Main(stdin io.Reader, stdout io.Writer) error

// Run forwards MCP messages from stdin to the backend at url for session
// until stdin closes or ctx ends.
func Run(ctx context.Context, url, session string, stdin io.Reader, stdout io.Writer) error
```

`Main` returns the existing error, `HYPERCUBE_SESSION_ID and HYPERCUBE_URL
must be set`, when either is empty, and otherwise calls
`Run(context.Background(), ...)`. `Run` is the body of today's `Bridge` after
the environment lookup. `identity` and `nopCloser` move unchanged, with
`identity` setting `sessionHeader`. The package imports the standard library
and `github.com/modelcontextprotocol/go-sdk`.

`internal/mcp` keeps `SessionHeader`, `Handler`, and the tools. Its package
comment becomes:

```go
// Package mcp serves the Hypercube tools to agents at the backend's /mcp
// route. Each agent's harness runs hypercube-mcp (internal/mcpbridge), which
// forwards requests here with the session ID in SessionHeader. The tools run
// in the backend, beside the browser's actions.
```

### The two commands

`cmd/hypercube-mcp/main.go`:

```go
// Command hypercube-mcp is the stdio MCP server each agent's harness runs. It
// forwards requests to the Hypercube backend at HYPERCUBE_URL for the session
// in HYPERCUBE_SESSION_ID.
package main

func main() {
	err := errors.New("usage: hypercube-mcp")
	if len(os.Args) == 1 {
		err = mcpbridge.Main(os.Stdin, os.Stdout)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hypercube-mcp:", err)
		os.Exit(1)
	}
}
```

`cmd/hypercube/main.go` keeps `hypercube mcp`, now calling
`mcpbridge.Main(os.Stdin, os.Stdout)`. Native harnesses started before an
upgrade have `<hypercube> mcp` in their configuration and may start it again
after the bundle is replaced, so the command stays, with the same code as
the new executable. The usage line is unchanged.

### Programs

`internal/session/runtime.go`:

```go
// Programs are the executables a runtime starts. Plugins is the directory of
// harness plugins. MCP is the MCP bridge that each agent's harness runs.
type Programs struct {
	Plugins string
	MCP     string
}

// New returns a runtime for the database. url is where agents reach the
// backend.
func New(db *store.DB, url string, programs Programs) *Runtime
```

`Runtime` stores it in a private `programs` field and the exported
`PluginDir` field is removed. `pluginPath`, `sessions.go`, and
`selection.go` pass `r.programs.Plugins`. `harnessclient.Plugins` and
`harnessclient.Find` use the directory they are given; `pluginDir` and its
`os.Executable()` fallback are deleted, along with "or beside the running
executable when dir is empty" in their comments.

The backend command finds the programs once, in a new file
`cmd/hypercube/programs.go`:

```go
// programs returns the plugins and helpers beside the real hypercube
// executable. bin/hypercube and installed CLI links are symlinks into the
// bundle, so the path is resolved first.
func programs() (session.Programs, error) {
	exe, err := os.Executable()
	if err != nil {
		return session.Programs{}, err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return session.Programs{}, err
	}
	dir := filepath.Dir(exe)
	return session.Programs{Plugins: dir, MCP: filepath.Join(dir, "hypercube-mcp")}, nil
}
```

`startApp(listener net.Listener, programs session.Programs, desktop
*desktop.Desktop)` passes them to `session.New`; its comment drops "or
beside the executable when it is empty". `serveUI` calls `programs()` and
returns its error.

Today plugins are found through `os.Stat`, which follows the plugin links
`make build` puts in `bin/`. After this change the backend reads the bundle
directory itself, where the plugins are regular files. Native harness CLIs
are still found on the user's `PATH` by the plugins.

### Start arguments

`internal/harness/protocol.go`:

```go
type StartArgs struct {
	// MCP is the MCP bridge executable the harness runs, with Env, to give
	// the agent its Hypercube tools.
	MCP string `json:"mcp,omitempty"`
	// Hypercube is set instead of MCP in starts saved by earlier versions:
	// the backend executable, which the harness ran with the argument mcp.
	Hypercube string `json:"hypercube,omitempty"`
	// Env is the environment for the MCP bridge.
	Env map[string]string `json:"env"`
	...
}
```

`startArgs` sets `MCP: r.programs.MCP` and no longer calls `os.Executable()`.
Saved command lines are resent byte for byte, so a start saved before the
upgrade still names `hypercube`. A plugin reads such a start when the backend
relaunches a session whose launch directory is gone.

`plugins/internal/plugin/plugin.go`:

```go
// MCPCommand returns the executable and arguments a harness runs for the
// Hypercube MCP server: the bridge, or for a start saved by an earlier
// version, the backend's mcp command.
func MCPCommand(a harness.StartArgs) (string, []string, error) {
	switch {
	case a.MCP != "":
		return a.MCP, []string{}, nil
	case a.Hypercube != "":
		return a.Hypercube, []string{"mcp"}, nil
	}
	return "", nil, errors.New("the start names no MCP server")
}
```

The arguments are an empty slice, not nil, so the JSON configurations write
`"args": []`. Each plugin calls `MCPCommand` at the place listed in Related
code and returns its error from the start:

- Claude, Codex, Pi: `"command": command, "args": args`.
- OpenCode: `"command": append([]string{command}, args...)`.
- Scripted: `exec.Command(command, args...)`.

`Env` is used exactly as today.

## Test plan

- `internal/mcpbridge/bridge_test.go` runs against its own MCP backend, an
  `httptest.Server` with `mcp.NewStreamableHTTPHandler` (stateless, JSON
  responses) serving one tool, `whoami`, which answers with the request's
  `X-Hypercube-Session` header, and answering 403 for any session other than
  `s`:
  - `TestRun` — the existing script (initialize, `notifications/initialized`,
    a `tools/call` of `whoami`, `no/such`) through `Run` for session `s`
    produces three answers with IDs 1, 2, and 3; the call answers `s`; the
    unknown method answers an error.
  - `TestRunAnswersEachRequestWhenRefused` — session `nobody` and the URL
    `:invalid` each produce an error answer for every call, with the call's
    ID, as `TestBridge` checks today.
  - `TestRunWaitsForRestartingBackend` — moved from
    `TestBridgeWaitsForRestartingApp`: the backend's port opens 300 ms after
    `Run` starts and the call is answered.
  - `TestMainNeedsEnvironment` — `Main` with `HYPERCUBE_URL` empty fails.
- `internal/mcp` tool tests pass with `connect` using a test-local transport
  that sets `SessionHeader`, with `TestBridge` and
  `TestBridgeWaitsForRestartingApp` removed.
- `plugins/internal/plugin` `TestMCPCommand` — `MCP` set returns it with `[]`
  even when `Hypercube` is also set; only `Hypercube` returns it with
  `["mcp"]`; neither returns an error.
- Plugin start tests use `MCP: "/bin/hc-mcp"` and expect
  `"args":[],"command":"/bin/hc-mcp"` (Codex, Pi), the same values in
  Claude's `--mcp-config`, and the wrapper script as OpenCode's `MCP`. One
  Claude case keeps `Hypercube: "/bin/hc"` and expects `"args":["mcp"]`.
- The stand-in logs `MCP` with each start (`Entry.MCP`, `json:"mcp,omitempty"`).
  A session test checks that a start from a runtime built with
  `Programs{MCP: "/x/hypercube-mcp"}` logs that path.
- `TestAppProcess`:
  - Builds both executables into the plugin directory:
    `go build -o <plugins>/ . ../hypercube-mcp`.
  - Starts the backend through a symlink in another directory,
    `<dir>/bin/hypercube -> <plugins>/hypercube`. Creating the parent session
    shows the backend found the `test` plugin beside its real executable.
  - Checks that the parent's logged start has `MCP` equal to
    `<plugins>/hypercube-mcp`.
  - Calls `create_session` through `hypercube-mcp` (as it does today through
    `hypercube mcp`) and `list_sessions` through `hypercube mcp`, which lists
    `w1`.
- `go list -deps ./cmd/hypercube-mcp | grep kkestell/hypercube` lists only
  `cmd/hypercube-mcp` and `internal/mcpbridge`.
- `grep -rn 'os.Executable' --include='*.go' internal plugins` finds only
  `internal/hold/hold.go` (removed by the holder plan) and tests.

## Implementation plan

1. Create `internal/mcpbridge/bridge.go` with `Main`, `Run`, `identity`,
   `nopCloser`, and `sessionHeader` from `internal/mcp/mcp.go`, and
   `bridge_test.go` as in the test plan. Delete `Bridge`, `identity`, and
   `nopCloser` from `internal/mcp`, update its package comment, and remove
   the imports it no longer uses (`errors`, `io`, `os`, `syscall`, `time`,
   and the SDK's `jsonrpc`).
2. In `internal/mcp/mcp_test.go`, replace `identity(id)` in `connect` with a
   test-local `withSession` transport and delete `TestBridge` and
   `TestBridgeWaitsForRestartingApp`.
3. Add `cmd/hypercube-mcp/main.go`. Change `hypercube mcp` in
   `cmd/hypercube/main.go` to call `mcpbridge.Main`, and update the package
   comment to say `hypercube mcp` serves harnesses configured by earlier
   versions.
4. Add `session.Programs`, the `programs` field, and the `New` parameter;
   remove `PluginDir`. Replace `r.PluginDir` in `launch.go`, `sessions.go`,
   and `selection.go`. Delete `pluginDir` from `harnessclient/discovery.go`.
5. Add `cmd/hypercube/programs.go`, change `startApp`'s parameter, and call
   `programs()` from `serveUI`.
6. Update the runtime fixtures:
   - `internal/session/fixture_test.go` `newRuntime`:
     `New(f.db, "http://127.0.0.1:1", Programs{Plugins: f.plugins})`.
   - `internal/mcp/mcp_test.go` `family`: create `<dir>/plugins`, install the
     `one` and `two` stand-ins there, and pass `Programs{Plugins: ...}`;
     `started` stops setting `PluginDir`. Check that the refusal tests still
     fail for the reasons they assert.
   - `internal/ui/ui_test.go:277`: `session.Programs{Plugins: plugins}`.
   - `internal/ui/ui_test.go:56`, `internal/ui/terminal_test.go:39`,
     `cmd/hypercube/integration_test.go:244`: `session.Programs{}`; these
     runtimes start no agents.
7. Add `StartArgs.MCP`, make `Hypercube` `omitempty`, and set `MCP` in
   `startArgs`. Add `Entry.MCP` to the stand-in and log it.
8. Add `plugin.MCPCommand` and its test, and use it in the Claude, Codex,
   OpenCode, Pi, and scripted plugins. Update the plugin tests listed in
   Related code.
9. Update `TestAppProcess` as described in the test plan.
10. Update the documentation and run `make check`.

## Documentation updates

- `agents/development.md`, Code: add `mcpbridge` to the `internal/` package
  list, and list `cmd/hypercube-mcp/` — the MCP bridge each agent's harness
  runs.
- The architecture, glossary, and restart guidance that name `hypercube mcp`
  are updated in [runtime integration](2026-11-07-011-runtime-integration.md),
  when bundles contain the new executable.
