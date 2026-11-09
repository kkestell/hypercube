# Holder executable

## Goal

`hold.Start` launches a holder by running `os.Executable()` with the argument
`hold`. Any program that calls it must therefore also implement Hypercube's
`hold` command. Five test packages meet that requirement by making their test
binary act as the holder (`holdtest.Main`), and `internal/hold`'s own tests
repeat the same dispatch.

When this task is done:

- `cmd/hypercube-hold` runs `hold.Main`.
- `hold.Start` takes the holder executable as its first argument and runs it.
- The backend command finds `hypercube-hold` beside its real executable and
  passes it through `session.Programs` to agent launches and panel shells.
- Tests build the real `hypercube-hold` once per test binary and pass its
  path; no test binary acts as a holder.
- `hypercube hold` still calls `hold.Main`, for backends of earlier versions
  that launch holders while their bundle is being replaced.

Prerequisite: [MCP bridge executable](2026-11-07-008-mcp-bridge-executable.md),
which added `session.Programs` and `cmd/hypercube/programs.go`. Addresses
review findings F1 and F6. Bundles and development builds gain
`hypercube-hold` in [runtime integration](2026-11-07-011-runtime-integration.md).

## Related code

- `internal/hold/hold.go:54` — `Start(dir, argv, env, cwd, size)`: checks the
  socket path length, creates and locks `lock`, listens on `in`, then runs
  `os.Executable()` with `hold [-terminal -cols N -rows N] <dir> <argv...>`,
  passing the lock and listener as files 3 and 4, with `Setsid`. On a start
  failure it removes `in`.
- `internal/hold/holder.go:24` — `Main(args)`: flag set named `hold`, usage
  `usage: hypercube hold [-terminal -cols N -rows N] <dir> <command>
  [args...]`.
- `internal/hold/holdtest/holdtest.go` — `Main(m)`: runs `hold.Main` when the
  test binary is started as `hold`; otherwise sets
  `GORACE=atexit_sleep_ms=0` (so race-enabled holders exit at once) and runs
  the tests. Called from the `TestMain` of `cmd/hypercube`, `internal/mcp`,
  `internal/session`, `internal/terminal`, and `internal/ui`.
- `internal/hold/testmain_test.go` — a `parent` mode that calls `Start` and
  sleeps (for `TestSurvivesKilledParent`), its own `hold` dispatch, and the
  same `GORACE` setting.
- `internal/hold/hold_test.go` — `start` helper (line 28), `TestLongSocketPathRefused`,
  `TestSurvivesKilledParent` (line 262, runs the test binary as `parent`).
- `internal/harnessclient/client.go` — `Launch(dir, plugin)` calls
  `hold.Start(dir, []string{plugin, "serve"}, os.Environ(), "", nil)`;
  `Attach(dir)` calls `hold.Open`.
- `internal/session/launch.go:138` — `harnessclient.Launch(dir, path)`.
- `internal/session/runtime.go` — `New` builds `Terminals:
  terminal.Set{Dir: filepath.Join(store.Dir(), "run")}`.
- `internal/terminal/terminal.go` — `Set{Command, Dir}`; `Open` adopts a live
  shell or calls `hold.Start` (line 68) for a new one; `Adopt` only attaches.
- `internal/session/restart_test.go:230` — starts an orphan holder directly
  with `hold.Start`.
- `internal/terminal/terminal_test.go:22,82` — build `Set{Dir, Command}`.
- `cmd/hypercube/main.go` — `case ... "hold": err = hold.Main(os.Args[2:])`.
- `cmd/hypercube/programs.go` — `programs()` from the MCP plan.
- `cmd/hypercube/integration_test.go` `TestAppProcess` — builds `hypercube`
  and `hypercube-mcp` into the plugin directory.

## Decisions

### The command and `Start`

`cmd/hypercube-hold/main.go`:

```go
// Command hypercube-hold runs one child process independently of the
// Hypercube backend. It owns the child's input socket, retained output, and
// exit record in a holder directory; internal/hold's Start runs it.
package main

func main() {
	if err := hold.Main(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "hypercube-hold:", err)
		os.Exit(1)
	}
}
```

`hold.Main`'s flag set is named `hypercube-hold` and its usage error reads
`usage: hypercube-hold [-terminal -cols N -rows N] <dir> <command> [args...]`.

```go
// Start runs holder, the hypercube-hold executable, detached, to start argv
// with the environment and working directory in the holder directory dir.
// size starts it on a terminal of that size.
func Start(holder, dir string, argv []string, env []string, cwd string, size *Size) (*Holder, error)
```

The arguments become `[-terminal -cols N -rows N] <dir> <argv...>`, without
`hold`. Nothing else in `Start` changes: the socket length check, lock,
listener, inherited files 3 and 4, `Setsid`, environment, and removal of `in`
when the holder cannot start. A missing executable fails with the
`exec` error, which reaches the session's transcript as "Could not start the
agent: ..." as other start failures do.

`hypercube hold` keeps calling `hold.Main`. During `make install-service`
on macOS and during `scripts/dev` rebuilds, the new executable replaces the
old one before the old backend stops, and an agent or shell launched in that
window runs the new `hypercube` with `hold`. The usage string in
`cmd/hypercube/main.go` is unchanged; the package comment says `hypercube
hold` serves backends of earlier versions.

### Passing the holder

```go
// Programs are the executables a runtime starts. Plugins is the directory of
// harness plugins. MCP is the MCP bridge that each agent's harness runs.
// Holder is hypercube-hold, which runs agents' plugins and panel shells.
type Programs struct {
	Plugins string
	MCP     string
	Holder  string
}
```

