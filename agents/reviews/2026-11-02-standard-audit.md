# Review against the project standard

**Conclusion: complete.** The accepted fixes are merged into main and
`make check` passes.

Range: `760eeae..f55f444`. Four independent reviewers worked in separate
worktrees. Their reports list full-file coverage and supporting evidence:
[Go, Opus](2026-11-02-go-opus-review.md),
[Go, Sol](2026-11-02-go-sol-review.md),
[frontend, Opus](2026-11-02-frontend-opus-review.md), and
[frontend, Sol](2026-11-02-frontend-sol-review.md).

The parent verified every proposed finding against current source and its
callers. A focused Bun check independently reproduced the hidden `er` file
request while the Browser tab was active. The remaining confirmed failures
were verified by tracing their paths and inspecting the reviewers' focused
checks. Documentation, instructions, and development scripts were also read;
historical plans and reports do not prescribe current implementation changes.

## Accepted Go findings

- G1: preserve a surviving terminal holder when Open precedes startup Adopt
  (Sol 1).
- G2: deliver a saved queue without a command when reattaching an idle agent
  (Sol 2).
- G3: simplify child worktree/launch ownership, keeping failed worktree cleanup
  manageable and one owner for launch completion (Opus 1).
- G4: correct the mutex ownership comment, without a field glossary (Opus 2).
- G5: delete redundant Holder.First; Output already finds retained output
  (Opus 4).
- G6: remove repeated internal guards and reuse running() in Answer (Opus 5).
- G7: fold the single-use Spec into Profile, retaining its JSON shape (Opus 6).
- G8: correct inaccurate timeout and obsolete naming comments (Opus 7).
- G9: remove obsolete snapshot/integration test scaffolding and old role-word
  assertions (Opus 8).
- G10: remove the test of operating-system symlink replacement (Opus 8).
- G11: remove the duplicate input-close/stop test (Opus 8 amendment).
- G12: remove obsolete threadSource assertions and use current tool names in
  protocol fixtures (Opus 8 amendment).
- G13: list the holder's files and the exit-record ordering in the package
  comment, and explain the output drain and stop grace periods (Opus 3).
- G14: comment the launch fields whose meaning is not evident and group them
  by the lock that guards them (Opus 2).
- G15: build the browser tools' timeout messages from browserTimeout
  (Opus 7).
- G16: replace removed role wording in the Claude plugin fixtures, and drop
  the removed-heading check while keeping the stale-peer assertion (Opus 8).

## Accepted frontend findings

- F1: poll files only for file tabs (Opus 1).
- F2: remove ineffective memo wrappers on store-consuming transcript items
  (Opus 2).
- F3: inline the single-use popup hook and restore menu focus on Escape
  (Opus 3, Sol 2).
- F4: replace the sidebar menu hook's closure round trip with owned state and
  plain item functions (Opus 4).
- F5: inline the single-use nested-session filter and delete its module
  (Opus 5).
- F6: share repeated transcript disclosure markup and styles (Opus 6).
- F7: put shared overlay geometry in the store and placement deduplication in
  its owning hook (Opus 7, Sol 6).
- F8: track asynchronous overlay size changes so the native browser does not
  cover menus (Sol 1).
- F9: label the actual profile, answer, and rename controls (Sol 3).
- F10: expose the selected session with aria-current (Sol 4).
- F11: use supported VS Code controls and theme/shared sizes in the browser
  toolbar, preserving disabled navigation (Sol 5, narrowed).
- F12: remove the dead literal splitter-color fallback (Opus 8).
- F13: correct the obsolete title-bar comment (Opus 9).
- F14: reuse shared icon, border, and indentation sizes (Opus 10, narrowed).
- F15: remove the obsolete agent field from transcript fixtures (Opus 11).
- F16: share duplicated HTTP failure handling, preserving JSON and body-read
  error handling (Opus 12).
- F17: clamp a menu again once it has drawn its items, so a menu opened near
  the bottom of the window stays inside it. The clamp runs before the overlay
  measures the menu, so the browser hides under the moved menu.

## Rejected findings

- Go Opus 3, a doc comment on every exported hold identifier: the names say
  what they do; the package comment carries the contract.
- Go Opus 7, launch and delivery timeout messages built from their durations:
  `time.Duration` formatting reads as "1m0s". The messages now say "in time",
  which stays accurate when tests shorten the bounds.
- Go Opus 8, the stale-peer-snapshot assertion: it checks current behavior.
- Frontend Opus 2, passing the sender down as a prop: removing the
  ineffective `memo` wrappers resolves the misleading code with less.
- Frontend Opus 10, font-size steps using the border token: typography and
  border thickness are different concerns despite equal values.
- Frontend Sol 5, `vscode-toolbar-button`: it has no `disabled` property in
  the locked dependency, so the toolbar uses icon-only `vscode-button`.
- Go Opus, escaped descendant holding stderr (checked, not reported): no
  current plugin can reach it.

## Open

- `editor/editor-tabs.jsx` moves the focus with `requestAnimationFrame` after
  closing or switching tabs, which the standard's rule against timers that
  wait for rendering forbids. Replacing it needs a focus request that
  survives a closed pane's unmount, likely held in the store.
- The Bun suite prints two `ECONNREFUSED` errors to port 80 when run whole,
  on main before these fixes too. No single test file reproduces them alone.

## Instructions

AGENTS.md puts the standard under Code style, and the hypercube skill refers
planning, architectural review, and code review to it.

## Implementation

- `CreateChild` registers the child before creating its worktree, so a
  duplicate name is refused during slow worktree setup and stopping the
  parent covers the child. Each failure path is now linear: a worktree
  failure ends the launch and closes `done` itself, because `run` never
  starts; a launch failure removes the worktree and keeps the row if that
  fails.
- Saved-state formats and holder and plugin protocols are unchanged. Active
  agents and panel shells do not need restarting to load these fixes.
