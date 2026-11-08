# Remove the Codex harness integration

## Plan

[Remove the Codex harness integration](../plans/2026-11-07-012-remove-codex-integration.md)

## Summary

The plan's goal is met. The Codex plugin, its fixtures, and the shared
JSON-RPC and form-schema helpers used only by it are deleted. `Notes` moved to
`requests.go`, `Requests.Find` is gone, `ReadLines` is inlined into
`Process.Read`, and the `Item.Async` distinction and its stand-ins and tests
are removed. Claude Code, OpenCode, and Pi remain bundled.

Build, install, discovery, UI labels and icons, transcript summaries, scenes,
docs, and the architecture diagram now describe only the supported harnesses.
Existing Codex sessions, transcripts, native history, historical `agents/`
records, settings, credentials, `HOME`, and the native `codex` executable are
untouched.

## Departures from the plan

- `TestAnsweredQuestionStartsTheIdleTimeout` used `!asyncask` to open a
  question while the agent worked. With the async stand-in removed, it uses
  `!ask`; its idle-timeout assertion still passes.
- The stale-link upgrade check runs before the zsh reinstall, the first
  reinstall in `test-install`.

## Decisions

- macOS `place` removes only the `BINDIR` Codex link that points at the
  installed app. Linux install prunes only dangling `hypercube-harness-*` links
  whose target is under the replaced bundle. Both follow uninstall's ownership
  rule, so links to custom plugins are left alone.
- Packaging tests assert absence directly: the bundle has no Codex executable
  or discovery entry, and a reinstall removes a dangling Codex link.

## Checks run

- `GOCACHE=/tmp/hypercube-00ba4eac-go-cache make check` — passed. Log:
  `/tmp/hypercube-00ba4eac-remove-check.log`.
- `GOCACHE=/tmp/hypercube-00ba4eac-go-cache go test -race -count=1 ./internal/session ./internal/harness/... ./plugins/internal/plugin ./plugins/scripted`
  — passed without the test cache.
- `scripts/linux/run bash -c 'scripts/linux/test-bundle && scripts/linux/test-install && scripts/linux/test-release'`
  — all three passed. Log: `/tmp/hypercube-00ba4eac-remove-linux.log`.
- `cd internal/ui && bunx tsc -p . && bun test web/agent/model-select.test.tsx web/app/settings-dialog.test.tsx web/transcript/transcript.test.tsx web/transcript/transcript-tree.test.ts`
  — 66 tests passed.
- Disposable `make binaries OUT=<tmp>` check — a fake
  `hypercube-harness-codex` was removed and the three current plugins built.
- `jq empty docs/screenshots/*.json`, `bash -n`/`sh -n` on changed scripts,
  and `git diff --check` — passed.
- `git grep -n -i codex` — remaining matches are historical `agents/`
  records, preserved Codex-data documentation, intentional cleanup and test
  assertions, generic sample IDs, and unrelated Codex sandbox advice.

## Manual verification

1. Stale executable cleanup in a disposable output directory.

   ```sh
   tmp=$(mktemp -d /tmp/hypercube-00ba4eac-remove-out.XXXXXX)
   printf '#!/bin/sh\nexit 0\n' > "$tmp/hypercube-harness-codex"
   chmod +x "$tmp/hypercube-harness-codex"
   GOCACHE=/tmp/hypercube-00ba4eac-go-cache make binaries OUT="$tmp"
   test ! -e "$tmp/hypercube-harness-codex"
   ```

   Observed: `make binaries` printed the removal, then built `hypercube` and
   the Claude Code, OpenCode, and Pi plugins.

2. Architecture diagram.

   ```sh
   rsvg-convert -w 1440 -h 2432 -o agents/images/architecture.png agents/images/architecture.svg
   identify agents/images/architecture.png
   ```

   Observed: `PNG 1440x2432`, matching the previous raster size.

3. Bundle, discovery, and installed-link cleanup ran in the disposable Linux
   container. `test-bundle` asserted no Codex executable in the bundle or
   `bin/` and no Codex entry in `/harnesses`; `test-install` created a dangling
   `~/.local/bin/hypercube-harness-codex` before reinstalling and asserted its
   removal; `test-release` asserted the packaged install contains no Codex
   executable or link.

4. macOS `place` cleanup condition, exercised with temporary paths rather than
   a bundle. An owned stale Codex link was removed and a link to a custom
   plugin elsewhere was kept.

## Follow-up work

- Parent review and commit.
- No macOS bundle was built for the one stale symlink, as the plan directs;
  its cleanup is in the Makefile and the Linux installer test covers the
  ownership rule.
- Existing screenshots were not regenerated; only future scene captures use
  the supported harness settings.
