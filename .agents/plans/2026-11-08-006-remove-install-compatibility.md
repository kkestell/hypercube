# Retire old build and installation cleanup

- Task ID: `2026-11-08-006-remove-install-compatibility`
- Status: ready; awaiting independent plan review
- Revision: 2; resolves Round 1's current owned-link reconciliation scope
- Source HEAD: `ae022eeebf9dc90c75a61527267db0682b194057`
- Author: `compat-plan`, Hypercube `d36e9339`, native
  `ses_e41a1172bffeqvkaFBnCzmGxvc`; OpenCode / `openai/gpt-6.1-sol` / `xhigh`
- Artifact path (repository-relative): `.agents/plans/2026-11-08-006-remove-install-compatibility.md`
- Shared guidance (repository-relative): `.agents/designs/2026-11-08-003-remove-backwards-compatibility.md`,
  linked [here](../designs/2026-11-08-003-remove-backwards-compatibility.md).
- Prerequisite: execute after 005 so disposable Linux tests verify the integrated
  current CLI/plugin baseline. Other removal edits are otherwise independent.
- Request: remove installation/build/dev paths retained for earlier versions,
  preserving current replacement, service and holder behavior.

## Outcome and boundaries

Build/install rules create and link the current executable set. They contain no
cleanup targeting retired Codex names or identification of older combined
desktop/backend executables. Development cleanup recognizes current dedicated
holders only.
Current desktop quit, complete-bundle replacement, service SIGTERM replacement,
plugin/helper discovery, signing, AppArmor and supported shell environments remain.

Do not run host installation, service restart, app quit, dev cleanup, or any
command that modifies installed state. Linux installer execution is allowed only
inside the existing disposable test container. No push/merge or historical doc
rewrite. The task does not authorize deleting stale files from either real
installation merely because cleanup code is being retired.

## Current behavior and decisions

`Makefile:binaries`, `make/linux.mk:build`, and `make/macos.mk:build/place` delete
retired Codex binaries/links. Remove those explicit lines/conditional, retaining
the current `PLUGINS := claude opencode pi` build/link loops. `binaries` can reuse
an output directory, but this retirement adds no replacement stale-output scan.
Bundle packaging already recreates its own output as normal build behavior.

`scripts/linux-install` scans links into the replaced bundle and deletes ones
that no longer resolve. Retain this generic owned-link pruning unchanged: a
current custom bundle can contain additional plugins whose targets disappear
when the stock bundle replaces it. This has no historical-version condition.
Keep the ownership restriction to links into this app, complete staging/rename
replacement, refusal to overwrite nonbundles, current CLI/plugin link creation,
desktop process shutdown, launcher/icon handling, and AppArmor.
`scripts/linux-uninstall`'s owned-link removal likewise remains normal behavior.

`make/macos.mk:quit-desktop` checks executablepath to distinguish the earlier
combined backend from today's desktop app. Replace that distinction with a
current-only running check: AppleScript tests `application id APP_ID is running`
before telling it to quit. Preserve the guard so install-service does not launch
a closed app merely to quit it. Do not change launchd bootout/port waiting,
service SIGTERM semantics, app signing, or holders. `scripts/install-service`'s
"old backend" means the previous running instance, not an old-version adapter.

`scripts/dev:stop_holders` matches both dedicated `hypercube-hold` and old
`hypercube hold` command lines. Remove only the second alternative. Keep its
scope to holder PID files beneath the chosen dev state, its exit wait, and all
normal cleanup behavior. Do not execute clean against the user's dev folder.

## Execution sequence

1. Remove explicit retired-Codex artifact cleanup from Makefile and both platform
   make fragments. Keep the currently supported binary/plugin/link names and
   dedicated MCP/holder placement unchanged.
2. Retain `scripts/linux-install`'s generic owned broken-link loop unchanged.
   Remove the dev old-holder match and simplify only the macOS old-executable
   distinction as above; keep complete normal platform behavior elsewhere.
