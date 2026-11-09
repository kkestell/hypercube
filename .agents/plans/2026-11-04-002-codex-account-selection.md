# Select the Codex account for new sessions

## Goal

New Hypercube Codex conversations follow Neuron's current account selection,
without changing running agents or requiring Neuron for ordinary Codex users.
Keep native conversation IDs and private resume history usable.

## Related code

- `plugins/codex/storage.go` resolves the source home, links its inputs into
  Hypercube's private home, and imports an older requested native rollout.
- `plugins/codex/main.go` supplies the private `CODEX_HOME` and `sqlite_home`
  when launching app-server. The UI service launches the plugin afresh.
- `plugins/codex/storage_test.go` and `main_test.go` exercise storage and the
  launch protocol using temporary state and harness stand-ins.

## Decisions

The existing single private home is the baseline. It cannot select a second
account without conflicting with its existing credential/configuration links.
Separate private homes for different source paths satisfy that requirement
without copying credentials or merging account databases.

- A nonempty inherited `CODEX_HOME` takes precedence. Otherwise read the
  optional `~/.config/neuron/codex-profile.zsh` on each new launch. Accept its
  agreed literal forms, `unset CODEX_HOME\n` and
  `export CODEX_HOME="$HOME/.codex-strib"\n`. An absent file keeps the
  `~/.codex` default; other read/format errors fail clearly. Do not execute
  shell configuration or require Neuron's launcher.
- Keep `StateDir/codex` for the default source when its existing input links
  are compatible. If its links belong to a different source, leave it intact
  and use a source-specific home instead. All nondefault sources use
  `StateDir/codex/homes/<sha256-of-absolute-source-path>`. Keep the current
  explicit shared-input list, symlink semantics, and per-home SQLite override.
- When resuming a conversation already private, locate its rollout first in
  the legacy private home or `StateDir/codex/homes/*` and reuse that home
  unchanged, before reading CODEX_HOME or the optional selection file. This
  preserves its existing account and storage even with an invalid selection
  file. Do not copy
  rollouts between account homes. The existing single-rollout import remains
  for a requested native conversation that has not yet become private.
  A native conversation available only in a different source home still
  fails explicitly; do not search or import another account's native history.
- No core protocol, application database, session schema, user configuration,
  launcher, or credentials changes are needed. No source home is modified by
  Hypercube. No running session or service is restarted or installed by this
  work.

OpenAI documents that [CODEX_HOME includes auth and session storage](https://learn.chatgpt.com/docs/config-file/environment-variables)
and that [file credentials are stored beneath that home](https://learn.chatgpt.com/docs/auth?surface=cli).
The repository already isolates writable state and shares source credentials
through native-supported symlinks; retain that boundary.

## Implementation plan

1. Extend `plugins/codex/storage.go` with optional source selection and direct
   helpers for locating private resumes and selecting a compatible private
   home. Validate existing default-home links before writing any new links.
2. Extend `plugins/codex/storage_test.go` to cover no-switcher defaults,
   explicit overrides, both switch values, selection changes between launches,
   independent source homes, unchanged old links, and legacy/current resumes
   after selection changes. Retain existing isolation and error coverage.
3. Adapt `plugins/codex/main_test.go` process assertions to the chosen private
   layout and cover a new launch selecting the optional account while keeping
   the private home, SQLite override, inherited HOME, and native launch intact.
4. Update `docs/harness-support.md` with optional selection, precedence,
   separate private account storage, and retained resume accounts.

## Test plan

- An ordinary user without Neuron starts with `~/.codex` and the legacy
  private directory. No Neuron executable or selection file is required.
- A nonempty explicit source override wins over either selection and retains
  relative-path resolution. Missing selection defaults; malformed or
  unreadable selection fails without running arbitrary shell text.
- Default, Star Tribune, then Default launches use the expected stable private
  homes. Their credential/configuration links keep pointing to their original
  sources, and outputs/SQLite remain private.
- A legacy private rollout and a new per-source rollout resume in their
  original homes even after toggling. An older native rollout imports only
  its requested history; missing native history, including one belonging only
  to another source account, stays an explicit error. Invalid selection must
  not break a private resume.
- Repeated/concurrent same-source setup keeps working. A legacy private home
  linked to an explicit nondefault source is retained while a default launch
  selects another private home.
- Existing native process stand-ins still verify private CODEX_HOME,
  sqlite_home, unchanged HOME, thread identity, and failure behavior.

## Activation

The user specifically requested the smallest activation procedure that leaves
existing agents and the UI service running. Verify whether plugin-only
replacement can retain the installed app's valid signature; otherwise report
the existing no-service-restart installation path. Do not install anything.

Verified on a temporary copy of the installed signed app: replacing and
signing only the Codex plugin makes bundle verification fail with "nested code
is modified or invalid"; signing the bundle afterward restores verification.
Use the existing `make install` target to build, sign, and replace the bundle
without the service restart in `make install-service`. The running service
locates the plugin beside its executable on each future agent launch, while
existing holders and agents keep their current processes. Activation remains
pending user coordination.
