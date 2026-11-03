# Go fixes review — go-sol-review

Reviewer: go-sol-review. Reviewed `f55f444..1b51b7f`
(`1b51b7fbf987203e0667f5120decba239dfaefe7`) in the go-fixes worktree.

**Changes requested:** G3 introduces one reproduced cleanup/lifecycle regression.
G1, G2, and G4–G12 are resolved. The instruction draft's two consolidation
omissions were reported to the parent, corrected there, and re-read; no
instruction finding remains. Only this report was written in this worktree.

## Finding

### [P2] G3 loses the child record when a stopped creation cannot clean up

Location: `internal/session/children.go:61–95`, especially the return at line 92.
Related: `internal/session/sessions.go:173–204` and
`internal/workspace/workspace.go:224–291`.

The accepted G3 requirement explicitly preserves the child row when worktree
cleanup cannot complete. The rewrite creates the worktree before recording the
child, then checks whether the parent still runs. If Stop overlaps setup, that
check fails and the new pre-registration cleanup branch returns its error. When
Git refuses to remove a dirty worktree, the directory and branch remain, but no
child row was ever saved. The child disappears from the app, cannot be addressed
by `end_session`, and a subsequent creation with that name collides with the
leftover branch/worktree. The later launch-failure branch still keeps the row;
only the new earlier branch loses it.

This is confirmed with the same focused, race-enabled check against both
versions. A Git wrapper runs the real `git worktree add`, signals readiness, and
waits on a FIFO. The test then calls the public `Stop` operation, waits until the
parent's saved `Running` field is false, writes one untracked file into the
completed worktree, and releases setup. It checks that failed cleanup retains
the child record:

```text
f55f444: PASS
1b51b7f: FAIL
failed cleanup left 0 child rows; worktree still present; error:
the session is not running; worktree cleanup failed: worktree .../racing has uncommitted changes
```

The FIFO controls the interleaving; there is no sleep used to guess when setup
finishes. The isolated parent and temporary repository were cleaned up.

Smallest safe fallback: revert the production G3 reorder in `children.go`
(restore 50 lines and remove 48) and retain the independent fixes. That returns
G3's net cost to its baseline and preserves its existing Stop/cleanup ownership.
If G3 is revised instead, retaining a child record until successful cleanup and
coordinating creation with Stop/Delete are required outcomes; simply adding a
row after this failed cleanup is incomplete when the parent has been deleted.
Do not force removal of dirty work. The G4 claim that only `run` closes `done`
must follow whichever G3 implementation is ultimately selected.

The parent received and accepted this finding before the report was committed.
No fix was made by this reviewer.

## Accepted findings checked

| ID | Result and evidence |
| --- | --- |
| G1 | Resolved. `Open` adopts on a cache miss before removal/start. `Adopt` leaves an existing cached owner alone. The existing test now opens before adoption and checks the same PID, retained output, and continued input/output. Proactive adoption remains for unopened shells. |
| G2 | Resolved. `run` starts watchers for restored commands before its single `kickLocked` call. A newly recorded delivery is therefore watched only by `kickLocked`; a restored delivery prevents a new one through `delivering`. Existing send serialization prevents duplicate writes. Readiness, stopping, shutdown, and `AwaitingUser` policy remain in the existing path. |
| G3 | Not resolved safely; see reproduced finding above. Worktree-before-launch and sole `run` ownership remove the original artificial end/manual close, but the new pre-registration failure path loses required state. |
| G4 | Resolved in this commit. Runtime comments exclude `sent` from `mu`; the live comment names the send lock and `run`'s ownership of `done`. No field glossary was added. |
| G5 | Resolved. `First` is deleted, and shell output starts at zero. `Output.Read` already moves an earlier position to the first retained segment. |
| G6 | Resolved. `apply` retains external record validation while the redundant `reply`/`eventLocked` guards are removed. The direct mirror-test caller uses a started live client. `Answer` reuses `running` and preserves its existing finished-request error. |
| G7 | Resolved. Profile directly holds the same tagged JSON fields; all former `Spec` literals are flattened, with no remaining Go references to `harness.Spec`. |
| G8 | Resolved. Launch/delivery timeout messages no longer claim a fixed duration; the shortened-timeout assertion follows the change. Store, Quit, and CreateChild comments are corrected. MCP browser timeout wording is untouched. |
| G9 | Resolved. Snapshot seed callers no longer pass the unused lead name; the seed loops over children directly. Integration helpers use profile/session identity directly. Only the obsolete role-wording assertion was removed; the stale-peer assertion remains. |
| G10 | Resolved. The symlink-target rename loop is deleted; prepareStorage input-sharing, isolation, repeated setup, and resume checks remain. |
| G11 | Resolved. The duplicate input-close test is deleted; the two retained serve tests close input, wait, and check Stop. |
| G12 | Resolved. The old threadSource absence assertions are removed. Server-name tests use create_session/end_session. Claude fixture strings are untouched. |

