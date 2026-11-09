# Combined runtime refactor review

## Scope and coverage

Reviewed the combined changes from `5773449^` through `78cc9a8`, rather than
reviewing each intermediate commit in isolation. This covers application
configuration extraction, the harness protocol/client split, standalone MCP
and holder commands, backend HTTP composition, runtime packaging, and Codex
removal. Compared the implementation with plans 006–012 dated 2026-11-07 and
their work logs.

Inspected the changed production code and its callers, test changes, build
recipes, installer cleanup, and associated documentation changes. Traced new
agent and panel-shell launches, executable discovery through backend symlinks,
saved start-command compatibility, reattachment to existing holders, startup
and shutdown ordering, browser access checks, MCP forwarding, notification
preferences, and child-report waiting behavior. Checked the combined package
dependencies and the remaining users of the removed shared helpers.

Gaps:

- Did not run live native harnesses, install or restart the user's app, or
  repeat the development watcher smoke test. The process integration test
  uses a stand-in plugin and real backend, bridge, holder, and shell processes.
- Did not rebuild or sign a macOS application bundle or rerun Linux container
  bundle/install/release tests. Packaging and installer changes were inspected;
  this review independently built the standalone executable layout. Earlier
  work logs record packaging checks, but those were not rerun here.
- Effective macOS protected-folder access under the new holder remains
  unverified. Matching signing identifiers alone does not establish it.
- Did not regenerate screenshots or visually review the binary diagram.

## Summary

No findings.

The extracted packages have clear responsibilities without adding a second
lifecycle or transport implementation. The backend supplies executable paths;
the protocol package has no internal package dependencies; production plugins
depend only on the protocol and their shared plugin package. The compatibility
commands and legacy saved-start handling remain connected to the same runtime
implementations. No additional abstraction or recovery machinery is needed.

## Checks run

- `make check` — passed: browser and website builds, Go builds, TypeScript,
  367 browser tests, 28 desktop tests, vet, formatting, and all Go race tests.
  Browser tests emitted React `act(...)` warnings but had no failures. Some Go
  package results were cached; the backend process integration test ran.
- `make binaries OUT=<disposable directory>` — passed. A Python check verified
  exactly three commands and three production plugins, executable permissions,
  removal of a seeded retired Codex executable, and successful backend version
  output. The temporary output was removed.
- `go list` direct imports and module dependency checks — passed: the protocol
  imports no internal packages, the browser package imports neither MCP nor
  desktop, helpers have only their respective internal implementation package,
  and production plugins exclude backend runtime packages. An initial template
  incorrectly assumed standard-library packages have module metadata; the
  corrected query succeeded.
- `bash -n scripts/dev scripts/linux/test-bundle scripts/linux/test-install
  scripts/linux/test-release` and `sh -n scripts/linux-install` — passed.
- `node --check scripts/screenshots/main.js` — passed.
- `git diff --check 5773449^ HEAD` — passed.

## Verdict

Approve the combined refactor. No confirmed correctness, resource-lifetime,
compatibility, or readability findings; no implementation fixes were needed.
The installation and live-harness coverage gaps above remain explicit.
