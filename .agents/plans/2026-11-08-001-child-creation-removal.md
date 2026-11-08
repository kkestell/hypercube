# Wait for admitted child creation before removing a family

- Task ID: `2026-11-08-001-child-creation-removal`
- Status: ready
- Source HEAD: `c1d4fca1fa171600fc051ed331045be517b4e19f`
- Author: Hypercube worker `child-removal-plan`; GPT-6.1 Sol
- Request: [kkestell/hypercube#58](https://github.com/kkestell/hypercube/issues/58)
- Artifact path (repository-relative): `.agents/plans/2026-11-08-001-child-creation-removal.md`
- Dependencies/shared design: none. Parent owns plan commits and independent review.

## Outcome and boundaries

Deleting or archiving a top-level session must not race an already admitted
child creation's worktree setup or failed-start cleanup. A clean family must
be removed on the first request, without competing branch deletions, stale
child-row failures, or an unclosed launch completion channel.

Keep existing dirty/unmerged refusal behavior, checks after writers stop,
partial-removal recording, archive chats, approval restoration after failed
creation, and normal launch shutdown. This is transient ordering inside the
session runtime: no persistence migration, Git API changes, process cancellation
framework, durable recovery, worktree-manager redesign, or unrelated lifecycle
repair. Individual `EndChild` behavior is not expanded by this assignment.

## Current behavior and evidence

`internal/session/children.go:createChild` saves the child, consumes any approval,
records its launch, and inserts `r.live[child.ID]` under `r.mu`. It then releases
the lock before `workspace.AddWorktree` and `r.launch`. On a setup failure it
calls `r.end` and closes `l.done`; on launch failure `run` closes `l.done`.
Creation subsequently performs worktree cleanup and/or `discard`, which deletes
the child row and restores its approval.

`internal/session/removal.go:claimRemoval` takes the same lock, snapshots children,
and claims the parent through `r.deleting`. `removeFamily` checks worktrees,
marks launches stopping, waits for their `done`, checks again, removes trees,
and records deletion/archive. `internal/session/sessions.go:stopLaunches` waits
only for launch completion. That completion precedes creation's rollback, so
the two operations can remove the same branch or use a child snapshot whose
row was discarded. `r.end` also removes the launch from `r.live` before rollback;
absence from that map does not establish that creation finished.

`internal/workspace/workspace.go:AddWorktree` includes Git creation, setup, and
its own failure cleanup. `RemoveWorktree` tolerates resources already absent,
but its existence checks and Git deletion are not atomic with another remover.
Making that API more forgiving would not settle row/approval ownership.

The existing `pauseGit` in `internal/session/removal_test.go` pauses **after**
the matched command completes and before its caller receives the result. Use
`pauseGit(parent.Dir, "worktree", "add")`: the command runs in the parent,
not the child's new directory. Existing membership tests cover removal-first
admission, not creation-first overlap. These conclusions are source-grounded;
no runtime reproduction or Go checks were run during planning.

## Chosen ordering and ownership

Introduce one runtime-only map, keyed by child session ID, of creation-completion
channels, guarded by `r.mu`. Initialize it in `New`. This tracks operations,
not agent lifetimes. Only successfully registered `createChild` calls enter it,
including direct-directory children and calls reached through proposals/offers.

Register the channel in the same locked section that publishes the child's
live launch, before unlocking. Install completion cleanup for every subsequent
return: under `r.mu`, remove the entry and close its channel exactly once.
Completion means the entire creation call has finished, including successful
return-value reads or failed-start worktree cleanup, child discard, and approval
restoration. Publish completion even when rollback itself returns an error;
leave retained resources and rows subject to existing removal safety checks.
Do not register failed admission/transaction attempts.

For an active top-level removal, after obtaining its existing claim and installing
`defer r.releaseRemoval(id)`, capture completion channels for the children in the
claimed membership snapshot under `r.mu`. Wait for those channels outside the
lock, **before** computing `familyWorktrees`, inspecting worktree safety, or
selecting launches to stop. The claim and creation registration share the lock:
every admitted call with a surviving row appears in that child snapshot. A call
already discarded before the snapshot has finished its resource rollback and
approval restoration, even if its deferred completion publication is pending.
A call cannot register later because `createChild` checks the claimed parent
under that same lock. A call that completes between claim and channel capture
needs no wait. A failed creator can delete its row in that interval, but its
original ID still identifies any pending operation until completion.

After all captured calls complete, reread `tx.Children(id)` and use this refreshed
list for worktree enumeration and final persistence. Refresh even when no channels
remain at capture: a creator may already have rolled back after the original
snapshot. Thus successful children are removed/archived normally; discarded
children are not removed or recorded a second time. Keep the claim through the
existing checks, stopping, cleanup and persistence; release it on every error.
Leave individual-child and already-archived removal paths unchanged.

Creation keeps sole ownership of its rollback. Removal begins only after that
ownership finishes, then owns cleanup of the surviving family. Do not make
creation transfer cleanup on seeing `r.deleting`, and do not cancel it merely
because removal was requested. An admitted child may finish starting before
the initial safety checks; a dirty-family refusal consequently leaves its normal
running behavior intact. Waiting can include existing setup/start latency;
retain the plugin's existing start timeout and do not invent a Git timeout.

Keep `l.done` separate: `run` closes it once when launched; `createChild` closes
it once after ending a setup failure when `run` never starts. The new channel
does not replace or delay that contract. Never wait under `r.mu`, which creation,
launch readers, and rollback require. Waiting for `l.done`, holding the runtime
lock through Git, and marking launches stopping before initial checks are rejected:
respectively they miss rollback, block unrelated runtime work, and change refusal
behavior.

## Execution sequence

1. Add the narrowly documented operation map in `internal/session/runtime.go`;
   register and finish operations in `children.go` without duplicating existing
   failure cleanup or changing launch ownership.
2. In `removal.go`, wait for admitted creations and refresh the child snapshot
   at the active-family boundary above. A small helper may capture/wait channels;
   keep channel waits visibly outside the runtime lock. Preserve the rest of the
   removal algorithm and its error paths.
3. Extend `removal_test.go` with creation-first overlap regressions below, reusing
   fixture and Git barriers. No new production test hooks or generalized barrier
   framework. Reassess scope with the parent if another mechanism proves necessary.

## Verification

Add table-driven delete/archive coverage using a worktree parent. Start
`CreateChild` asynchronously, pause its completed Git worktree add, read the saved
child and capture its launch under `r.mu`, then request removal asynchronously.
Wait on the parent's claimed flag under `r.mu` before releasing Git; never use
fixed sleeps. The creation call and removal must finish within bounded test waits.

Exercise successful creation and missing-plugin launch failure. Assert the
creation result matches the case, removal succeeds without retry, child and parent
worktrees/branches are gone, no family launch survives, and the captured launch's
`done` is closed. Delete leaves no family rows; archive retains ended successful
children and parent/chats, while a discarded failed child stays absent. Also
assert the creation operation entry is gone. These ownership assertions supplement
observable removal results, rather than substituting for them.

Cover setup failure's no-`run` completion with the same overlap: precreate a
nonempty directory at the expected child worktree path so the real `worktree add`
fails, and pause its completed failure using the existing barrier. Both removal
modes must finish; creation reports failure and its captured `done` closes. This
checks the other existing completion owner without faking a successful Git command
as a failure. Use fixture-owned files only. Keep ordinary failed-creation and
proposal-retry tests passing.

Run from the repository root during implementation:

```sh
make ui
go test -race ./internal/session -run 'Test(FamilyRemoval|PartialFamilyRemoval|RefusedRemoval|ChildLifecycle|ProposalCanRetry)' -count=10
go test -race ./internal/session
make check
```

Name new overlap tests with the `TestFamilyRemoval` prefix so the focused command
includes them. Inspect the full diff and `gofmt` changed Go files. `make check`
is required before code commits; report failures honestly. No installation,
service restart, browser smoke test, or full build is needed for this plan-only
handoff.

## Handoff and normalization

Ready for independent plan review; no open engineering decisions or blockers.
This is one bounded lifecycle-ordering assignment: operation completion, family
snapshot refresh, and deterministic Git integration regressions form one mechanism.
It is more substantive than changing a diagnostic or passing a callback, and
comparable to repairing ownership in an existing asynchronous lifecycle. Splitting
would separate the barrier from the ordering it must protect. Detail specifies
ownership and executable failure scenarios without prescribing function bodies.
Use the experimental 600–1,500-word band; no curated reference manifest exists
in this checkout. Parent will receive measured metrics and self-check results.
