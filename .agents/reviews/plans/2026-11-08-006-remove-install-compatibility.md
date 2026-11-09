# Plan review: build and installation retirement

## Round 1 — 2026-11-08

- Task ID: `2026-11-08-006-remove-install-compatibility`
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
  `.agents/plans/2026-11-08-006-remove-install-compatibility.md`
- Plan blob: `8b0305ce46f3a166ec707e0d6e1511b8b5da3f96`

### Guidance blobs

All paths are repository-relative; linked peer plans were read for the serial
integration contract.

| Path | Blob |
| --- | --- |
| `.agents/designs/2026-11-08-003-remove-backwards-compatibility.md` | `eccf816e6f7a10a977d65ca0bd920e532f9b0923` |
| `.agents/plans/2026-11-08-003-remove-backwards-compatibility.md` | `16ff0e39fb7c3c940d23526e1bb97aab5b156840` |
| `.agents/plans/2026-11-08-004-remove-browser-compatibility.md` | `781e49e77ac7d71b9ab3217e70fdefad60c2acf3` |
| `.agents/plans/2026-11-08-005-remove-protocol-compatibility.md` | `919cf60a6783c5098b6e94bbd9e8e13ddb7201fc` |

### Verdict

- Work size: **right-sized**.
- Detail: **appropriate** in level, but one generic current mechanism is
  incorrectly classified as historical-only.
- Readiness: **changes-required**; primary action **clarify-scope**, secondary
  **resolve-decisions**.

### Findings

1. **Required, open — correct the shared live grant-answer baseline.** Shared
   design lines 73–76 omit the current scalar producer in
   `internal/session/grants.go:29–34`. See finding 1 in
   [005's review](2026-11-08-005-remove-protocol-compatibility.md). Update that
   unit/design before final integrated verification; it need not expand 006.
2. **Required, open — retain generic owned-link pruning on current reinstall.**
   Plan lines 38–43 and 63 instruct deletion of
   `scripts/linux-install:65–70`; shared design line 101 calls it historical
   stale-plugin cleanup. Unlike the named Codex deletes, this loop contains no
   old format, renamed executable or historical-version condition: it removes
   any broken harness link owned by the bundle just replaced. Lines 72–74 of the
   same installer create links for every current bundle plugin, not only the
   three shipped names. Discovery likewise accepts every executable plugin
   (`internal/harnessclient/discovery.go:21–39`), and plan line 74 explicitly
   preserves custom plugins.

   A present-day bundle can therefore install an additional custom plugin;
   reinstalling the current stock bundle replaces its target but must also
   reconcile its now-broken owned command link. Removing this loop loses that
   normal replacement behavior even with no historical Hypercube version
   involved. It is the installer counterpart of the generic uninstall ownership
   check that the plan correctly keeps, not a Codex alias.

   **Correction:** keep the generic existing loop and its ownership restriction;
   remove only named Codex cleanup and old-holder/executable distinctions. Correct
   the shared disposition. Remove Codex-specific fixture naming/assertions, but
   preserve existing generic replacement coverage with a neutral additional
   plugin link if retaining that case; no new cleanup framework, runtime allowlist
   or expanded Linux matrix is needed.

### Independent assessment and normalization

Other retirement decisions are proportionate: bundle build already recreates its
output; the named Codex deletes are obsolete. The macOS executable distinction is
historical, but keeping a current running-app guard avoids launching a closed app
just to quit it. Current SIGTERM/service replacement and holder cleanup remain
separate responsibilities. The dev match can lose only the old combined-command
alternative. Existing Linux tests preserve reinstall, three shell environments,
helper placement and holder survival; the selected bundle/install pair is enough
without release packaging or screenshot work.

`plan_metrics.py` measured **995 words**, six fenced-code words. Target:
experimental **600–1,500** from `normalization.md`; no curated accepted manifest.
The current-only macOS check plus a bounded existing Linux integration suite makes
this a normal platform assignment, not a tiny text-deletion brief. It does not
create an installer subsystem or require splitting. The scope correction removes
an unjustified deletion, not necessary detail.

Run: source/build/script/test/instruction inspection, Git/blob anchors and metrics.
Reused: planner's Docker `linux/aarch64` availability claim only (shared design
lines 118–120), not a Linux-test success claim. Inspection of `scripts/linux/run`
confirms a disposable container, copied sources and separate tester HOME; the
required bundle/install command does not target host installation. Planning ran
no builds, installers or tests. Required implementation `make check`, shell
syntax checks, macOS dry run and disposable Linux bundle/install verification
remain unresolved until execution; no real service/app restart is authorized.