- `programs()` sets `Holder: filepath.Join(dir, "hypercube-hold")`.
- `session.New` builds `Terminals: terminal.Set{Dir: ..., Holder:
  programs.Holder}`.
- `harnessclient.Launch(holder, dir, plugin string)`; `run` in
  `launch.go` passes `r.programs.Holder`.
- `terminal.Set` gains `Holder string` ("the hypercube-hold executable that
  runs new shells"); `Open` passes it to `hold.Start`.
- `hold.Open`, `harnessclient.Attach`, and `Set.Adopt` attach to running
  holders and take no executable.

### Test holder

`internal/hold/holdtest` builds the real command. It imports no Hypercube
package, so `internal/hold`'s own tests can use it.

```go
// Package holdtest builds hypercube-hold for tests that start holders.
package holdtest

// Path is the hypercube-hold executable that Main built.
var Path string

// Main builds hypercube-hold into a temporary directory, runs the tests, and
// removes the directory. A race-enabled test binary gets a race-enabled
// holder. Call it from TestMain.
func Main(m *testing.M)
```

`Main` runs `go build -o <tmp>/hypercube-hold [-race]
github.com/kkestell/hypercube/cmd/hypercube-hold`, printing the build output
and exiting 1 on failure. `-race` comes from a constant in two files:

```go
// race_on.go
//go:build race

package holdtest

const race = true
```

```go
// race_off.go
//go:build !race

package holdtest

const race = false
```

`Main` keeps setting `GORACE=atexit_sleep_ms=0` before `m.Run()`, with its
existing comment, so each race-enabled holder exits at once.

`internal/hold/testmain_test.go` keeps the `parent` mode, which now receives
the holder path as an argument (`<test binary> parent <holder> <dir>`), drops
its own `hold` dispatch and `GORACE` line, and ends with `holdtest.Main(m)`.

The session, MCP, and UI test binaries also serve as harness stand-ins.
Their `TestMain` calls `harnesstest.Run` before `holdtest.Main`, replacing
`TestStandIn`, so stand-in processes do not build another holder. Normal test
processes each build the holder once.

`cmd/hypercube/testmain_test.go` is deleted: `TestAppProcess`'s backend
starts holders from its own sibling executable, and nothing else in the
package starts one.

## Test plan

- `internal/hold`: every test runs against `holdtest.Path` — line output and
  reattachment, segments and discard, exit status and stderr, output write
  failure, closed and lost holders, partial input lines, the long socket
  path, stop, terminal resize, hangup, and retention.
  `TestSurvivesKilledParent` passes the path to the `parent` process.
- New `TestStartWithoutHolder`: `Start` with a holder path that does not
  exist returns an error and leaves no `in` socket.
- `internal/terminal` tests pass with `Set.Holder` set; adopting an existing
  shell after a restart still works.
- `internal/session` (including the orphan in `restart_test.go`),
  `internal/mcp`, and `internal/ui` tests pass with `Holder: holdtest.Path`.
- `TestAppProcess` builds the three executables with
  `go build -o <plugins>/ . ../hypercube-mcp ../hypercube-hold` and passes
  unchanged: agents and the panel shell keep their PIDs and transcript over
  SIGTERM and SIGKILL restarts and end on SIGINT. Before the first restart,
  add one check that the holders are `hypercube-hold`: for each `pid` file
  under `<dir>/state/hypercube/run/*/`, `ps -p <pid> -o command=` starts with
  `<plugins>/hypercube-hold`.
- `go test -race ./internal/hold/...` passes and its holders exit without the
  one-second race-detector delay (the suite takes about as long as before).
- `go list -deps ./cmd/hypercube-hold | grep kkestell/hypercube` lists only
  `cmd/hypercube-hold` and `internal/hold`.
- `grep -rn 'os.Executable' --include='*.go' internal plugins` finds only
  tests.

## Implementation plan

1. Add `cmd/hypercube-hold/main.go`. Change `hold.Start` to take `holder`
   and drop the `hold` argument; update `Main`'s flag set name and usage.
2. Rewrite `internal/hold/holdtest` as above with `race_on.go` and
   `race_off.go`. Update `internal/hold/testmain_test.go` and `hold_test.go`
   (`start`, `TestLongSocketPathRefused`, `TestSurvivesKilledParent`) and add
   `TestStartWithoutHolder`.
3. Add `Programs.Holder` and set it in `programs()`. Pass it to
   `terminal.Set` in `session.New` and to `harnessclient.Launch` in `run`.
4. Add `terminal.Set.Holder` and use it in `Open`. Update
   `terminal_test.go`.
5. Update the runtime fixtures to pass `Holder: holdtest.Path`:
   `internal/session/fixture_test.go` `newRuntime`, `restart_test.go:230`,
   `internal/mcp/mcp_test.go` `family`, `internal/ui/ui_test.go` `handler`
   and line 277, and `internal/ui/terminal_test.go:39`.
6. Delete `cmd/hypercube/testmain_test.go`, update `TestAppProcess`, and
   update `cmd/hypercube/main.go`'s package comment.
7. Update the documentation and run `make check`.

## Documentation updates

- `agents/development.md`, Code: list `cmd/hypercube-hold/` — the holder that
  runs agents' plugins and panel shells.
- The architecture, glossary, and restart guidance that name `hypercube hold`
  are updated in [runtime integration](2026-11-07-011-runtime-integration.md).
