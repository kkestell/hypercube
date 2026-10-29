# Agents, sessions and where they work

How workspaces, sessions and agents map onto processes, directories and git. This is the design for bringing agent processes back; nothing here is implemented on `bun` yet. Process management as built before lives on branch `process-management`.

Harnesses are treated as generic OS processes here. How a specific harness is launched and instrumented is the harness plugin's job (see `agents/research/harnesses/SUMMARY.md`).

## Concepts

**Environment.** Where processes run and what filesystem they see: the host, a sandbox on the host (bubblewrap on Linux, the macOS equivalent), or a container. An environment can:

- start a process in a PTY (command, directory, environment variables) and kill it, with everything it owns;
- run one-off commands, such as `git worktree add`;
- map paths between its view and the host's;
- be set up and torn down (e.g. start and stop a container).

The host environment is what `process-management` built. Sandbox and container are other implementations of the same interface. Each workspace has one environment, and everything in the workspace runs in it.

**Workspace.** A source plus an environment. For now the source is a local repo path: the main checkout.

**Working copy.** A directory inside the environment where agents work: the main checkout, or a git worktree created from it. Hypercube creates worktrees, tracks which agents use each one, and removes them. Several agents can share a working copy.

**Session.** A team with a home working copy. Exactly one lead, plus any number of workers. A session can be about code, or just a place to ask questions, read logs or poke at infrastructure with the project's context; it doesn't have to produce changes.

**Agent.** A harness process running in a working copy, started through the workspace's environment. Its role is lead or worker. Its working copy is assigned when it's created and never changes.

## Working copies and git

**Session home.** Creating a session creates a worktree on a new branch, `hypercube/<session-slug>`, from the main checkout's current `HEAD` commit (not its uncommitted changes). The lead works there. Sessions never share files while they work.

**Workers.** When the lead starts a worker, it chooses where the worker works:

- **the lead's working copy** (shared with the lead and any other workers there), or
- **its own worktree** on a new branch, `hypercube/<session-slug>/<worker-slug>`, from the session branch.

Every mix is allowed: all shared, all separate, or some of each.

**Location.** Worktrees live in a Hypercube directory outside the repo, `~/.cache/hypercube/worktrees/<workspace>/<session>/…`, so they never appear in the main checkout's tree or tooling. In a container or sandbox they live wherever the environment can see them; the environment maps the path.

**Integration.** Hypercube never merges anything. The lead merges its workers' branches into the session branch. When the session's work is done, the lead lands it, by merging into the main checkout or opening a pull request, after coordinating with the other leads in the workspace.

**Cleanup.** A session's worktrees and branches stay until the user deletes the session, since many sessions never change code and some are kept for a long time. A worker's worktree stays at least as long as its session.

**Non-git directories.** Eventually a workspace can be any directory. Without git there are no worktrees: the session's home is the directory itself and every agent shares it. Git is the main case; this is the fallback.

## Messaging

- Within a session, the lead and its workers message each other.
- Within a workspace, leads message each other, mainly to coordinate landing work on the main branch.
- Workers don't message outside their session; anything cross-session goes through their lead.

Messaging is how leads avoid landing conflicting work at the same time. There is no lock in v1.

How a message reaches a running harness differs per harness (hooks, plugins, typing into the PTY); see the research summary.

## Decisions

| Question | Decision |
|---|---|
| Where a session's lead works | Its own worktree on `hypercube/<session-slug>` |
| Where workers work | The lead chooses per worker: the lead's working copy or a new worktree from the session branch |
| What a session branches from | The main checkout's `HEAD` commit |
| Where worktrees live | `~/.cache/hypercube/worktrees/…`, outside the repo |
| Who merges and lands | The lead, by merge into the main checkout or a pull request, after coordinating with other leads |
| When worktrees and branches are removed | Only when the user deletes the session |
| Environment scope | One per workspace |
| Landing coordination | Messaging between leads; no lock in v1 |
| Non-git workspaces | Part of the design, later; everyone shares the directory |

## Open questions

- Deleting a session with unmerged commits: delete the branch anyway, keep it, or ask?
- How the lead asks Hypercube to start a worker (likely an MCP tool, as in the Rust Hypercube), and what else it can ask for.
- Container specifics: whether the repo is bind-mounted or cloned, and how credentials get in.
- Whether a lead picks its workers' harnesses, or the session or workspace sets a default.
