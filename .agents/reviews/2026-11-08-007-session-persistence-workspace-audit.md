# Session lifecycle, persistence, and workspace operations audit

## Scope and coverage

Detailed read-only audit of `internal/session`, `internal/store`, and
`internal/workspace`, with relevant terminal, UI, holder, and backend callers
and tests. The emphasis is simplicity, maintainability, and hygiene, including
concrete cleanup and ownership failures. File size alone is not a finding.

Three child sessions audited lifecycle, persistence, and workspace operations
independently. This review consolidates their reports and the parent's checks
of the principal implementation paths. Child reproductions are identified as
such; the parent did not independently rerun those temporary reproductions.

The checkout moved from `decouple-runtime-plan` to `main` during the audits.
The auditors reported the same audited content at `78cc9a8`; the parent
confirmed that commit and branch afterward. These are whole-code findings,
not claims that the latest refactor introduced them. No production code or
tests were changed. Linux container checks, live harness tests, and the full
repository check were not run.

## Summary

| ID | Severity | Category | Fix | Title |
| -- | -------- | -------- | --- | ----- |
| F1 | Medium | Correctness | Open | Session shells are adopted after deletion can be served |
| F2 | Medium | Cleanup | Open | Failed unarchive leaves a worktree that blocks retry |
| F3 | Medium | Lifecycle ownership | Open | Failed end persistence leaves a dead launch registered |
| F4 | Medium | Cleanup ownership | Open | Child setup overlaps family removal |
| F5 | Medium | Error handling | Open | Git failures are persisted or displayed as non-Git state |
| F6 | Medium | Correctness | Open | SQLite connection strings misinterpret question marks in paths |
| F7 | Low | State hygiene | Open | Merge status and deleted-session state are retained |
| F8 | Low | Resource cleanup | Open | Transaction rollback is not deferred |
| F9 | Low | Maintainability | Open | Session field mappings lack a complete round-trip test |
| F10 | Low | Error handling | Open | Worktree setup treats every stat error as absence |
| F11 | Low | Recovery | Open | Missing holder state is treated as an unstarted launch |
| F12 | Low | Hygiene | Open | Unused runtime state and duplicated end-state updates |

## F1. Session shells are adopted after deletion can be served

Medium · Correctness · Open · `internal/session/restore.go:74-105`

Restore adopts workspace shells before calling `serve`, but session shells
only afterward. A deletion request can remove a parked session while its
existing shell is not yet registered. `terminal.Set.Stop` stops only a shell
already in its map (`internal/terminal/terminal.go:119-126`); later adoption
can therefore register a shell whose session has already been deleted.

### Evidence

The parent checked both adoption loops and terminal stop behavior. The
lifecycle auditor reproduced the orphan using a temporary overlay test that
deleted a parked session inside Restore's serving callback. The existing
`TestWorkspaceShellIsAdoptedBeforeServingDeletion` protects the analogous
workspace case but not session shells.

### Resolution

Adopt session shells alongside workspace shells before serving. Keep message
delivery after serving so agents can use callbacks. Add the symmetric
session-shell deletion test.

## F2. Failed unarchive leaves a worktree that blocks retry

Medium · Cleanup · Open · `internal/session/removal.go:61-80`

Unarchive creates a worktree and branch before reading HEAD and saving the
session. Failure in either later step leaves the resources behind while the
session stays archived. A retry then fails because the branch already exists.

### Evidence

The parent compared Unarchive with Create's explicit rollback at
`internal/session/sessions.go:93-105`. The workspace auditor reproduced the
failure with a temporary Git wrapper that failed HEAD lookup in the new
worktree, then confirmed that retry failed on the existing branch. Existing
unarchive coverage exercises success.

### Resolution

Mirror Create's rollback on both HEAD-read and persistence failure, preserving
cleanup errors. Keep session persistence owned by the session package; a new
transaction or recovery framework is unnecessary.

## F3. Failed end persistence leaves a dead launch registered

Medium · Lifecycle ownership · Open · `internal/session/launch.go:265-314`

If the end transaction fails, `endLocked` logs and returns without removing
the live entry or notifying command waiters. The reader nevertheless closes
`done`. Once the entry is marked stopping, `wake` can loop on an already
closed channel without ever finding a replacement
(`internal/session/lifecycle.go:65-85`).

### Evidence

Confirmed by inspection of `run`, `end`, `endLocked`, and `wake`. No injected
database-failure reproduction was run. This requires an end-write failure,
not an ordinary plugin exit. Contrary to one child-report statement, this
failure does not itself remove the holder directory: `end` returns before
resource removal when recording the end fails.

### Resolution

Define a bounded failure policy that completes in-memory launch ownership and
releases waiters even when recording fails. Preserve durable command ownership:
merely deleting the live entry could let a later launch reuse old commands.
Add a focused injected end-write failure test covering stop/restart progress
and command handling. Do not introduce a general recovery subsystem.

## F4. Child setup overlaps family removal

Medium · Cleanup ownership · Open · `internal/session/children.go:82-118`

