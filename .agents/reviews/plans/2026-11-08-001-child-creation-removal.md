# Plan review: admitted child creation and family removal

## Round 1

- Task ID: `2026-11-08-001-child-creation-removal`
- Mode: plan
- Author: Hypercube worker `child-removal-plan-review`; OpenAI GPT-6.1 Sol
- Independence: distinct worker from the plan author; independently inspected
  production code, callers, fixtures, and test barriers.
- Input (repository-relative):
  `.agents/plans/2026-11-08-001-child-creation-removal.md`
- Input Git blob: `19ebcc13ce53cd96f685c12e13ec18414478534c`
- Plan source HEAD: `c1d4fca1fa171600fc051ed331045be517b4e19f`
- Reviewed checkout HEAD: `08e19ce8a5ec0206ac66efb7a256ac020fe313dc`
- Confirmed directory:
  `/Users/kyle/.local/share/opencode/worktree/803f5f/fix-child-creation-removal`
- Confirmed branch: `fix-child-creation-removal`
- Guidance blobs: no linked shared designs or additional local execution-guidance
  inputs. The plan is self-contained.
- Applicable repository instruction anchors (repository-relative):
  - `AGENTS.md`: `5511f174c0041550712d40f1b803b0a7fe617a21`
  - `agents/development.md`: `fe3a70054763a818cc82f6e13de8d7140a4bdcc8`
- Installed workflow anchors (absolute paths):
  - `/Users/kyle/.agents/skills/xreview/SKILL.md`:
    `713e40a7bcb1d8b9b644cf51cc091fe1e0e1d80c`
  - `/Users/kyle/.agents/skills/xauto/references/workflow.md`:
    `dd843c263afffe9af1bad7dff305dabc79fa5799`
  - `/Users/kyle/.agents/skills/xreview/references/plan-review.md`:
    `8d96c52b094861aa5aa2af12e5e7a72171663dc3`
  - `/Users/kyle/.agents/skills/xreview/references/normalization.md`:
    `5a2afe29e0b3236735fc23bdf6ae65be066bed46`
- Workflow authority: parent reports the user's approved xauto stages supersede
  the repository's legacy k* stage sequence for this task. This worker writes
  only this review; the parent owns commits.

### Verdict

**accept** — work size **right-sized**, detail **appropriate**, readiness
**ready**. Primary action: `accept`; no secondary corrective action. No unresolved
required corrections or blockers.

### Evidence and decision checks

1. **Creation completion has the correct owner and lifetime.** Plan lines 57–69
   register under the same lock that publishes the saved child's launch and defer
   completion until the entire call returns. In `children.go:62–85`, child row,
   approval consumption, and launch registration precede unlocked Git setup.
   `children.go:103–122` then performs setup, launch, rollback, and the successful
   return-value read. A separate operation channel covers that whole interval;
   `live.done` cannot, because `run` closes it before the creator removes the
   failed worktree and discards the row. Keeping completion on rollback errors
   also avoids introducing a permanent wait or new recovery guarantees.

2. **The membership argument holds through rollback.** `claimRemoval` reads the
   children and sets `r.deleting[id]` while holding `r.mu`; `createChild` checks
   that claim under the same lock before its transaction. Capturing operation
   channels from the original child IDs therefore covers every admitted creator
   that could still own cleanup. A row discarded before the claim has already
   passed resource cleanup and transactional approval restoration. A row discarded
   between claim and channel capture is either still tracked or already complete.
   The unconditional refreshed `tx.Children(id)` in plan lines 84–89 is necessary:
   retaining the original snapshot would make archive's `recordEnded` read an
   already deleted child. There is no need to transfer cleanup ownership or
   broaden the workspace API.

3. **The wait belongs before safety checks, not merely before final cleanup.**
   `removeFamily` currently enumerates and checks trees before selecting launches
   and waiting on `done`. The plan's active-family insertion point precedes all
   three operations, and keeps the claim until existing cleanup finishes. Waits
   are outside `r.mu`, allowing `run`, `end`, and `discard` to progress. Admitted
   launches can still start: `launch`/`run` do not reject the parent's removal
   claim. The initial dirty/unmerged refusal thus does not prematurely stop the
   family. Existing post-stop safety checks, per-child partial recording,
   archived deletion, and individual-child paths remain intact.

