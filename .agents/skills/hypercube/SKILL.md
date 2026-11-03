---
name: hypercube
description: Required workflow for nontrivial Hypercube changes, including architecture changes, new concepts, subsystems, and major features. Agents must select it when applicable; simple, contained changes may be made directly.
---

# Hypercube

Follow the workspace's `AGENTS.md`. Its Code style section is the standard for
plans, architectural reviews, implementations, and code reviews.

## When to use this workflow

Use this skill when invoked or when the work changes architecture, system
boundaries, state or lifecycle ownership, introduces a subsystem or major
feature, or spans concerns needing coordinated design and implementation.

Localized fixes, copy, styling, documentation, and small refactors that fit the
existing design may be done directly. Inspect enough code to decide; switch to
this workflow if the work grows beyond a contained change.

## Restarting active sessions

Before starting work, explain whether loading it requires active sessions to
restart, using the criteria in `AGENTS.md`.

## Planning

For nontrivial work, establish assumptions that materially affect scope with the
user, then delegate planning to an Opus 5.5 child session using `/kplan`. Give it the
user's requirements, confirmed assumptions, and the `AGENTS.md` standard. The
plan must show the least costly complete implementation and justify additions
against current requirements. Ask about each material assumption separately,
wait for its answer, then ask the next; give the assumption and plausible
alternatives. Resolve ordinary details without expanding scope.

When the user specifies a review or implementation workflow, follow it. An
explicit invocation for a simple change does not require delegation.

## Architectural review

Before implementing a new subsystem, large feature, or architectural change,
have an independent child session review the plan against `AGENTS.md`. It must
neither author the plan nor implement it. Judge the proposal in the context of
the whole system and compare it with the plan's simpler baseline.

The top-level agent verifies findings and resolves justified concerns in the
plan before assigning implementation to an appropriate child session.

## Code review and completion

Have a child session from a different model family review the implementation
using `/kreview`, applying the `AGENTS.md` standard to whole touched files. For
architectural work, also check the architectural review's conclusions.

Require findings before edits, overriding `/kreview`'s default fixes and staging.
The top-level agent verifies each finding and decides which warrant fixes under
`AGENTS.md`. Fix, stage, and commit only when authorized by the user. When fixes
are authorized, assign them and have the reviewer confirm they are resolved.

Complete the checks in `AGENTS.md`. For browser changes, start an isolated
`make dev` preview and give the user its address for their visual checks. Leave
other sessions' dev servers and the installed app alone. Clean up the preview
and its isolated state when finished; iterate if the user provides feedback.

The top-level agent owns integration and the final commit, including only task
changes. Offer `make install-service` after completion and repeat any required
restart warning.
