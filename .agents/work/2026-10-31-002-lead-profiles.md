# Lead profiles and shared Hypercube icon

## Plan

`agents/plans/2026-10-31-006-lead-profiles.md`

## Summary

Implemented every task: role-based profiles, the per-session lead menu,
worker-role guidance, updated examples, and the shared browser/menu bar icon.
The branch includes the required `f7dd03b` dependency.

## Checks run

- The initial focused Go test command failed because the sandbox could not
  access the default Go build cache. Repeating it with
  `GOCACHE=/tmp/hypercube-kwork-006-go-cache` passed the config, worker tool,
  startup context, lead endpoint, and icon tests.
- `env BUN_INSTALL_CACHE_DIR=/tmp/hypercube-kwork-006-bun-cache make ui` —
  passed; regenerated the stylesheet for the menu's viewport bounds.
- `env GOCACHE=/tmp/hypercube-kwork-006-go-cache
  GOMODCACHE=/tmp/hypercube-kwork-006-go-mod-cache make check` — passed build,
  vet, formatting, and all race-enabled tests. `TestTmuxNativeLifecycle` passed;
  the reported pre-existing `respawn-pane fork failed: Device not configured`
  failure did not reproduce.
- `node --check` on the extracted inline script — passed. Parsed both config
  examples and checked roles and unique names; confirmed the favicon bytes
  exactly match `git show bun:src/favicon.svg`.
- `git diff --check` — passed.

## Manual verification

Live checks were not run, as the lead reserved them for the user. With a fresh
dev directory, the menu check can be reproduced with:

```sh
export HYPERCUBE_DEV_DIR="$(mktemp -d /tmp/hypercube-leads.XXXXXX)"
make dev
```

Open the workspace's `+` menu; check its placement, Escape and outside-click
dismissal, and session creation with each lead. Clean up with `make dev-clean`.

## Follow-up work

- The lead copies the example to the user's config when installing. The user's
  config was not edited.
- After installation, check the browser tab and menu bar icons in light and
  dark themes. No live workspaces or service settings were changed here.
