# Change a session's model and effort

## Plan

`agents/plans/2026-11-03-009-session-model-effort.md`

## Summary

Ready sessions select configured profiles within their harness, including while
working. Native plugin controls report the accepted model and effort through
the existing command reply transaction. Saved settings survive reattachment
and conversation resume; errors and partial changes remain visible.

## Decisions

- Exposed readiness as a derived browser snapshot field, using the existing
  running and launch-ready state. Existing-session profiles load when ready.
- Moved OpenCode's existing option selection into `configure.go` so launch and
  live configuration share one path. Pi's shared state response lives beside
  its configuration controls. Added a shared plugin-serving test connection
  for the four native stand-ins.

## Checks run

- Focused session, plugin serve, and UI-action tests passed under `-race`.
- Composer and profile-selector browser behavior tests passed.
- `make check` outside the sandbox passed, including after rebasing onto
  current main: generated assets, browser tests, vet, formatting, and all Go
  tests under `-race`.
- Two earlier runs failed `TestWorktreeSessionChild` on a Git index lock.
  Five isolated race-enabled repetitions on this branch passed. Full uncached
  session race suites then passed on both main (37.311s) and this branch
  (39.901s), using `go test -race -count=1 ./internal/session/`. The cause of
  the earlier failures was not established; no unrelated fix was made.
- Final `make check` passed on main `4c235dd`, including all 332 browser tests.
  `git merge-base --is-ancestor main HEAD` confirmed the final base.

## Manual verification

Started the checkout's isolated development server with temporary state:

```sh
HYPERCUBE_DEV_DIR=$(mktemp -d /tmp/hypercube-model-dev.XXXXXX) \
  HYPERCUBE_DEV_PORT=44291 make dev
curl --fail --silent http://localhost:44291/profiles
```

The server started at `http://localhost:44291/` and returned all four configured
profiles. Stopped it with Ctrl-C and removed its temporary state. No browser
rendering or visual check was performed; those belong to the user. Previously
running sessions need restarting to load the plugin changes. The installed app
and user workspaces were left alone.
