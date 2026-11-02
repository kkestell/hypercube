---
name: hypercube
description: Use whenever a Hypercube lead adds a new feature or changes how an existing feature works.
---

# Hypercube

Leads must use this skill when adding new features or changing existing feature
behavior in this workspace. Follow the workspace's `AGENTS.md` instructions.

## Delegation and planning

For a small, contained change that introduces no new concepts or architectural
changes, delegate implementation directly to a worker with intelligence
appropriate to the task.

For a nontrivial change, including large codebase changes, architectural changes,
or new concepts, delegate planning to an Opus 5.5 worker using `/kplan`. Then
assign implementation from the resulting plan to an appropriate worker.

## UI and UX approval

When the request involves UI changes, first ask whether the user wants to
preview the UI in dev mode or proceed directly with planning. Wait for their
answer before starting or opening the dev UI. If they choose planning, skip the
frontend mockup and approval phase and have an Opus 5.5 worker use `/kplan`.

Default to VS Code's UI and UX conventions unless the user requests otherwise,
whichever path they choose. If they choose a preview, follow these steps:

1. Stop any existing isolated dev mode server, start a new one with `make dev`,
   and open it in the default browser. Leave the installed app and the user's
   workspaces alone.
2. Use the real frontend as the mockup. Stub any required backend behavior; this
   phase is purely frontend.
3. Apply the delegation and review rules to the frontend work. If it requires
   planning, keep that plan limited to the frontend phase.
4. Let the user judge the UI and UX in the browser. Iterate until they explicitly
   approve it; wait for approval before proceeding to full implementation.
5. After approval, shut down the dev server and reset its isolated state with
   `scripts/dev clean` so the next dev session starts fresh.
6. Have an Opus 5.5 worker use `/kplan` to plan the implementation, then delegate
   the work. Frontend code may change during implementation, but preserve the
   core UI and UX the user approved.

## Review and completion

Have a different worker review the change using `/kreview`. Choose a reviewer
from a different model family than the implementation worker.

Tell the reviewer to report findings first and wait for the lead's decisions
before fixing or staging them, overriding `/kreview`'s default immediate fixes.
The lead reads the review and decides which findings merit fixes, which to defer,
and which are speculative and should wait for evidence from actual use. Have the
same reviewer directly fix the findings the lead selects.

Complete the checks required by `AGENTS.md`. The lead owns integration and the
final commit; commit only the changes belonging to the task.
