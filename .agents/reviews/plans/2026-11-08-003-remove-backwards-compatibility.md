# Plan review: current database and configuration

## Round 1 — 2026-11-08

- Task ID: `2026-11-08-003-remove-backwards-compatibility`
- Mode: `plan`; independent reviewer, not the plan author.
- Author: `compat-plan-review`; Hypercube coordinator ID unavailable (current
  context and `list_sessions` expose no own ID); native conversation
  `ses_e4195b6fcffe3Mncn7jH9SS1YY` from `OPENCODE_SESSION_ID`;
  OpenCode / `openai/gpt-6.1-sol` / `xhigh` (parent creation metadata).
- Parent: Hypercube `a58ee257`, native `ses_e41aa9914ffeXSrMrW4OhBDxsQ`,
  OpenCode / `openai/gpt-6.1-sol` / `xhigh` (`list_sessions`).
- Checkout: `/Users/kyle/.local/share/opencode/worktree/803f5f/remove-backwards-compatibility`;
  branch `task/remove-backwards-compatibility`; initially clean.
- Reviewed checkout HEAD: `2e5d9268b246a95c88c7c97ffb122afd12f90b99`.
- Plan source HEAD: `ae022eeebf9dc90c75a61527267db0682b194057` (verified commit;
  only the five planning records differ from reviewed HEAD).
- Plan path (repository-relative):
  `.agents/plans/2026-11-08-003-remove-backwards-compatibility.md`
- Plan blob: `16ff0e39fb7c3c940d23526e1bb97aab5b156840`

### Guidance blobs

All paths below are repository-relative. The shared design links the peer plans;
their integration guidance was reviewed as well.

| Path | Blob |
| --- | --- |
| `.agents/designs/2026-11-08-003-remove-backwards-compatibility.md` | `eccf816e6f7a10a977d65ca0bd920e532f9b0923` |
| `.agents/plans/2026-11-08-004-remove-browser-compatibility.md` | `781e49e77ac7d71b9ab3217e70fdefad60c2acf3` |
| `.agents/plans/2026-11-08-005-remove-protocol-compatibility.md` | `919cf60a6783c5098b6e94bbd9e8e13ddb7201fc` |
| `.agents/plans/2026-11-08-006-remove-install-compatibility.md` | `8b0305ce46f3a166ec707e0d6e1511b8b5da3f96` |

### Verdict

- Work size: **right-sized**.
- Detail: **appropriate** locally; linked guidance has incorrect settled scope
  and contract claims.
- Readiness: **changes-required**; primary action **resolve-decisions**.

No local persistence-plan correction is required. Acceptance waits on the shared
guidance findings below; this is not conditional acceptance.

### Findings and independent assessment

1. **Required, open — correct the shared current-answer baseline.** Shared design
   lines 73–76 and 95 incorrectly assume current producers use only `choices`.
   `internal/session/grants.go:29–34` currently records scalar `Choice` for
   AlwaysAllow and granted approvals. See finding 1 in
   [005's review](2026-11-08-005-remove-protocol-compatibility.md). Correct the shared
   claim and 005's producer scope before accepting this guidance revision.
2. **Required, open — preserve normal current installer reconciliation.** Shared
   design line 101 classifies the generic owned-broken-link loop as historical
   cleanup. It also serves supported present-day custom bundle plugins. See
   finding 2 in [006's review](2026-11-08-006-remove-install-compatibility.md).
   Correct that shared disposition, then re-review every linked plan.

The persistence decisions themselves match the code. `store.go:109–155,261–270`
confirms `last_active INTEGER NOT NULL` has no default and inserts provide its
required value. Retaining this one baseline is necessary to preserve current
databases without introducing a migration. `migrate.go:78–100` confirms both search
indexes must move into initialization, whereas the version steps and attention
sweep can be deleted; current lifecycle owners clear attention. Config Load
already supplies supported partial-document defaults without rewriting files.
The fixed baseline fixture plus post-reopen insertion protects the actual schema
constraint; existing public transaction APIs can populate all listed records.
Deleting migration-only fixtures does not require a new recovery/version system.
The current config replacement in the app integration fixture preserves its
startup and process-lifecycle responsibilities.

### Normalization, evidence and handoff

The bundled `plan_metrics.py` measured **1,042 words**, zero fenced-code words.
Active target: **600–1,500**, `normalization.md` experimental default; no curated
accepted-reference manifest was supplied. Initialization/config retirement with
an installed-schema fixture and meaningful reopen coverage is comparable to a
normal cross-layer existing-lifecycle repair, not a diagnostic brief or a new
persistence subsystem. No split, filler or trimming is justified.

Run: checkout/branch/status/commit inspection, exact `git hash-object` anchors,
metrics helper and source/test/instruction inspection. Reused: no build/test
claim; planning needs no suite run. Required implementation `make check` remains
in the plan. No actual database, browser profile, installation or running session
was inspected or modified. Review changes only this record and the three peer
review records; no staging or commits.

Handoff: planner fixes the two shared-guidance findings in their owning units;
003 needs no additional mechanism or local plan expansion. Renew this review
against the corrected guidance blobs before execution.