The commit changes 158 lines in and 173 out. Apart from the permitted G1/G2
additions, the accepted simplifications delete code or stay neutral; no schema,
saved-state format, frontend implementation, or subsystem was added.

## Instruction draft

Read the original checkout's `AGENTS.md` and Hypercube `SKILL.md` in full. The
standard is centralized in AGENTS, and the skill applies it to plans,
architecture, implementation, and whole-file review without copying the full
standard. User authorization remains required before fixing/staging/committing;
explicit user workflows take precedence. The baseline/current-requirement rule,
independent architecture review, different-model code review, restart warnings,
and user-owned visual checks remain.

I reported that the first skill draft had weakened the specified planning model
from Opus 5.5 to Opus and omitted separate, answered questions for material
assumptions. The parent restored both, and I re-read the corrected skill. No
remaining instruction change is requested.

Reviewed draft SHA-256 values:

```text
AGENTS.md: f6bbea3f10568ededa4ec7fd794b92bf3dfc1921c8a706d91caf8583312dca32
.agents/skills/hypercube/SKILL.md: a9262748413199f6fa42dd394b580892d179c0f99ec6c5b89a6d148b23250463
```

## Verification

No broad checks were repeated. The parent reports `make check` passed on the
implementation. The sole unresolved concrete concern justified the two runs of
`TestReviewStoppedCreationKeepsFailedCleanupRow` with `-race`: the baseline
passed and 1b51b7f failed as described above.

The tests used Go overlays under `/tmp`, leaving both worktrees' source/tests
unchanged. Reproduction source:
`/tmp/hypercube-go-fixes-children-review-test.go`; implementation overlay:
`/tmp/hypercube-go-fixes-children-overlay.json`; baseline overlay:
`/tmp/hypercube-go-fixes-children-baseline-overlay.json`. These paths were sent
to the parent for the assigned fix. Go used
`GOCACHE=/tmp/hypercube-go-sol-review-gocache`.

## Coverage

All 24 touched files were read in full at the reviewed implementation. There
are no deleted files in this commit. Exact list from
`git diff --name-only f55f444 1b51b7f`:

```text
cmd/hypercube/integration_test.go
internal/harness/config.go
internal/hold/hold.go
internal/report/report_test.go
internal/session/children.go
internal/session/children_test.go
internal/session/commands.go
internal/session/deliver.go
internal/session/event.go
internal/session/input.go
internal/session/launch.go
internal/session/launch_test.go
internal/session/restart_test.go
internal/session/runtime.go
internal/session/sessions.go
internal/store/store.go
internal/terminal/terminal.go
internal/terminal/terminal_test.go
internal/ui/ui_test.go
plugins/codex/main_test.go
plugins/codex/storage_test.go
plugins/internal/plugin/serve_test.go
plugins/opencode/main_test.go
plugins/pi/main_test.go
```

Dependencies traced include session restoration, fixture helpers, message and
feed handling, mirror-test event callers, workspace worktree creation/removal,
holder output retention, and the already-reviewed harness/plugin implementations
behind the unchanged test helpers. Original-checkout instruction drafts were
reviewed separately as listed above. Other reviewers' reports were not used.
