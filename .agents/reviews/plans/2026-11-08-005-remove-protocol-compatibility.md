# Plan review: plugin and runtime protocol retirement

## Round 1 — 2026-11-08

- Task ID: `2026-11-08-005-remove-protocol-compatibility`
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
  `.agents/plans/2026-11-08-005-remove-protocol-compatibility.md`
- Plan blob: `919cf60a6783c5098b6e94bbd9e8e13ddb7201fc`

### Guidance blobs

Paths are repository-relative; peer plans are transitive shared integration
guidance, not additional implementation assignments for this reviewer.

| Path | Blob |
| --- | --- |
| `.agents/designs/2026-11-08-003-remove-backwards-compatibility.md` | `eccf816e6f7a10a977d65ca0bd920e532f9b0923` |
| `.agents/plans/2026-11-08-003-remove-backwards-compatibility.md` | `16ff0e39fb7c3c940d23526e1bb97aab5b156840` |
| `.agents/plans/2026-11-08-004-remove-browser-compatibility.md` | `781e49e77ac7d71b9ab3217e70fdefad60c2acf3` |
| `.agents/plans/2026-11-08-006-remove-install-compatibility.md` | `8b0305ce46f3a166ec707e0d6e1511b8b5da3f96` |

### Verdict

- Work size: **right-sized**.
- Detail: **appropriate** in level, but contains an incorrect settled contract.
- Readiness: **changes-required**; primary action **resolve-decisions**.

### Findings

1. **Required, open — include the live grant-answer producer in the change.**
   Plan lines 18–23 say current producers already use `choices`, and lines 33–37
   describe `Choice` as solely old saved input. Shared design lines 73–76 repeat
   that assumption. In fact `internal/session/grants.go:29–34` constructs
   `harness.Command{... Choice: item.Choices[0]}`. Both AlwaysAllow's pending
   approvals (`grants.go:77–82`) and automatic granted approvals use recordAllow.
   Removing the field as instructed cannot compile without modifying this live
   producer; dropping its choice would break a required current permission path.

   **Correction:** explicitly change recordAllow to a one-element `Choices`
   array in the same unit as the decoder/field removal, and correct the shared
   baseline and claim that no current encoding changes are needed. Preserve the
   existing per-session/all-session grant tests, including automatic approval
   after restart (`grants_test.go:42–107`), as meaningful required coverage.
   The stand-in's derived Entry.Choice can remain as already planned.

   Keep saved command rows unchanged: `commands.go:60–84` sends their exact
   bytes to their holder; a surviving latest plugin still runs today's Serve,
   which understands both fields. New array answers are also understood by those
   surviving plugins. Explain that existing lifecycle boundary rather than
   claiming no current scalar rows can exist, retaining an obsolete decoder
   indefinitely, or adding a command migration/recovery system. Existing replay
   checks should remain; this finding does not require a new mixed-version suite.
2. **Required, open — correct the linked installer scope disposition.** Shared
   design line 101 removes normal generic owned-link reconciliation as though
   historical-only. See finding 2 in
   [006's review](2026-11-08-006-remove-install-compatibility.md). All linked
   acceptances must use the corrected guidance revision.

### Independent assessment and normalization

The remaining choices are grounded: current startArgs supplies MCP directly;
shared Serve is the right validation boundary; Claude/Pi need nonnull empty
argument arrays, while OpenCode needs its one-element executable array. Dedicated
helper dispatch and latest holder locations do not change. MCP child validation
already rejects dir with fork (`internal/mcp/children.go:134–135`), so removing
the saved-proposal-only exception aligns the runtime with its current contract.
Current offers contain Dismiss. Exact-input workspace identity also serves
present-day Git reclassification, and no-variant OpenCode models still need
ordinary default normalization; preserving both is correct.

The metrics helper measured **1,020 words**, zero fenced-code words, against the
experimental **600–1,500** band from `normalization.md`; no curated accepted
manifest. Protocol retirement across existing producer/decoder/native wiring,
CLI dispatch and small historical runtime exceptions is a bounded cross-layer
assignment comparable to one operator family through established layers. It adds
no transport, lifecycle or substantive independent subsystem. The grant fix fits
this unit; splitting or padding would not improve it.

Run: source/caller/test inspection, targeted searches for Command.Choice/Hypercube
and MCPCommand, Git anchors and metrics. Reused: no suite/build claim; none was
needed for plan review. `make check` remains required before implementation
commits. No actual command queue, live holder, installation or harness was touched.

Handoff: correct recordAllow's specified encoding and its shared baseline, fix
the linked installer disposition, then obtain renewed exact-blob reviews. No code
or plan fixes, staging or commits were made by this reviewer.
