# Sessions, context, and working copies

A session has its own conversation and working directory. Two sessions can have
separate histories while editing exactly the same files. Starting another chat
does not create another checkout.

## Working in the same checkout

Sessions you create yourself are top-level sessions. They use the workspace's
original folder by default. A workspace is a project folder; for Git projects,
it is the top level of the checkout.
Sessions using that folder share files, staged changes, and the current branch.
If one changes a file or switches branches, the others see that change.

This is useful for reviewing work that has not been committed. For example,
ask an agent:

> Have another agent review the uncommitted changes in this checkout.
> Ask it to report findings without editing files.

The agent can create a child in the same directory. Your original session is
its parent. That reviewer sees the actual working files, including changes
absent from Git history. Its chat appears beneath the parent in the sidebar.

## Working on separate branches

For independent edits, a child can have its own Git worktree: another checkout
on its own branch. The parent can keep working without sharing staged files
or mixing the child's edits into its own working copy.

A parent might therefore use both arrangements:

```text
Parent session         original checkout
├── Review child       same checkout, including uncommitted edits
└── Implementation     separate worktree and branch
```

A new worktree starts at the parent's current commit. It does **not** include
the parent's uncommitted changes. If a child's work depends on those changes,
they need to be committed first, or the child needs to use the existing checkout.
Ignored local files such as `.env` can be supplied through
[`.worktreeinclude`](configuration.md#local-files-in-worktrees).

Work from a child's branch reaches the parent through a Git merge. The child
is instructed to commit on its branch; the parent merges it when asked. Ending
the child removes its managed worktree only after Git confirms that its changes
are committed and merged.

You can also give a session you create its own worktree: turn on **Worktree**
before its first prompt. Its children's branches merge into that session's
branch. The **Merge** button below its message box asks the agent to integrate
its work into the original checkout.

Outside Git, children need an existing directory because Hypercube cannot
create worktrees. Worktrees separate working files; agents still have access
to the rest of your local filesystem.

## What the other agent knows

A fresh child receives its assignment and local instructions. A forked child
inherits the parent's saved native conversation at the time it starts; its
Hypercube transcript begins with its own assignment. Forks use the parent's
harness and can choose another model. A fork works where its parent works: a
parent in the original checkout shares its folder, uncommitted files included,
with the fork, and a parent in its own worktree gives the fork a new worktree
from its last commit. A child uses its parent's harness, model, and effort unless the parent passes
others, and fresh children can use any installed harness. Native
restrictions still apply: OpenCode forks need the parent's folder, so they
cannot use a new worktree.

When a child successfully finishes a turn, Hypercube automatically reports the
first 400 characters of its final response. Progress text is excluded. A
longer response's report gives its full length, and the parent can read the
child's transcript, or any top-level session's, with its `read_session` tool.
Reports wait until the parent is idle and no live child is working, then arrive
together and let the parent continue. A child waiting for your answer does not
hold them up: the report names it, and the parent continues other work while
you answer it. Failed launches and ended children do not hold them up either.

Agents can still send explicit messages during work. These do not wait for the
other children to finish. Messages appear in both conversations, so you can
see what was communicated. You can also speak to a child directly.

A parent can message its children and other top-level sessions, even in
another workspace. A child can message only its parent and cannot create
children of its own. Ended sessions cannot receive agent messages.

Each session starts when it is used and stops after it has been idle; see
[session lifecycle](usage.md#resume-and-cleanup).

Native subagents created by a harness are separate from Hypercube children.
Their activity appears inside a tool call in the existing chat rather than as
another session in the sidebar.
