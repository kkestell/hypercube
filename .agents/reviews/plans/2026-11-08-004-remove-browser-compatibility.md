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
