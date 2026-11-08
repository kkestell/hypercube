# Remove the Codex harness integration

## Goal

Remove Hypercube's bundled Codex plugin and all current product support for it,
without deleting user data, native Codex history, user settings, or historical
project records. After the change, Claude Code, OpenCode, and Pi are the
bundled harnesses; generic plugin discovery still finds whatever executable
plugins are installed. Existing Codex sessions and transcripts remain readable
but cannot be launched, resumed, converted, or recovered by Hypercube.

## Related code

- `plugins/codex/` — the Codex plugin, its native smoke tests, and recorded
  app-server fixtures. Delete the directory.
- `plugins/internal/plugin/rpc.go`, `rpc_test.go` — JSON-RPC support used only
  by the Codex plugin. Delete.
- `plugins/internal/plugin/form.go`, `form_test.go` — elicitation form schema
  support used only by Codex, except `Notes`, which Claude and OpenCode use.
- `plugins/internal/plugin/process.go`, `process_test.go` — `ReadLines` is used
  only by the removed RPC support and `Process.Read`; the RPC subtest is the
  remaining caller of the RPC helper.
- `plugins/internal/plugin/requests.go` — `Requests.Find` has no callers.
- `internal/harness/protocol.go`, `internal/session/reports.go`,
  `internal/harness/harnesstest/`, `internal/session/reports_test.go` — the
  only production emitters of `Item.Async` are in the Codex plugin.
- `Makefile`, `make/macos.mk`, `make/linux.mk` — `PLUGINS` builds, signs, and
  links the bundled harness executables.
- `scripts/linux-install`, `scripts/linux/test-bundle`,
  `scripts/linux/test-install`, `scripts/linux/test-release` — bundle layout,
  plugin links, and installed-plugin expectations.
- `scripts/dev`, `plugins/scripted/main.go` — development and screenshot
  harness lists and sample model guidance.
- `internal/ui/web/agent/harness-icon.tsx`,
  `internal/ui/web/agent/selection-label.ts` — the Codex mark and proper name.
- `internal/ui/web/transcript/transcript-tree.ts` — generic display of native
  tool names and shell commands, including those in saved Codex transcripts.
- `internal/harnessclient/discovery.go` — generic executable discovery. It
  needs no Codex entry or exclusion.

## Decisions

- Keep generic plugin discovery. Do not add a static list of supported
  harnesses or a Codex denylist to the core.
- Preserve stored Codex sessions, transcripts, and native Codex history. They
  display with raw harness ID `codex` and cannot launch because no plugin
  exists. Do not add conversion, recovery, or migration machinery.
- Remove the Codex icon and proper-name mapping because they imply current
  support. In `transcript-tree.ts`, delete `nativeSummary` and its Codex-only
  shell-unwrapping test, and call `toolSummary(input)` directly. Keep the cheap
  general `nativeTools` labels so preserved records remain readable.
- Remove only shared helpers with no remaining production caller. Move
  `Notes` into `requests.go` and keep its behavior for Claude and OpenCode.
  Delete `Requests.Find`, and inline `ReadLines` into `Process.Read` as the
  only remaining use.
- Remove the `Item.Async` question distinction: no current harness emits it
  and no browser or database code reads it. Keep blocking-question behavior,
  `InputPending`, and `Final`. Unknown `async` fields in old item JSON remain
  harmless because JSON ignores unknown fields, so no migration is needed.
- Clean retired artifacts explicitly and locally. Do not build a plugin
  lifecycle system:
  - `make binaries` removes `$(OUT)/hypercube-harness-codex`, which fixes the
    incremental `bin/dev` case.
  - macOS and Linux `build` remove the stale `bin/hypercube-harness-codex`
    link before linking current plugins.
  - macOS `place` removes the retired link from `BINDIR`; Linux install prunes
    dangling `hypercube-harness-*` links that point into the replaced bundle,
    following the ownership rule already used by uninstall.
- Update screenshot scene settings so future captures use a supported
  harness. Replace each Codex scene session with OpenCode using
  `openai/gpt-6.1-terra` at `high` effort. Do not regenerate the existing
  raster screenshots for this support-list change.
- Preserve `agents/plans/`, `agents/work/`, `agents/reviews/`,
  `agents/research/harnesses/codex.md`, `agents/scratch/`, and the completed
  Linux checklist in `agents/todo.md`. Generic tests elsewhere may continue to
  use `codex` as an arbitrary sample ID.
- Keep the Codex sandbox advice in `agents/development.md`; it concerns running
  repository commands inside a coding agent, not Hypercube's harness support.

## Test plan

- `make check` covers the build, Go tests, UI tests and type check, website,
  formatting, vet, and the race detector.
- `scripts/linux/run bash -c 'scripts/linux/test-bundle && scripts/linux/test-install && scripts/linux/test-release'`
  validates the bundle layout, plugin links and descriptions, installer
  upgrade cleanup, service restart, and release packaging.
