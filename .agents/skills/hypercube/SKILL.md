---
name: hypercube
description: Workflow for adding a Hypercube feature or changing how one works. Use only when the user invokes it.
---

# Hypercube

Use this skill only when the user invokes it. Follow the workspace's
`AGENTS.md` instructions.

The purpose of this workflow is to keep the entire system small and understandable
as it evolves. For every change, seek the smallest complete system that meets the
user's requirements, with the fewest concepts and lowest cognitive complexity,
measured across the whole codebase as `AGENTS.md` describes. Less is more;
apply YAGNI and prefer a simple, sufficient solution over a more elaborate one.
Prefer proven approaches and borrow freely from relevant prior art rather than
inventing mechanisms for ordinary problems.

## Restarting active sessions

Before starting work, tell the user clearly whether loading the change will
require restarting active sessions, as `AGENTS.md` describes.

## Delegation and planning

For a small, contained change that introduces no new concepts or architectural
changes, delegate implementation directly to a child session with intelligence
appropriate to the task.

For a nontrivial change, including large codebase changes, architectural changes,
or new concepts, first confirm your assumptions with the user. Then delegate
planning to an Opus 5.5 child session using `/kplan`, giving it the confirmed
assumptions as requirements. Complete any required architectural review before
assigning implementation from the resulting plan to an appropriate child session.

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
the proposed architecture before implementation by an independent child session who
neither authored the plan nor will implement it.

Evaluate the proposal in the context of the entire system, not just the changed
component. Compare it with the smallest complete solution using existing code
and proven prior art. Consider the combined concepts, state, dependencies,
configuration, and lifecycles it creates or removes. Each added responsibility
must serve a current requirement that the simpler solution cannot meet. Challenge
parallel paths, speculative extensibility, and complexity merely moved elsewhere.

The top-level session agent reads the review and resolves justified concerns in the plan before
implementation. Favor removing or simplifying mechanisms over patching a larger
design. Do not expand scope to address speculative problems.

## User interface

Default to VS Code's UI and UX conventions unless the user requests otherwise.

## Review and completion

Have a different child session review the change using `/kreview`. Choose a reviewer
from a different model family than the implementation child session. For architectural
changes, also check that the actual implementation satisfies the architectural
review's conclusions.

The reviewer judges every file the change touches as it stands after the change,
not only the diff, and reports code that has grown hard to follow. Such a
finding is never speculative or deferred: restructure that code before the
change is done.

Tell the reviewer to report findings first and wait for the top-level session agent's decisions
before fixing or staging them, overriding `/kreview`'s default immediate fixes.
The top-level session agent reads the review and decides which findings merit fixes, which to defer,
and which are speculative and should wait for evidence from actual use. Have the
same reviewer directly fix the findings the top-level session agent selects.

Complete the checks required by `AGENTS.md`. When the change affects the
browser view, stop any existing isolated dev server, start a new one with
`make dev`, and give the user its address so they can test the finished work.
Leave the installed app and the user's workspaces alone. Iterate on the user's
feedback, then shut down the dev server and reset its isolated state with
`scripts/dev clean`.

The top-level session agent owns integration and the final commit; commit only the changes
belonging to the task. After the commit, offer to run `make install-service`,
repeating the restart warning if the change needs it.