Child creation registers the row and live entry before worktree setup, then
releases the runtime lock. Family removal can claim that child during setup.
Creation failure and family removal can consequently both attempt worktree
and branch cleanup.

### Evidence

The parent traced registration, the unlocked setup, and the removal claim in
`internal/session/removal.go:85-182`. The lifecycle auditor used a barrier
around worktree creation and reproduced a parent deletion failing with
"branch not found" after the creation path had removed the branch. The parent
survived and required retry; no orphan or data loss was demonstrated.

Existing membership-claim coverage tests creation starting after the claim,
not setup already in progress. Creation also owns a special `done` close when
the reader never starts; that exception must remain explicit.

### Resolution

Give in-progress child setup an explicit exclusion against family removal, or
transfer cleanup ownership once removal claims it. Use the existing admission
model rather than a second lifecycle mechanism. Add a deterministic barrier
test for creation already in progress, including who closes completion and
who removes resources.

## F5. Git failures are persisted or displayed as non-Git state

Medium · Error handling · Open · `internal/workspace/workspace.go:107-115`

`At` ignores any Root failure. AddWorkspace then repeats Root and persists
`NoGit` for any error (`internal/session/sessions.go:23-36`). Changes similarly
treats every Root error as outside Git (`internal/ui/changes.go:25-26`). A
broken repository or permission failure can silently disable features rather
than explain the failure. Re-adding the same directory returns its saved
classification without rechecking it.

### Evidence

The parent checked these paths and the conflicting existing policy in
`internal/ui/mention_files.go:16-37`, which distinguishes missing Git metadata
from failed Git commands. This finding follows from error classification; the
parent did not reproduce a permission or ownership failure.

### Resolution

Put the distinction between non-Git directories and repository failures in
the workspace package and use it consistently. Reuse the useful existing
metadata-detection behavior without assuming all failed Git commands mean
absence. Cover a plain folder and a folder with broken Git metadata in both
workspace admission and Changes behavior.

## F6. SQLite connection strings misinterpret question marks in paths

Medium · Correctness · Open · `internal/store/store.go:170-171`

Open concatenates an unescaped filesystem path with SQLite query parameters.
A legal question mark in the state path becomes the connection string's query
separator. The database can be created at a truncated path outside the intended
private state directory, and connection settings can be lost.

### Evidence

The parent confirmed the construction. The persistence auditor reproduced the
behavior with the installed SQLite driver and a temporary `state?x` path:
the database landed at the truncated path and busy_timeout was zero. Existing
path coverage uses spaces, not a question mark.

### Resolution

Construct a properly escaped `file:` URI. Test a state path containing `?`,
assert the actual database location, and check the configured busy timeout.

## F7. Merge status and deleted-session state are retained

Low · State hygiene · Open · `internal/session/merges.go:34-49`

Merge refresh adds entries with `maps.Copy`, but never removes entries for
archived or deleted sessions. Archived worktree sessions retain Base, so their
snapshots can retain merge status for a removed branch. Runtime maps such as
`seqs` and `opens` also have no corresponding successful-deletion cleanup.

### Evidence

The parent checked merge refresh, runtime fields, and removal paths. The
workspace auditor reproduced retained merge status after measuring and then
archiving a worktree session. The existing archived-worktree test starts with
an empty merge map and does not cover removing a previously measured entry.

### Resolution

Remove obsolete merge status and prune deleted-session state at clear ownership
boundaries. Preserve other workspaces' cached results; replacing the whole
merge map with one workspace's refresh would be incorrect. Account for refresh
ordering, outstanding viewers, and late launch writes before deleting state.
Keep this as explicit cleanup, not a general cache subsystem.

## F8. Transaction rollback is not deferred

Low · Resource cleanup · Open · `internal/store/store.go:201-210`

DB.run rolls back returned errors but leaves a transaction open if its callback
panics. A recovered HTTP-handler panic could therefore retain a connection and
write transaction while the backend continues running.

### Evidence

The parent checked the transaction wrapper. No production callback panic was
demonstrated. BatchReports indexes its input slice, but its only caller already
guards nonempty input; that internal invariant is not a separate validation
defect.

### Resolution

Defer rollback after BeginTx succeeds, then commit on callback success. Add a
focused test that recovers a callback panic and confirms a later write works.
No new panic recovery layer or input-validation API is needed.

## F9. Session field mappings lack a complete round-trip test

Low · Maintainability · Open · `internal/store/store.go:254-285`

The Session fields are independently maintained in the struct, insert,
update, and selection/scan lists. They currently agree, but the repetition
requires coordinated edits and existing tests do not check the complete set.

### Evidence

The persistence auditor compared every mapping and found no current mismatch.
Existing tests check selected fields and migration behavior rather than all
mutable fields together. The parent inspected the explicit mappings.

### Resolution

Add one full Session round-trip test covering insert/read and update/read,
including usage and durable launch state. Keep explicit SQL; reflection or
generation would add more machinery than this problem warrants.

## F10. Worktree setup treats every stat error as absence