Handoff: retain generic current link reconciliation and update its scope/test
guidance; correct the shared grant baseline in 005. Renew all linked plan reviews
against the changed guidance before execution. Reviewer wrote only the four
review records, without staging, committing or fixing code/plans.

## Round 2 — 2026-11-08

- Task ID: `2026-11-08-006-remove-install-compatibility`; mode: `plan`.
- Author/provenance: same independent `compat-plan-review` as Round 1;
  OpenCode / `openai/gpt-6.1-sol` / `xhigh`, native
  `ses_e4195b6fcffe3Mncn7jH9SS1YY`; own coordinator ID remains unavailable.
- Checkout and branch: unchanged from Round 1; clean at review start.
- Reviewed HEAD: `5c75b1859744f1d8667507b42ddc0e0db8aa1caa`.
- Plan source HEAD: `ae022eeebf9dc90c75a61527267db0682b194057`.
- Plan path (repository-relative):
  `.agents/plans/2026-11-08-006-remove-install-compatibility.md`
- Plan blob: `f6bf8eca928d621d7eb122d1f19a7bd86053896e`.

### Guidance blobs

Repository-relative paths; complete linked design/peer-plan guidance set:

| Path | Blob |
| --- | --- |
| `.agents/designs/2026-11-08-003-remove-backwards-compatibility.md` | `2ddcbac1570b7ef08da4015f76cb05ac06cfd42a` |
| `.agents/plans/2026-11-08-003-remove-backwards-compatibility.md` | `16ff0e39fb7c3c940d23526e1bb97aab5b156840` |
| `.agents/plans/2026-11-08-004-remove-browser-compatibility.md` | `781e49e77ac7d71b9ab3217e70fdefad60c2acf3` |
| `.agents/plans/2026-11-08-005-remove-protocol-compatibility.md` | `cbb2b2eaf2c1cbba7b7d0dfa0fc60e79d400bf89` |

### Verdict and finding dispositions

**Accept.** Work size: **right-sized**; detail: **appropriate**; readiness:
**ready**; action: **accept** only. No unresolved findings.

- Round 1 finding 1 **resolved**: revised shared design and 005 account for the
  current grant producer, its simultaneous array conversion and saved-byte replay
  to surviving latest plugins; existing approval/restart behavior stays covered.
- Round 1 finding 2 **resolved** in plan lines 40–47/67–78 and shared design
  line 111. Generic owned-link pruning remains unchanged. The replacement fixture
  now starts with an actual executable extra-plugin target and resolving owned
  link; the normal stock reinstall removes that target and prunes its link.
  Named historical Codex cleanup and old command/executable distinctions still
  retire; no runtime allowlist or broader cleanup system is introduced.

Rechecking `scripts/linux-install:50–74` confirms the neutral fixture exercises
the existing complete-bundle replacement plus ownership-restricted broken-link
loop, rather than merely asserting a preexisting dangling historical name.
Retaining the existing zsh reinstall and holder checks makes that adjustment
proportionate. The macOS running-app guard, unchanged generic installer, disposable
container boundary and final serial CLI/plugin verification remain coherent.

The bundled metrics helper measures **1,095 words**, six fenced-code words,
within the **600–1,500 experimental-default** band; no curated references. This
remains a normal platform/lifecycle integration assignment by the Round 1
reference comparison. The correction preserves one existing mechanism and
reuses one existing test; it does not create a separate substantial work unit or
justify padding/trimming.

Evidence: full revised plan/shared-guidance/diff inspection, current installer
contract recheck and refreshed Git/blob/metric anchors. Reused Round 1 inspection
of the unchanged runner/test lifecycle and the planner's Docker availability
claim, not any Linux-test success claim. No builds, installers, tests, app quits or
service restarts run. Acceptance is plan-only: required `make check`, changed-shell
syntax checks, macOS dry run and disposable Linux bundle/install tests remain due
during execution. Only review rounds appended; no staging/commits or code changes.
