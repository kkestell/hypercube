# Git error classification: plan review

## Round 1

- Task ID: `2026-11-08-002-git-error-classification`
- Mode: plan; independent reviewer, not the plan author.
- Verdict: **accept**. Work size: right-sized. Detail: appropriate. Readiness:
  ready for implementation. Primary action: `accept`; secondary actions: none.
- Plan (repository-relative):
  `.agents/plans/2026-11-08-002-git-error-classification.md`
- Actual reviewed plan blob: `6424f2dd339946ca4f58338405f208bc0c7f3861`.
- Guidance blobs: no linked shared designs or transitive execution guidance.
  Applicable repository guidance read: `AGENTS.md`
  (`5511f174c0041550712d40f1b803b0a7fe617a21`) and
  `agents/development.md` (`fe3a70054763a818cc82f6e13de8d7140a4bdcc8`).
- Fresh installed workflow inputs (absolute paths under `/Users/kyle/.agents/skills/`):
  `xreview/SKILL.md`: `122b8b9af6dc0cbd0852ac61be6797bdb7437cad`;
  `xreview/references/plan-review.md`: `0fa64b8e615ad9bad598777672b6620118db9c6b`;
  `xreview/references/normalization.md`: `5a2afe29e0b3236735fc23bdf6ae65be066bed46`;
  `xauto/references/workflow.md`: `6ee172ca48063c36a57c1669857cf3d8aa8eb323`.
- Actual checkout: `/Users/kyle/src/hypercube/.hypercube/03a23830`;
  branch: `hypercube/session-03a23830`; reviewed source HEAD:
  `982a4081a186315c43af179bece609242332caff`. The plan's earlier source
  HEAD is background provenance, not this review's checkout anchor.
- Author: Hypercube worker address `git-errors-plan-review`, coordinator
  `03a23830`; underlying child hex unavailable from exposed metadata. Native
  conversation: `ses_e42a1a009ffegDdoyRUt3YjfK4`; OpenCode /
  `openai/gpt-6.1-sol` / default effort. Native ID obtained through documented
  `opencode.session_move` to the same confirmed absolute checkout.
- Coordinator provenance supplied: `03a23830` /
  `ses_e42fd27dfffekQ56hXQYkGJZrg` / OpenCode /
  `openai/gpt-6.1-sol` / default effort.

### Evidence and findings

No required corrections or unresolved decisions.

Independently read GitHub issue #59 and traced the current code. The chosen design
settles the relevant distinctions without changing workspace ownership:

- Plan lines 54–74 keep successful Git discovery authoritative, reuse the existing
  ancestor `.git` inspection rather than parse diagnostics, and limit fallback to
  an explicit sentinel. This fits `workspace.Root`, `At`, and both `gitDir`
  callers; the moved mention helper retains its current metadata-only behavior.
- Lines 76–92 resolve the important saved-subfolder case. Exact-input precedence
  before discovered root preserves the old record and its sessions even when a
  separate root record exists. One discovery before saving and a `no_git`-only
  update prevent Git failures from becoming persisted classifications without
  introducing relocation, merging, migrations, or background recovery.
- Verified `store.Tx` exposes the needed transaction boundary and immutable saved
  fields, and `Runtime.save` publishes after commit. Existing HTTP `fileError` /
  `refused` paths preserve ordinary refusal messages as 409 responses. Browser
  Changes loading and `git-panel.tsx` already display these errors; extra frontend
  machinery is unnecessary.
- The verification cases are feasible with existing repository/worktree fixtures,
  database helpers, HTTP handlers and executable Git wrappers. Initializing real
  fixtures before overriding PATH avoids interfering with setup. Existing mention
  tests exercise plain-directory fallback and invalid markers. Failure-no-write
  and identity/session-preservation checks protect the consequential state change.

### Normalization and verification

The installed metrics helper measured **1,270 words**, no fenced-code words,
inside the experimental **600–1,500** target from the installed normalization
rubric. No curated accepted reference manifest was supplied. The two local plans'
historical quartiles are descriptive only.

This is comparable to the rubric's normal cross-boundary restoration/refresh
assignment: one classification contract, its saved correction boundary and HTTP
consumers, with state-preservation and failure regressions. A diagnostic-only
brief would leave meaningful selection and persistence decisions implicit;
splitting would separate tightly related parts of this fix without removing an
independent subsystem. The plan specifies those decisions without code dumps or
unearned recovery machinery. Confidence is high for this bounded assignment.

Run: checkout/status/HEAD/branch and blob inspection; issue retrieval; source and
test inspection; installed `plan_metrics.py`. No implementation tests, builds or
experiments run: no concrete uncertainty required execution. Implementation must
run the required `make check` once against its final code, with narrower iteration
checks only as justified. This is plan acceptance, not implementation acceptance.

Handoff: implement the exact reviewed plan. No code or plan edits, staging,
commits, or delegation were performed by this reviewer.
