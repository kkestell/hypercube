# Harness protocol and plugin client packages

## Goal

`internal/harness` defines the plugin protocol, which every harness plugin,
the store, reports, and the browser handlers import. The same package also
holds the backend's plugin client, which starts plugins under holders, and
the code that runs plugins' discovery commands. Every plugin therefore
depends on `internal/hold` and process execution it never uses.

This task moves the client and discovery execution into a new package,
`internal/harnessclient`. When it is done, `internal/harness` holds only
protocol data and its validation, and imports no other Hypercube package.
Behavior is unchanged.

Prerequisite: [application configuration](2026-11-07-006-application-config.md),
which removed `config.go` from `harness`. Addresses the rest of review finding
F4.

## Related code

- `internal/harness/harness.go` — the package comment (plugins, `serve`,
  per-launch configuration), `Client`, `Launch`, `Attach`, `Client.Send`,
  `Client.Read`, `Stop`, `Discard`, `Remove`, `Close`. Imports `hold`.
  `Read` calls the private `decode`.
- `internal/harness/discovery.go` — two halves in one file:
  - Data: the comment describing `describe` and `models`, `Settings`,
    `Settings.Check`, `Description`, `ModelsRequest`, `Model`, `Catalog`,
    `Catalog.Check`, `Harness`.
  - Execution: `pluginPrefix`, `pluginDir`, `Plugins`, `Find`,
    `describeTimeout`, `Describe`, `Models`, `cancelGrace`, `run`.
- `internal/harness/protocol.go` — commands, records, events, items,
  `Command.Line`, `decode`, `Event.check`.
- `internal/harness/image.go` — `CheckImages`; stays.
- `internal/harness/harness_test.go` — `TestDecodeChecksEvents`, which tests
  `decode`.
- `internal/harness/discovery_test.go` — `TestDescribe`, `TestModels`,
  `TestModelsCancel` (uses `cancelGrace`), and helpers `plugin` and
  `processExists`.
- Callers of the moving code, all in `internal/session`:
  - `launch.go:26` — `live.client *harness.Client`.
  - `launch.go:128-142` — `harness.Launch(dir, path)` and
    `harness.Attach(dir)` in `run`.
  - `launch.go:209` — `harness.Find(r.PluginDir, s.Harness)` in `pluginPath`.
  - `sessions.go:64` — `harness.Find` when creating a session.
  - `selection.go:16,41` — `harness.Describe` and `harness.Models`.
- `internal/harness/harnesstest` — the stand-in plugin used by process tests.
  It imports only `harness` data; it stays where it is.

## Decisions

### What moves

| Code | Destination |
| --- | --- |
| `harness.go`: `Client` and its methods, `Launch`, `Attach` | `internal/harnessclient/client.go` |
| `discovery.go` execution half | `internal/harnessclient/discovery.go` |
| `discovery_test.go` | `internal/harnessclient/discovery_test.go` |
| `harness_test.go` | `internal/harness/protocol_test.go` |

The discovery data half stays in `internal/harness/discovery.go` with the
comment that describes the two commands.

Exported names keep their names in the new package: `harnessclient.Client`,
`harnessclient.Launch`, `harnessclient.Attach`, `harnessclient.Plugins`,
`harnessclient.Find`, `harnessclient.Describe`, `harnessclient.Models`. The
discovery functions return and accept the `harness` data types
(`[]harness.Harness`, `harness.ModelsRequest`, `harness.Catalog`).

### Record decoding

`Client.Read` needs the decoder that lives with the protocol. Export it:

```go
// DecodeRecord parses one line of a plugin's output and checks that it is a
// well-formed reply or event.
func DecodeRecord(line []byte) (Record, error)
```

`Event.check` stays private; `DecodeRecord` is its only caller.
`TestDecodeChecksEvents` calls `DecodeRecord`.

### Package comments

`internal/harness` gets its package comment on `protocol.go`:

```go
// Package harness defines the protocol between Hypercube and its harness
// plugins. A plugin is an executable named hypercube-harness-<name>. Its
// `serve` command keeps a native harness alive across turns and translates
// between the harness and the commands and records in this file; its
// `describe` and `models` commands are described in discovery.go. A plugin
// builds the harness's command line from per-launch arguments, environment,
// and overrides, without changing the user's settings, credentials, or HOME.
package harness
```

`internal/harnessclient/client.go`:

```go
// Package harnessclient runs harness plugins for the backend: it finds them
// in the plugin directory, runs their discovery commands, and starts and
// attaches to their serve command under a holder.
package harnessclient
```

### Unchanged in this task

`Plugins` and `Find` still fall back to the running executable's directory
when `dir` is empty, and `Launch` still calls `hold.Start`, which runs the
current executable as `hold`. The
[MCP bridge plan](2026-11-07-008-mcp-bridge-executable.md) removes the
directory fallback and the [holder plan](2026-11-07-009-holder-executable.md)
changes `Launch`. `Runtime.PluginDir`, `session.New`, and the test
`TestMain`s that dispatch `hold` stay as they are.

## Test plan

- `internal/harness` `TestDecodeChecksEvents` (renamed file) passes against
  `DecodeRecord` with every existing case.
- `internal/harnessclient` `TestDescribe`, `TestModels`, and
  `TestModelsCancel` pass with qualified data types; `TestModelsCancel` still
  sets `cancelGrace` and still finds no surviving native process.
- `internal/session`, `internal/mcp`, `internal/ui`, and `cmd/hypercube`
  tests pass unchanged apart from imports.
- Dependency checks:
  - `go list -f '{{join .Imports "\n"}}' ./internal/harness` lists no
    `github.com/kkestell/hypercube/...` package.
  - `go list -deps ./plugins/claude ./plugins/codex ./plugins/opencode
    ./plugins/pi ./internal/store | grep kkestell/hypercube/internal` lists
    `internal/harness` and the packages those programs import directly, and
    no `hold`, `harnessclient`, `config`, `session`, `terminal`, or `ui`.
  - `go list -deps ./internal/harnessclient` lists `harness` and `hold` and
    no `session` or `config`.

## Implementation plan

1. Create `internal/harnessclient/client.go` with the package comment above
   and `Client`, `Launch`, `Attach`, and the methods from `harness.go`.
   Qualify `harness.Record`; `Read` calls `harness.DecodeRecord`. Delete
   `internal/harness/harness.go`.
2. In `internal/harness/protocol.go`, rename `decode` to `DecodeRecord` with
   the doc comment above and add the package comment. Rename
   `harness_test.go` to `protocol_test.go` and update the call.
3. Move the execution half of `discovery.go` to
   `internal/harnessclient/discovery.go`, qualifying `harness.Harness`,
   `harness.Description`, `harness.ModelsRequest`, and `harness.Catalog`.
   Remove the imports `internal/harness/discovery.go` no longer uses
   (`bytes`, `context`, `encoding/json`, `os`, `os/exec`, `path/filepath`,
   `syscall`, `time`).
4. Move `discovery_test.go` to `internal/harnessclient` as
   `package harnessclient` and qualify the data types it builds.
5. Update `internal/session/launch.go`, `sessions.go`, and `selection.go` to
   call `harnessclient`, keeping `harness` imports where they still use
   protocol types.
6. Run `go build ./...`, the dependency checks, and
   `grep -rn 'harness\.\(Client\|Launch\|Attach\|Plugins\|Find\|Describe\|Models\)\b' --include='*.go' .`,
   which should find nothing.
7. Update the documentation and run `make check`.

## Documentation updates

- `agents/development.md`, Code: add `harnessclient` to the `internal/`
  package list.
- `agents/architecture.md` line 29 links discovery's commands to
  `internal/harness/discovery.go`, which still defines them; no change.