3. Reword `scripts/linux/test-install`'s Codex-named reconciliation case as a
   neutral additional plugin, such as `hypercube-harness-extra`. Before the zsh
   reinstall, copy a current plugin executable into that path in the installed
   test bundle and create its owned link in the test user's bin directory;
   confirm it resolves. Reinstalling the stock bundle removes the extra target;
   assert the now-broken owned link is pruned. Keep the zsh reinstall and all
   shell-environment, holder-survival, restart and uninstall assertions. Do not
   delete present-day reconciliation coverage with the historical name, broaden
   cleanup to unowned links, or add a runtime allowlist.
4. In `scripts/linux/test-bundle` / `test-release`, remove old-cleanup-specific
   Codex checks. Keep positive current bundle/executable/layout/discovery checks.
   Prefer asserting the current discovered set in test-bundle over a negative
   hard-coded retired name if its existing checks need an exact-set assertion;
   do not add a runtime plugin allowlist (custom plugins remain supported).
5. Complete a final source/build/install compatibility audit against the shared
   scope map and all four units' diffs. A genuine omitted historical adapter
   must be removed in its owning unit with verification/review, not hidden by
   relabeling it; report any consequential scope growth to the parent.

## Verification and safety

Run `make check` before committing code. Also syntax-check changed shell scripts
with their declared shells (`bash -n scripts/dev scripts/linux/test-bundle
scripts/linux/test-install scripts/linux/test-release`). The unchanged installer
loop is exercised by the Linux test below. Inspect the macOS make rule with
`make -n quit-desktop`; do not execute the recipe on the installed app. The chosen
AppleScript must preserve the running guard, application ID and quit action; a compiler-only
check may use a temporary output without executing the script. No host
packaging/install or actual desktop quit is required for this narrow change.

Required Linux behavior command, from repository root:

```sh
scripts/linux/run bash -c 'scripts/linux/test-bundle && scripts/linux/test-install'
```

The runner copies tracked/untracked nonignored files into a fresh systemd/Xvfb
container and executes as disposable `tester`. test-bundle invokes make build;
test-install invokes make install-service/uninstall inside that container.
Its modifications to HOME, shell configuration, app links, service and isolated
scripted sessions therefore do not affect the user's installation. This selected
pair verifies changed Linux build/link/install paths, removal of the neutral
owned extra-plugin link, and normal replacement/holder behavior; desktop
screenshot and tagged release packaging tests add no necessary evidence for
deleting these historical paths. Still syntax-check the edited release test.
Do not run test-install directly on macOS or outside runner.

Planning confirmed `docker info --format '{{.OSType}}/{{.Architecture}}'` returns
`linux/aarch64`; Go/Bun/Docker executables are present. If runtime availability
changes, record the failure and leave required Linux validation unresolved,
rather than claim a macOS syntax pass proves Linux behavior. Reuse the recorded
successful final code checks during independent review unless a concrete
uncertainty or intervening code change warrants another run.

The final audit searches executable source plus build/install/dev/test scripts
for migration/upgrade symbols, old scalar/MCP aliases, main compatibility
commands, old browser keys and Codex cleanup. Classify remaining matches with
the shared design; generic third-party compatibility, permissions, current
defaults and historical records stay. Inspect the whole task-owned diff, not
just search output. Record commands/results, final scope dispositions and code
anchors in this unit's log, and report any required check not completed.

## Handoff and normalization

Ready; no unresolved decisions or planning-time tooling blocker. This is a
bounded platform-retirement assignment: mostly deletions, but macOS running-app
behavior and substantive Linux build/install lifecycle verification make it more
than a tiny copy-edit brief. Reusing the existing container/scripts avoids new
installer machinery or multi-distribution expansion. Detail settles the safe
test boundary and current behaviors without prescribing shell function bodies.
Use the experimental 600–1,500-word band; no curated manifest was provided.
Planning writes records only; parent owns plan/review commits.