4. **The proposed Git barriers and failure scenarios are executable.**
   `pauseGit` in `removal_test.go:28–96` runs the real command, saves its exit
   status, and blocks before returning it. `AddWorktree` invokes `worktree add`
   in `parent.Dir`, matching the specified barrier. The child row and launch are
   already published at that point and can be captured under `r.mu`. Missing
   plugins are checked by `run` after worktree creation, not rejected by
   `prepareChild`; `TestChildLifecycle` already exercises this failure. For the
   no-`run` case, the child path is computable before admission using
   `workspace.WorktreeDir(parent.Dir, parent.ID, name)`; a fixture-owned nonempty
   directory causes a real add failure. `AddWorktree` finishes its own branch
   cleanup before `createChild` closes `done` and discards the child. Both failure
   cases therefore exercise real completion owners without inventing a synthetic
   successful-command error. Condition polling can reuse `fixture.until`, and
   the existing barrier cleanup releases blocked Git before runtime teardown.

5. **Verification stays within the requested behavior.** Delete/archive success,
   launch failure, and setup failure check first-request removal, retained archive
   history versus discarded child rows, resource cleanup, and completion. Existing
   refusal, partial-removal, lifecycle, and proposal-retry tests protect unchanged
   guarantees. No unrelated cancellation, persistence, restart recovery, global
   settings changes, or expanded `EndChild` semantics are assigned.

### Work size and normalization

The substantive responsibilities are (a) tracking creation completion independently
of launch lifetime, (b) ordering active-family removal and refreshing its saved
membership, and (c) a local Git integration matrix covering two removal modes and
three creation outcomes. They form one bounded ownership repair with a meaningful
concurrency-test burden. This is comparable to the normalization reference's
existing asynchronous-lifecycle ownership repair and substantive Git integration
suite, rather than a diagnostic/callback edit. Splitting would separate the ordering
from the failure paths its tests must protect; there is no second independent
mechanism requiring a separate assignment.

The bundled metrics helper measured **1,353 whitespace-separated words**, including
15 fenced-code words. The active target is the installed experimental-default
**600–1,500** band. There are zero curated accepted references; the one-plan local
history is descriptive only. The plan is inside the band, but acceptance rests on
the source checks above, not length. Ownership, lifecycle boundaries, and test
oracles are settled without prescribing implementation bodies. No substantial
removable implementation dump or unresolved engineering choice was found.

### Checks and limitations

- Confirmed actual directory, branch, clean starting status, and full HEAD with
  Git. Verified the plan's source commit exists. `git diff --stat` from plan source
  HEAD to reviewed HEAD contains only the plan, so the inspected code matches the
  plan's source revision.
- Computed input and instruction/workflow anchors using `git hash-object --`.
- Ran `python3 /Users/kyle/.agents/skills/xreview/scripts/plan_metrics.py --plans
  .agents/plans --plan .agents/plans/2026-11-08-001-child-creation-removal.md`.
- Inspected `children.go`, `child_options.go`, `runtime.go`, `launch.go`,
  `sessions.go`, `removal.go`, `workspace.go`, relevant existing session tests,
  fixture lifecycle, and Makefile test prerequisites.
- No runtime reproduction, Go tests, race checks, or `make check` were run in this
  plan-only review. Source establishes the conflicting ownership and feasibility,
  not an observed failure frequency. The add barrier deterministically establishes
  admitted-creation/removal overlap; it does not force a particular competing
  branch-deletion interleaving in the old implementation. Implementation must
  report actual regression and race-check results rather than claim a reproduction
  from this review.
- The original GitHub discussion was not independently fetched; scope was checked
  against the supplied issue objective and the plan's faithful outcome brief.

### Handoff

No plan revision is required. Implement the exact accepted input blob above and
run its specified focused race repetitions, full session race suite, and
`make check` before committing code. This acceptance covers the plan, not an
implementation or unperformed checks. Output (repository-relative):
`.agents/reviews/plans/2026-11-08-001-child-creation-removal.md`.
