---
name: hypercube
description: Use whenever a Hypercube lead adds a new feature or changes how an existing feature works.
---

# Hypercube

Leads must use this skill when adding new features or changing existing feature
behavior in this workspace. Follow the workspace's `AGENTS.md` instructions.

The purpose of this workflow is to keep the entire system small and understandable
as it evolves. For every change, seek the smallest complete system that meets the
user's requirements, with the fewest concepts and lowest cognitive complexity,
measured across the whole codebase as `AGENTS.md` describes. Less is more;
apply YAGNI and prefer a simple, sufficient solution over a more elaborate one.
Prefer proven approaches and borrow freely from relevant prior art rather than
inventing mechanisms for ordinary problems.

## Delegation and planning

For a small, contained change that introduces no new concepts or architectural
changes, delegate implementation directly to a worker with intelligence
appropriate to the task.

For a nontrivial change, including large codebase changes, architectural changes,
or new concepts, first confirm your assumptions with the user. Then delegate
planning to an Opus 5.5 worker using `/kplan`, giving it the confirmed
assumptions as requirements. Complete any required architectural review before
assigning implementation from the resulting plan to an appropriate worker.

## Confirming assumptions

Read enough of the code and docs to turn the request into concrete decisions:
behavior, user interface, state, lifecycles, configuration, and what is removed.
Present each assumption to the user as its own question, one at a time, and
wait for the answer before asking the next. State the assumption plainly, offer
it as the first choice, and list the plausible alternatives that would lead to
different work. Ask the most fundamental questions first, since their answers
shape the later ones. Stop when the remaining details are ones you can decide
yourself.

## Independent architectural review

New subsystems, large features, and every architectural change require review of
the proposed architecture before implementation by an independent worker who
neither authored the plan nor will implement it.

Evaluate the proposal in the context of the entire system, not just the changed
component. Compare it with the smallest complete solution using existing code
and proven prior art. Consider the combined concepts, state, dependencies,
configuration, and lifecycles it creates or removes. Each added responsibility
must serve a current requirement that the simpler solution cannot meet. Challenge
parallel paths, speculative extensibility, and complexity merely moved elsewhere.

The lead reads the review and resolves justified concerns in the plan before
implementation. Favor removing or simplifying mechanisms over patching a larger
design. Do not expand scope to address speculative problems.

## User interface

Default to VS Code's UI and UX conventions unless the user requests otherwise.

## Review and completion

Have a different worker review the change using `/kreview`. Choose a reviewer
from a different model family than the implementation worker. For architectural
changes, also check that the actual implementation satisfies the architectural
review's conclusions.

The reviewer judges every file the change touches as it stands after the change,
not only the diff, and reports code that has grown hard to follow. Such a
finding is never speculative or deferred: restructure that code before the
change is done.

Tell the reviewer to report findings first and wait for the lead's decisions
before fixing or staging them, overriding `/kreview`'s default immediate fixes.
The lead reads the review and decides which findings merit fixes, which to defer,
and which are speculative and should wait for evidence from actual use. Have the
same reviewer directly fix the findings the lead selects.

Complete the checks required by `AGENTS.md`. When the change affects the
browser view, stop any existing isolated dev server, start a new one with
`make dev`, and open it in the default browser so the user can test the finished
work. Leave the installed app and the user's workspaces alone. Iterate on the
user's feedback, then shut down the dev server and reset its isolated state with
`scripts/dev clean`.

The lead owns integration and the final commit; commit only the changes
belonging to the task.
