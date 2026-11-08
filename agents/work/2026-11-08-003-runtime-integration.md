# Ship the runtime helpers

## Plan

`agents/plans/2026-11-07-011-runtime-integration.md`

## Summary

The build and screenshot recipes now build the separate MCP bridge and holder.
Both bundles and releases inherit those files through `binaries`; the helpers
remain private to the bundle. macOS signs the holder with the backend's app
identifier. Development cleanup recognizes both holder command forms, and the
Linux delivery checks and current developer references describe the new layout.
No installer machinery or compatibility-command changes were needed.

## Departures from the plan

- The dev watcher hashes contents, so touching a Go file cannot trigger a
  rebuild. Updated the plan and changed a harmless comment in a disposable
  source copy instead. An existing dev server had unknown ownership; the smoke
  used its own source copy, output, temporary state, and port 4299.
- The shell smoke used the panel's actual WebSocket endpoint rather than opening
  the browser page. It exercised the real login shell, holder, scrollback,
  rebuild, reconnection, and stop path without changing the existing dev state.

## Checks run

- `make check` outside the sandbox — passed: UI and website builds, TypeScript,
  368 browser tests, 28 desktop tests, Go builds, vet, formatting, and all Go
  race tests. The initial sandbox run failed only because `TestAppProcess`
  cannot execute `/bin/ps` there; the full rerun passed unchanged.
- `make binaries OUT=/tmp/hc-011-bin` — passed; all three commands and four
  production plugins were executable. Temporary output removed after checking.
- `make build` — passed with the existing icon outputs and local signing
  certificate; no icon conversion or install was needed.
- `codesign -dv` on the backend and holder — both report
  `Identifier=com.github.kkestell.hypercube`.
- `codesign --verify --deep --strict bin/Hypercube.app` — passed. Inspected the
  bundle's executable files and `bin/` links; both helpers are in the bundle,
  and only the backend and plugins have CLI links.
- `scripts/linux/run scripts/linux/test-bundle`,
  `scripts/linux/run scripts/linux/test-install`, and
  `scripts/linux/run scripts/linux/test-release` — all passed on Debian ARM64
  in disposable containers, including symlinked backend discovery, installed
  holder command, restart/reinstall survival, and extracted archive helpers.
- `go list -deps` — the MCP command's sole module dependency is
  `internal/mcpbridge`, the holder's is `internal/hold`, and all four production
  plugins exclude `hold`, `harnessclient`, `config`, `session`, `store`, and `ui`.
- Inspected the screenshot recipe's three-command loop; captures were not
  regenerated, as planned.
- `git diff --check` — passed.

## Manual verification

- Started `scripts/dev up` in a disposable source copy with temporary
  `HYPERCUBE_DEV_DIR` and `HYPERCUBE_DEV_PORT=4299`. Connected to
  `/terminal?workspace=<scratch-workspace>` with matching Origin, wrote a
  marker and shell PID, and checked the holder command resolves to that copy's
  `bin/dev/hypercube-hold`. After changing a Go comment and waiting for a new
  backend PID, reconnecting replayed the marker and shell PID; the holder PID
  was unchanged. `scripts/dev stop` ended the backend and holder.
- Launched actual old `hypercube hold` and new `hypercube-hold` processes with
  holder directories in temporary dev state. With temporary HOME and
  `HYPERCUBE_DEV_DIR`, `scripts/dev clean` stopped both and removed that state.
- Removed the disposable sources and state, and checked no test backend or
  holder remained. No stop, restart, or workspace actions targeted the existing
  dev server or installed app.

## Follow-up work

macOS protected-folder access under the newly signed holder remains unverified.
It requires the user to install the build, restart an agent, and check access to
such a folder (for example `~/Documents`). No install was performed. Existing
agents and panel shells pick up the new launch commands after they restart.
