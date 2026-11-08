# Distinguish Git failures from plain folders

- Task ID: `2026-11-08-002-git-error-classification`
- Status: ready; awaiting independent plan review
- Source HEAD: `d951ee5524d89d322d3cff161727b01982a7a57d`
- Checkout: `/Users/kyle/src/hypercube/.hypercube/03a23830`
- Branch: `hypercube/session-03a23830`
- Request: [GitHub issue #59](https://github.com/kkestell/hypercube/issues/59).
- Author: planner, Hypercube worker address `git-errors-plan`; underlying child
  session ID unavailable because the exposed session metadata returns only its
  worker address/name. Native conversation: `ses_e42a503a4ffezrsVNqdzqGJDuM`;
  harness/model/effort: OpenCode / `openai/gpt-6.1-sol` / default.
- Parent: Hypercube `03a23830`, native `ses_e42fd27dfffekQ56hXQYkGJZrg`;
  OpenCode / `openai/gpt-6.1-sol` / default.
- Dependencies/shared design: none. Paths below are repository-relative.

## Outcome and boundaries

Plain folders remain supported. A failed Git command in a checkout must return
its diagnostic rather than silently classifying the folder as outside Git.
Adding or re-adding a folder must not create or persist `NoGit` from such a
failure. Re-adding an existing folder rechecks its classification and repairs a
stale `NoGit`, preserving its saved identity and sessions.

No automatic retry subsystem, startup sweep, schema migration, duplicate-workspace
merging, path relocation, credential changes, or Git configuration changes.
Existing sessions are not restarted or moved. Updating all consumers of saved
`NoGit` to probe dynamically is outside this change.

## Current behavior and evidence

`internal/workspace/workspace.go` owns `Root`, which currently returns the raw
`git rev-parse --show-toplevel` result. `At` validates the directory but ignores
every `Root` error. `internal/session/sessions.go:AddWorkspace` probes again,
sets `NoGit` from any error, and returns existing records unchanged.
`internal/store/store.go` persists `no_git`; workspace paths are unique and
sessions reference workspace IDs. `Runtime.save` publishes a change after its
transaction commits, so normal saving already refreshes browser snapshots.

`internal/ui/changes.go:gitDir` discards every `Root` error. Its two callers
therefore return a null Changes result or an outside-Git diff diagnostic instead
of the real failure. Existing `fileError`/`refused` handling returns ordinary
Git errors as HTTP 409 with their message, and the browser Changes panel already
renders those errors. No browser component change is needed.

`internal/ui/mention_files.go:gitWorktree` already walks ancestors with
`os.Lstat` looking for `.git`, accepting directories and worktree marker files.
Filesystem errors other than absence propagate. Its callers select Git file
listing versus a plain-directory walk without hiding broken Git metadata.
Preserve that behavior, including existing mention-file tests.

## Chosen design

Make the workspace package own both the metadata check and an explicit
`ErrNotGit` sentinel. Export the existing metadata helper under a clear name
(for example `HasGitMetadata`), documenting that metadata presence is not proof
that Git works. Move its implementation rather than duplicating it; mention-file
listing continues to use this check directly, without adding a root command.

Keep successful `Root` discovery authoritative: run `rev-parse` first and return
its successful root unchanged. On failure, validate that the input is an existing
directory using `Resolve`, then consult the shared ancestor metadata check.
Return directory/metadata inspection errors when that inspection fails. Return
the original Git error when metadata exists, including invalid `.git` markers,
command-start failures and Git's refusal diagnostics. Only return `ErrNotGit`
when inspection establishes no metadata. Do not parse localized stderr or infer
classification from an exit code. This preserves successful Git discovery and
allows ordinary plain folders even when the Git executable cannot run.

`At` may fall back to its resolved directory only for
`errors.Is(err, ErrNotGit)`; all other failures propagate. `gitDir` similarly
returns `ok=false, err=nil` only for the sentinel. Other errors go through its
existing HTTP error path; keep the existing missing-directory check and plain
folder responses.

`AddWorkspace` resolves the input and performs one `Root` discovery instead of
calling `At` and then probing again. Compute root and `NoGit` from success or
the sentinel; return any other error before entering a saving transaction.
Inside `Runtime.save`, prefer an existing record whose path exactly matches the
resolved input, then one whose path matches the discovered checkout root.
Refresh the selected record's `NoGit` through a narrow store update, retaining
ID, path, creation time, and session references. If neither exists, add the
workspace at the discovered root (or the input for a plain folder).

Exact-input precedence repairs a folder previously saved inside a checkout when
Git failed: re-adding that saved subfolder clears its stale classification
without moving it or merging it into a separately saved root workspace. This
exception preserves existing saved paths; newly added Git folders still use the
checkout root. Reclassification works both ways when metadata appears or is
removed. A failed recheck leaves all saved data untouched and reports the error.
No background reconciliation or database migration is needed: the representation
is unchanged and re-add is the explicit repair boundary.

## Implementation sequence

1. Add the shared metadata helper and `ErrNotGit` contract in `internal/workspace`;
   implement the classified `Root` failure path and restrict `At` fallback.
2. Replace the UI-local metadata helper with the workspace helper. Update
   `gitDir` to distinguish the sentinel from failures, preserving both callers'
   ordinary plain-folder behavior and existing error rendering.
3. Replace `AddWorkspace`'s duplicate discovery and unconditional existing-record
   return. Add a narrow `store.Tx` method updating only `no_git` by workspace ID;
   perform the refresh in the existing transaction and publish through `save`.
4. Add behavior regressions at the boundaries below. Inspect whole touched files
   for clarity; do not introduce a general classification cache or new state.

## Verification

Use existing temporary-directory fixtures, Git repository helpers, isolated
databases and HTTP handlers. Initialize repositories/fixtures before installing
a test-local executable named `git` on `PATH`; have it emit a distinctive refusal
message on stderr and exit nonzero. Existing UI tests already use executable
wrappers. Do not modify host ownership, Xcode state, safe.directory, or global
Git settings. Environment-mutating tests must not run in parallel.

- In `internal/workspace/workspace_test.go`, cover a plain directory returning
  `ErrNotGit` from `Root` and itself from `At`; successful discovery from a root
  and nested directory; a linked-worktree `.git` file; and a checkout command
  failure whose diagnostic reaches both `Root` and `At`, never `ErrNotGit`.
  Include an invalid marker case and retain missing-directory rejection.
- In `internal/session/sessions_test.go`, verify plain-folder support and
  duplicate-add identity. On a new checkout failure, assert an error and no
  workspace insertion. On an existing-record failure, assert no saved mutation.
  Seed stale `NoGit=true` for a valid checkout, re-add it and verify returned and
  persisted false with unchanged ID/path/creation time and session association.
  Include a stale saved subfolder with a separately saved checkout root: re-add
  selects and repairs the exact subfolder without merging or relocating either.
  Also verify plain-to-Git and Git-to-plain reclassification on re-add.
- In `internal/ui/changes_test.go`, request both `/changes` and `/diff` against a
  checkout with a failing root command. Assert HTTP 409 and the distinctive
  diagnostic, not null or “folder is outside Git”. Existing plain Changes null
  and missing-directory 404 tests remain; add plain diff conflict coverage if
  absent. Keep mention-file plain-folder and broken-marker regressions passing.

Before committing code run required `make check`, which includes the race-enabled
package checks. During iteration, run narrow regression or package checks only
when they answer a concrete uncertainty or verify an intervening change; do not
routinely repeat the same broad suites before `make check`. Record exact commands,
code revision, results and any environmental blockers in the work log. No build or
implementation tests are required for this planning-only artifact. No manual
live-app run is required: HTTP diagnostic propagation plus the existing browser
error display cover this behavior without affecting running user workspaces.

## Handoff assessment

Ready for independent plan review; no unresolved engineering decisions. This is
one normal execution assignment: one classification contract integrated with its
saved refresh boundary and HTTP consumers, plus meaningful isolated regressions.
Its coordinated state correction and cross-package failure coverage exceed a
single diagnostic-and-test brief, while reusing current persistence, transactions
and rendering keeps it below subsystem-scale work. Detail specifies consequential
behavior without prescribing function bodies. Use the experimental 600–1,500-word
band; no curated project reference manifest was supplied or found. Parent owns
plan commit and independent review; this stage makes no production edits, stages
nothing, commits nothing, and stops here.
