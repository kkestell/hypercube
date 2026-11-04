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

## Restarting active sessions

Before starting work, explain whether loading it requires active sessions to
restart, using the criteria in `AGENTS.md`.

## Planning

For nontrivial work, write a plan with `/kplan`, or delegate it. Confirm
assumptions with the user only when they would change the scope; ask about
each one separately, with plausible alternatives. Resolve ordinary details
without expanding scope. The plan shows the least costly complete
implementation and justifies additions against current requirements.

## Architectural review

Before implementing a new subsystem or an architectural change, have a session
that did not write the plan review it against `AGENTS.md`, in the context of
the whole system and against the plan's simpler baseline. Verify its findings
and resolve justified concerns in the plan. Features that fit the existing
design skip this step.

## Code review and completion

A substantial change gets one review with `/kreview` from a session that did
not write it, applying the `AGENTS.md` standard to whole touched files. The
review reports findings only, overriding `/kreview`'s default fixes and
staging. Verify each finding and fix those the user approves.

Complete the checks in `AGENTS.md`. For browser changes, use the running
`make dev` server, or start one if none is running, and give the user its
address for their visual checks. Leave the installed app alone.

Commit and merge only when the user asks. Offer `make install-service` after
completion and repeat any required restart warning.