Low · Error handling · Open · `internal/workspace/workspace.go:216-229`

The `.gitmodules` and `.worktreeinclude` probes return success for any stat
error. Permission or I/O failures can silently skip requested worktree setup.

### Evidence

Confirmed by parent inspection. No supported-platform I/O failure was
reproduced. Existing setup tests cover normal include copying and submodules.

### Resolution

Ignore only not-found errors and return other failures so existing worktree
setup rollback applies. Add a focused, deterministic non-not-found error test.

## F11. Missing holder state is treated as an unstarted launch

Low · Recovery · Open · `internal/session/launch.go:132-145`

When a saved launch's directory is absent, run starts a new holder without
distinguishing a decided-but-unstarted launch from a previously accepted one.
Restoration also excludes missing directories from lost-launch classification
(`internal/session/restore.go:126-145`). An accepted launch may no longer have
a persisted start command, leaving the replacement plugin uninitialized.

### Evidence

The lifecycle auditor reproduced this by deliberately stopping and removing a
ready launch's holder state while detached, then restoring. A replacement
holder started without a new start command. The parent checked the missing
directory branches. This is exceptional state loss, not normal restoration;
existing tests protect restoration of a decided launch with Ready equal to zero.

### Resolution

Distinguish an unstarted decision from lost state after accepted start. Use the
existing parked/lost behavior or explicitly decide a fresh launch with a new
start command. Preserve the supported unstarted-decision case and test both.

## F12. Unused runtime state and duplicated end-state updates

Low · Hygiene · Open · `internal/session/feed.go:59-73`;
`internal/session/removal.go:249-259,359-369`

The lifecycle auditor found that `write.runtime` is assigned but never read.
EndChild also repeats endArchived's ended/attention/save/drop-queue sequence,
creating two maintenance points for the same state transition.

### Evidence

The auditor searched uses of the field. The parent compared the duplicated
removal transitions; EndChild additionally returns the updated child record,
which a shared helper must preserve.

### Resolution

Remove the unused field and assignment. Reuse the existing end-state helper
without changing EndChild's return value. Explain Launch, Position, Ready,
and Quick beside their persisted declarations, and clarify the Session family
comment. These changes need no new abstraction.

## Qualified risks not promoted to required fixes

- **Short identifiers:** NewID uses 32 random bits
  (`internal/store/store.go:101-104`). Collisions can fail inserts, overwrite
  host-generated transcript items through intentional upsert semantics, or
  select an existing launch directory. None was observed. Widening IDs is a
  small possible improvement, but first check assumptions about user-visible
  short IDs, parsing, and tests; this is not demonstrated corruption.
- **Missing parent worktree:** EndChild uses parent.Dir, whereas family removal
  can fall back to the checkout root. The workspace auditor reproduced failure
  when the parent directory was missing. A fallback changes the merge target
  unless the intended parent branch is preserved, so do not silently broaden
  removal guarantees.
- **Repository-wide prune:** RemoveWorktree invokes `git worktree prune`.
  Its scope includes unrelated worktrees, but expiration policy limits which
  registrations are removed. No collateral deletion was demonstrated.
- **Start timeout precedence:** The lifecycle auditor noted competing timeout
  and start-reply causes but did not reproduce a failure. No new synchronization
  mechanism is justified on that evidence alone.
- **Additional store restrictions:** No write inside a Read callback, lost-update
  SaveSession path, or supported HOME-less launch failure was found. Separate
  read/write interfaces, per-field setters, and a state-directory API redesign
  are not justified.

## Checks run

- Parent: `go test -race ./internal/store ./internal/workspace ./internal/session`
  — passed, all results cached.
- Persistence auditor: `go test -race -count=1 ./internal/store` and
  `go vet ./internal/store` — reported passed.
- Workspace auditor: `go test ./internal/workspace` and focused session tests
  for worktree creation, deletion, unarchive, archive, child ending, merge state,
  and partial family removal — reported passed.
- Lifecycle auditor: focused existing restore, shell-adoption, family-removal,
  and decided-launch restoration tests — reported passed.
- Child temporary reproductions covered startup session-shell orphaning,
  overlapping child setup/removal, missing ready-holder state, failed unarchive,
  stale merge status, missing parent-directory removal, and SQLite path parsing.
  These were not committed regression tests and were not rerun by the parent.
- No production edits were made; full checks, Linux container checks, and live
  harness tests were not run for this audit.

## Verdict

Keep the current package boundaries. Session owns process sequencing and saved
state, workspace owns Git mechanics, and store provides direct transactional
persistence. Durable command recording, transactional reply application,
worktree checks before and after stopping writers, migration coverage, and
read-only Git/index safeguards are sound choices worth preserving.

Address F1-F4 first: they expose ordering or cleanup ownership problems with
visible consequences. F5-F6 correct silent misclassification and path handling.
The remaining findings call for bounded cleanup, clearer invariants, and a few
targeted regression tests, not a broad redesign. All findings remain open;
this review authorizes no implementation and includes no code fixes.