- Add a stale-link check to `scripts/linux/test-install`: before reinstalling,
  create a dangling `~/.local/bin/hypercube-harness-codex` symlink into the
  installed bundle and assert the reinstall removes it.
- Confirm build cleanup in a disposable output directory: place a fake
  executable at `<tmp>/hypercube-harness-codex`, run `make binaries OUT=<tmp>`,
  and check it is gone. Inspect the macOS link cleanup in the Makefile; do not
  build a whole macOS bundle solely to check one stale symlink. The Linux
  packaging tests cover the installed-link cleanup.
- Search tracked files (`git grep -n -i codex`) and confirm remaining matches
  are historical records, preserved user-data notes, generic test samples, or
  unrelated coding-agent advice. Confirm no `plugins/codex` files and no Codex
  entry in current harness lists.

## Implementation plan

1. Delete `plugins/codex/` including `testdata/`.
2. In `plugins/internal/plugin`, delete `rpc.go`, `rpc_test.go`, `form.go`, and
   `form_test.go`. Move `Notes` into `requests.go` and its test into a new
   `requests_test.go`. Delete `Requests.Find`. Inline `ReadLines` into
   `Process.Read` and delete the helper. Remove
   `TestRPCCallFailsWithTheExitReason` and the now unused `context` import from
   `process_test.go`.
3. Remove the async-question distinction: delete `Item.Async` and its comments
   from `internal/harness/protocol.go`, the async branch in
   `internal/session/reports.go`, the `!asyncask` stand-in in
   `internal/harness/harnesstest/`, and the async-only report test cases. Keep
   the generic blocking-question tests.
4. Change `PLUGINS` in `Makefile` to `claude opencode pi`. In `binaries`,
   remove `$(OUT)/hypercube-harness-codex` before building current plugins.
5. In `make/macos.mk`, remove `bin/hypercube-harness-codex` in `build` and the
   retired `BINDIR` link in `place`. In `make/linux.mk`, remove
   `bin/hypercube-harness-codex` in `build`.
6. In `scripts/linux-install`, prune `hypercube-harness-*` symlinks in `BINDIR`
   whose targets are under the installed app but no longer resolve, then link
   the current plugins.
7. Remove `codex` from the plugin arrays and product map in
   `scripts/linux/test-bundle`, `scripts/linux/test-install`, and
   `scripts/linux/test-release`. Add the stale-link upgrade check to
   `test-install`.
8. Remove `codex` from `scripts/dev`'s sample model guidance and from
   `plugins/scripted/main.go`'s display-name map.
9. Remove the `codex` icon entry from `harness-icon.tsx` and the `codex`
   proper name from `selection-label.ts`. Delete `nativeSummary` from
   `transcript-tree.ts`, call `toolSummary(input)` directly, and remove its
   Codex-only shell-unwrapping test while keeping the general native-label
   test.
10. Update UI tests coupled to the removed proper name:
    `model-select.test.tsx`, `settings-dialog.test.tsx`, and
    `transcript.test.tsx`. Use `opencode` as the sample harness and expect
    `OpenCode` in titles. Leave tests that use `codex` only as an arbitrary
    stored ID unchanged.
11. In `docs/screenshots/attention.json`, `details.json`, `file-preview.json`,
    `first-session.json`, `overview.json`, `suggestion.json`, and
    `worktrees.json`, replace Codex session and proposal settings with
    `{"harness": "opencode", "model": "openai/gpt-6.1-terra", "effort": "high"}`
    including `create_session` tool inputs. Do not regenerate images.
12. Update the architecture diagram text in `agents/images/architecture.svg`
    from the Codex line to `or Pi`, then regenerate
    `agents/images/architecture.png` from the SVG with `rsvg-convert` at the
    existing 1440×2432 size. Do not edit the PNG by hand.

## Documentation updates

- `README.md`, `AGENTS.md`, `docs/installation.md`, `docs/introduction.md`,
  `web/content/_index.md`, `web/content/features.md` — list only Claude Code,
  OpenCode, and Pi.
- `docs/harness-support.md` — remove the Codex requirements row, message and
  interrupt notes, permissions section, questions row and async-question
  paragraph, usage row, and effort statement mention. Adjust remaining
  multi-harness sentences.
- `docs/storage.md` — keep a short note that Codex sessions, transcripts, and
  native history are preserved. State that Hypercube no longer includes Codex
  support and does not resume or convert those conversations; start new work
  with a supported harness. Do not delete or move stored files.
- `agents/architecture.md` — remove Codex from the plugin list and research
  links; keep the generic plugin protocol description.
- `agents/development.md` — remove Codex from the `plugins/` layout sentence;
  keep the unrelated Codex sandbox advice.
- `agents/glossary.md` — remove Codex from the harness examples.
- `agents/todo.md` — leave the completed historical checklist unchanged.
