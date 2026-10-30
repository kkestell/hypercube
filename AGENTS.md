# Repository instructions

Kyle is the only user. Favor small implementations that are easy to understand
and change. Use idiomatic Go, plain data, and direct functions. Avoid
abstractions and configuration without a concrete need. Return clear errors
for bad input. Comments explain why; tests check observable behavior.

See [README](README.md) for the documentation index.

## Documentation

- Every fact has one home. Edit its existing home and link to it elsewhere.
  Do not repeat facts across guides, README, instructions, or reports.
- Code owns implementation details. Document necessary usage, decisions, and
  constraints; link to source instead of paraphrasing functions or schemas.
- Add documentation only for a concrete need. Prefer a small edit or deletion
  to a new file. Use short, plain statements; omit generic advice and filler.
- Keep the relevant guide current with behavior changes. Verify claims against
  the current implementation. Do not describe predecessor implementations.
- Do not create plans, work logs, status files, review reports, explanations,
  or research documents unless explicitly requested by the user.
- Remove obsolete documents and unused artifacts as part of the change that
  obsoletes them. Preserve still-relevant limitations in their existing guide.

## Scope and invariants

- Leave PTYs, rendering, input, scrollback, selection, resizing, and pane
  lifetimes to tmux. Do not build a terminal emulator, proxy, or renderer.
- Keep task registries, reporting protocols, workflow engines, and scheduled
  tasks outside the current scope.
- Start processes only for explicit starts, worker creation, or relaunch of an
  exited harness. Stop and delete must close their targets explicitly.
- Configure harnesses only through per-launch arguments, environment, and
  overrides. Leave global and project settings, credentials, and HOME alone.
- Deliver messages through hooks and native wake-ups. Never type a nudge into
  a pane; it can corrupt the user's draft.
- Keep native subagents from consuming or replacing their parent's messages,
  session ID, activity, or waiter. Surface their approvals in the same pane.
- Check live workspace and agent markers before closing or relaunching panes;
  never trust a saved pane number as ownership.
- Keep messages queued after failed writes and lifecycle records after failed
  cleanup. Refuse dirty or unmerged worker worktrees.
- Lock state writes and make them atomic; leave timestamps alone for unchanged
  updates.
- Target macOS and trusted local agents running with the user's credentials.

## Validation

Run `make check` once before committing behavior changes, or when explicitly
requested. During routine editing, use focused checks as needed; do not run full
suites or the tmux lifecycle integration test.
For comments, documentation, and ordinary file renames, use focused searches
and diff inspection; do not run full test, lint, build, or formatting suites.

Add tests only for concrete behavior or bugs that matter to Kyle. Assert
observable outcomes; do not mirror implementation constants, generated
structures, exact prompt prose, or internal call sequences. Harmless refactors
and wording changes should not require test changes. Reuse existing coverage
and delete redundant tests. Do not introduce test-only runtimes, dependencies,
frameworks, or simulated third-party APIs without an explicit request.

Test user-visible message, ownership, and lifecycle behavior with isolated
state and real tmux using local harness stand-ins. Keep test processes and
resources uniquely owned and cleaned up. Never stop the user's tmux server
or installed workspaces. Do not install browser tooling or spend model tokens
on routine validation. Use the [development setup](docs/installation.md#development)
when a live harness smoke test is needed, and clean up sessions you start.
Do not run `make reinstall` yourself.

## Working and communication

Do not commit, create branches, change the task title, or offer to commit unless
asked. Present review findings; do not automatically fix, stage, or commit them
without an explicit request. Keep verification and tool use proportional.

A request to commit and push includes all working-tree changes.

Keep required skill notices to one brief sentence. Do not send opening
acknowledgements, announcements of intent, or generic narration. Intermediate
updates contain concrete findings, material risks, blockers, or necessary
questions. Describe actions as complete only after tool results confirm them.
