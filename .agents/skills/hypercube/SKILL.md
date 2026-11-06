---
name: hypercube
description: Required workflow for nontrivial Hypercube changes, including architecture changes, new concepts, subsystems, and major features. Agents must select it when applicable; simple, contained changes may be made directly.
---

# Hypercube

Follow the workspace's `AGENTS.md`. Its Code style section is the standard for
plans, architectural reviews, implementations, and code reviews. Scale each
step below to the change, and follow any workflow, model, or budget the user
specifies.

## When to use this workflow

Use this skill when invoked or when the work changes architecture, system
boundaries, state or lifecycle ownership, introduces a subsystem or major
feature, or spans concerns needing coordinated design and implementation.

Localized fixes, copy, styling, documentation, and small refactors that fit the
existing design may be done directly. Inspect enough code to decide; switch to
this workflow if the work grows beyond a contained change.

## Design against the whole codebase

Judge every nontrivial change against the rest of the codebase, not only the
code it touches. The goal is less code, less to hold in your head, and a
codebase that stays easy to maintain. The smallest diff does not always get
there. Before planning, answer these:

- Does the codebase already do this, wholly or in part? Reuse it, or replace
  the duplicates with one implementation.
- Does the change expose a flaw in an existing abstraction: a wrong boundary,
  a concern split across modules, a concept that no longer fits? Rewrite the
  abstraction so the new behavior is its natural case.
- Is this a natural extension of the architecture, or does it feel bolted on?
  New behavior belongs in the module that owns its concern. When none does,
  give it a module of its own rather than stretching an unrelated one.
- What can the change delete? Obsolete code, special cases, and parallel paths
  that it makes redundant go in the same change.
- Would a reader of the result understand one coherent design, or a core plus
  exceptions?

Refactoring existing code to fit the change is part of the change. The plan
states what is rewritten and deleted, and why the result is simpler overall.

## Restarting active sessions

Before starting work, explain whether loading it requires active sessions to
restart, using the criteria in `AGENTS.md`.

## Planning

For nontrivial work, write a plan with `/kplan`, or delegate it. Confirm
assumptions with the user only when they would change the scope; ask about
each one separately, with plausible alternatives. Resolve ordinary details
without expanding scope. The plan shows the design that leaves the codebase
simplest overall, including any abstraction it rewrites and any code it
deletes, and justifies additions against current requirements.

## Architectural review

Before implementing a new subsystem or an architectural change, have a session
that did not write the plan review it against `AGENTS.md`, in the context of
the whole system and against the plan's simpler baseline. Verify its findings
and resolve justified concerns in the plan. Changes that add no new subsystem
and rewrite no abstraction skip this step.

## Implementation

The agent implementing a plan uses `/kwork`, whether that is this session or a
delegated one. A delegated implementer's brief names the plan and tells it to
invoke `/kwork`.

## Code review and completion

A substantial change gets one review with `/kreview` from a session that did
not write it, applying the `AGENTS.md` standard to whole touched files. The
review reports findings only, overriding `/kreview`'s default fixes and
staging. Verify each finding and fix those the user approves.

Complete the checks in `AGENTS.md`. For browser changes, use the running
`make dev` server, or start one if none is running, and open its address in
the user's browser with `open` for their visual checks. Leave the installed app alone.

Commit and merge only when the user asks. Offer `make install-service` after
completion and repeat any required restart warning.
