# Plan review: current browser-state formats

## Round 1 — 2026-11-08

- Task ID: `2026-11-08-004-remove-browser-compatibility`
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
  `.agents/plans/2026-11-08-004-remove-browser-compatibility.md`
- Plan blob: `781e49e77ac7d71b9ab3217e70fdefad60c2acf3`

### Guidance blobs

All paths below are repository-relative; peer plans are linked through the
shared design and were inspected for integration guidance.

| Path | Blob |
| --- | --- |
| `.agents/designs/2026-11-08-003-remove-backwards-compatibility.md` | `eccf816e6f7a10a977d65ca0bd920e532f9b0923` |
| `.agents/plans/2026-11-08-003-remove-backwards-compatibility.md` | `16ff0e39fb7c3c940d23526e1bb97aab5b156840` |
| `.agents/plans/2026-11-08-005-remove-protocol-compatibility.md` | `919cf60a6783c5098b6e94bbd9e8e13ddb7201fc` |
| `.agents/plans/2026-11-08-006-remove-install-compatibility.md` | `8b0305ce46f3a166ec707e0d6e1511b8b5da3f96` |

### Verdict

- Work size: **right-sized**.
- Detail: **appropriate** locally; linked guidance has incorrect settled claims.
- Readiness: **changes-required**; primary action **resolve-decisions**.

No local browser-plan correction is required. The linked guidance must be fixed
and re-reviewed; this is not conditional acceptance.

### Findings and independent assessment

1. **Required, open — correct the shared current-answer baseline.** Shared design
   lines 73–76 and 95 omit the live scalar producer in
   `internal/session/grants.go:29–34`. See finding 1 in
   [005's review](2026-11-08-005-remove-protocol-compatibility.md).
2. **Required, open — correct the shared installer disposition.** Shared design
   line 101 deletes generic reconciliation also used for current custom plugin
   bundles. See finding 2 in
   [006's review](2026-11-08-006-remove-install-compatibility.md).

The browser decisions match actual current writers, rather than merely comments:
`view-storage.ts:11–20` always snapshots `workspaceViews` and current workbench
sizes; `editor/view-state.ts:7–18` saves editor choices only; current attachment
still combines those with transcript disclosures. Thus keeping SessionViewState
but removing global-panel/agent-tab conversion is correct. `draft-storage.ts:81–89`
always emits `unsent` and `unsentDrafts`; `selection.ts:127–136` creates contexts
with null settings/worktree when defaults have not arrived. Requiring those
current fields while retaining null/default resolution preserves the writer
contract. Existing quota/URL/round-trip cases contain useful nonmigration
coverage that the plan explicitly preserves.

`harness/protocol.go:137–141,287–310` validates current kinds but permits absent
Final; the transcript renderer must still display startup, streaming and
failed/interrupted rows without full markers. Its exhaustive kind change and
removal of bespoke retired labels do not require changing turn grouping or
discarding saved tool content. Generic names still render those tool identities.

### Normalization, evidence and handoff

`plan_metrics.py` measured **981 words**, zero fenced-code words. Active target:
**600–1,500**, `normalization.md` experimental default; no curated accepted
references. Three small existing storage boundaries plus obsolete renderer
mappings and round-trip preservation fit a normal execution assignment, like
restored-tab loading/refresh ownership. No new editor or storage lifecycle is
assigned. It is neither a tiny diagnostic brief nor a browser replacement; no
split, expansion or trimming is warranted.

Run: checkout/commit/blob inspection, metrics and current code/test inspection.
Reused: no build/test result. Planning requires no suites; implementation's
`make check` supplies UI build/typecheck/Bun coverage. No real profiles or browser
storage were examined or changed; historical-format data is intentionally outside
the accepted reader contract, not a reason to introduce a cleanup system. No
code edits, staging or commits.

Handoff: correct the shared findings in 005/006 and the shared design; renew all
linked reviews with new exact guidance anchors. Do not expand 004 to repair
unrelated capabilities or invent storage versioning.

## Round 2 — 2026-11-08

- Task ID: `2026-11-08-004-remove-browser-compatibility`; mode: `plan`.
- Author/provenance: same independent `compat-plan-review` as Round 1;
  OpenCode / `openai/gpt-6.1-sol` / `xhigh`, native
  `ses_e4195b6fcffe3Mncn7jH9SS1YY`; own coordinator ID remains unavailable.
- Checkout and branch: unchanged from Round 1; clean at review start.
- Reviewed HEAD: `5c75b1859744f1d8667507b42ddc0e0db8aa1caa`.
- Plan source HEAD: `ae022eeebf9dc90c75a61527267db0682b194057`.
- Plan path (repository-relative):
  `.agents/plans/2026-11-08-004-remove-browser-compatibility.md`
- Plan blob: `781e49e77ac7d71b9ab3217e70fdefad60c2acf3` (unchanged).

### Guidance blobs

Repository-relative paths; complete linked design/peer-plan guidance set:

| Path | Blob |
| --- | --- |
| `.agents/designs/2026-11-08-003-remove-backwards-compatibility.md` | `2ddcbac1570b7ef08da4015f76cb05ac06cfd42a` |
| `.agents/plans/2026-11-08-003-remove-backwards-compatibility.md` | `16ff0e39fb7c3c940d23526e1bb97aab5b156840` |
| `.agents/plans/2026-11-08-005-remove-protocol-compatibility.md` | `cbb2b2eaf2c1cbba7b7d0dfa0fc60e79d400bf89` |
| `.agents/plans/2026-11-08-006-remove-install-compatibility.md` | `f6bf8eca928d621d7eb122d1f19a7bd86053896e` |

### Verdict and finding dispositions

**Accept.** Work size: **right-sized**; detail: **appropriate**; readiness:
**ready**; action: **accept** only. No unresolved findings.

- Round 1 finding 1 **resolved**: shared design lines 75–85 and revised 005
  explicitly change recordAllow while preserving stored bytes and current grant
  scope, automatic approvals and restart coverage.
- Round 1 finding 2 **resolved**: shared design line 111 and revised 006 preserve
  normal current owned-link reconciliation, separating it from named historical
  cleanup.

The unchanged browser plan retains current writer shapes, null draft semantics,
the current composite editor/disclosure view and optional transcript markers.
It removes obsolete conversion rather than current defaults or generic rendering;
the corrected peer guidance does not change these boundaries.

Normalization remains **981 words**, zero fenced-code words, measured by the
bundled helper against the **600–1,500 experimental-default** band. No curated
references; Round 1's comparison to a normal restored-tab/lifecycle repair remains
valid. No added work, missing consequential decision or removable detail warrants
normalization changes.

Evidence: inspected the complete changed guidance, refreshed Git/blob/metrics,
and reused Round 1 source/test inspection because all executable source remains
identical to source HEAD. Only the named nine workflow records differ. No builds,
tests or browser-profile actions run. Acceptance covers these inputs only;
implementation must still complete its planned `make check`. Only review rounds
appended; no staging/commits or code changes.
