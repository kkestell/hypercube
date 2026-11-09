# Workspaces outside Git

## Goal

`hypercube` only runs in a Git checkout today. Running it in a directory that is not a Git checkout, such as `~/work/strib`, should start a workspace there whose workers work directly in existing directories, with no branches or worktrees. An example is `create_agent(name="litellm", dir="litellm")` or `dir="/Users/kyle/src/foo"`. Removing such a worker closes its pane and leaves its directory alone. Workspaces in Git checkouts behave exactly as they do now: same state, tools, tool descriptions, SessionStart text, worktrees, and removal checks.

## Related code

- `cmd/hypercube/main.go:46` `current()`: finds the workspace root with `workspace.Root`, which fails outside Git.
- `cmd/hypercube/main.go:117` `start()`: reads the orchestrator's branch with `workspace.Branch` and creates the orchestrator on the first start.
- `cmd/hypercube/main.go:73` `down()`: checks `workspace.Removable` for every worker, then removes worktrees and deletes the state.
- `internal/store/store.go` `State`: the state file. A new workspace-level field goes here.
- `internal/mcp/mcp.go:42` `tools`, `:166` `createAgent`, `:240` `removeAgent`: the tool list and the worktree create and remove paths.
- `internal/harness/harness.go:54`: Codex trust uses `workspace.MainRoot(a.Dir)`, which fails outside Git.
- `internal/hook/hook.go:108` `facts`: SessionStart text, which describes branches, worktrees, and merging.
- `internal/workspace/workspace.go`: Git helpers and `Short`. New `IsRepo` and `Resolve` go here.
- `internal/limits/limits.go:65`: the status line already skips the branch when `workspace.Branch` fails, so it needs no change.

## Decisions

- **The mode is stored, not detected per call.** `State.NoGit` (`json:"no_git,omitempty"`) is set once, when `start()` creates the orchestrator in a directory that is not a Git checkout. Existing state files lack the field, so they stay in Git mode. The orchestrator's empty branch can't serve as the marker, because a Git orchestrator on a detached HEAD has an empty branch too. Storing the mode also keeps `facts` free of Git calls.
- **`NoGit` drives every mode-specific path.** It controls `create_agent`, `remove_agent`, `down`, the tool list, and `facts`. The only exception is Codex trust, which looks at each agent's own directory, because a directory worker may or may not be in a Git checkout.
- **The tool list depends on the mode.** In a `NoGit` workspace, `tools/list` returns a copy of `tools` in which `create_agent` has `dir` as a required property and both `create_agent` and `remove_agent` have descriptions for this mode. Git workspaces keep the current list as it is. In a Git workspace, `createAgent` ignores a `dir` argument, just as it ignores any other unknown argument today.
- **How `dir` is resolved.** `workspace.Resolve(base, dir)` handles three forms:
  - A leading `~/` is expanded to the home directory.
  - A relative path is joined to the orchestrator's directory.
  - An absolute path is used as is.

  The result must be an existing directory. It is not limited to the home directory and is not checked against other agents' directories. Two workers may share a directory, and the SessionStart text warns that they then share one checkout.
- **Directory workers have an empty `Branch`.** `list_agents` and the team list in `facts` then show no branch for them. In a `NoGit` workspace, `facts` leaves out the "on branch" wording.
- **Commits in a directory worker's checkout.** Hypercube says nothing about how to commit there. These are real checkouts that may be on a main branch, so the orchestrator's text tells it to instruct each worker, and the worker's text tells it to follow the orchestrator.
- **`down` in a `NoGit` workspace.** It kills the session and deletes the state. There are no worktrees to check or remove.

## Test plan

- `internal/workspace/workspace_test.go` (new):
  - `IsRepo` is false for a fresh temporary directory, true at the top of a temporary repository after `git init`, and true in a subdirectory of it.
  - `Resolve`:
    - Resolves a relative path against the base, a `~/` path against the home directory (with `HOME` set to a temporary directory), and an absolute path as is.
    - Returns an error for a missing path and for a regular file.
- `internal/hook/hook_test.go`:
  - New `TestNoGitFacts`. The state has `NoGit: true`, an orchestrator in one temporary directory and a worker in another, and a config file like the one in `TestOrchestratorModels`.
  - The orchestrator's SessionStart context contains `create_agent(name, dir,` and the directory-worker guidance. It does not contain `on branch` or `Merge a worker's branch`.
  - The worker's context contains its own directory, has no `on branch`, and contains `Handle branches and commits as the orchestrator tells you`.
  - The existing tests stay unchanged and cover Git mode.
- `internal/mcp/mcp_test.go` (new):
  - `toolsFor` returns `tools` unchanged for a Git state. For a `NoGit` state, `create_agent` requires `name` and `dir`.
  - `createAgent` in a `NoGit` state (config in a temporary `XDG_CONFIG_HOME`) fails without `dir` and fails for a missing directory. In both cases the state still holds only the orchestrator.
  - `removeAgent` in a `NoGit` state removes the worker from the state, and a file in the worker's directory still exists afterwards. Set `TMUX_TMPDIR` to a temporary directory and `TMUX` to empty, so the ignored `KillPane` call reaches no real tmux server.
- `internal/harness/harness_test.go`: `Command` for a Codex agent whose `Dir` is a temporary directory outside Git succeeds, and its argv contains `projects={"<dir>"={trust_level="trusted"}}`.
- Smoke test, following the AGENTS.md isolation rules: run `hypercube` in a temporary directory that is not a Git checkout and contains a temporary repository.
  - Have the orchestrator create a worker with `dir` set to that repository, then send it a message and get a reply.
  - `remove_agent` the worker, then run `hypercube down`.
  - Confirm the repository is untouched and no `.hypercube/` directory or `hypercube/` branch was created.
  - Run `hypercube` in a Git repository and confirm that creating and removing a worker still uses a worktree and a branch.

## Implementation plan

1. `internal/workspace/workspace.go`: add `IsRepo` and `Resolve`.

   ```go
   // IsRepo reports whether dir is inside a Git work tree.
   func IsRepo(dir string) bool {
   	out, err := git(dir, "rev-parse", "--is-inside-work-tree")
   	return err == nil && out == "true"
   }

   // Resolve returns dir as an absolute path. A leading ~/ is the home
   // directory, and a relative path is relative to base. dir must be an
   // existing directory.
   func Resolve(base, dir string) (string, error)
   ```

   Update the package comment to say the package also resolves worker directories.
2. `internal/store/store.go`: add `NoGit bool` to `State`, with `json:"no_git,omitempty"`. Comment: it is set for a workspace whose directory is not a Git checkout, and that workspace's workers work directly in existing directories, without branches or worktrees.
3. `cmd/hypercube/main.go`:
   - `current()`: use the working directory as the root, and call `workspace.Root` only when `workspace.IsRepo(cwd)` is true.
   - `start()`: compute `repo := workspace.IsRepo(root)` and call `workspace.Branch` only when `repo` is true. When the state has no agents, set `s.NoGit = !repo` before appending the orchestrator.
   - `down()`: skip workers when `s.NoGit` is set (`if a.Role != store.Worker || s.NoGit { continue }`), so no Git checks or removals run. Update its doc comment.
4. `internal/harness/harness.go` (Codex): start with `root := a.Dir` and replace it with `workspace.MainRoot(a.Dir)` only when `workspace.IsRepo(a.Dir)` is true. Extend the comment: a directory outside Git is trusted as it is.
5. `internal/mcp/mcp.go`:
   - Add `toolsFor(s store.State) []tool`. For a `NoGit` state it returns a copy of `tools` with `create_agent` and `remove_agent` replaced:
     - `create_agent`: `schema([]string{"name", "dir"}, "name", "dir", "harness", "model", "effort", "message")`. Description: "Orchestrator only. Create a worker that works directly in dir, an existing directory given as an absolute path, a path starting with ~/, or a path relative to the orchestrator's directory, and launch it. Hypercube creates no branch or worktree." Follow it with the existing sentences about defaults, OpenCode, and the message.
     - `remove_agent`: "Orchestrator only. Close a worker's pane and remove it from the team. Its directory is left as it is."
   - `tools/list` in `Serve` loads the state and returns `toolsFor(s)`.
   - `createAgent`:
     - Inside the `Update`, when `s.NoGit` is set, require `args["dir"]` ("dir is required in a workspace that is not a Git checkout"), set `a.Dir` from `workspace.Resolve(orch.Dir, args["dir"])`, and set `a.Branch` to empty. Otherwise keep `workspace.WorktreeDir`.
     - After the `Update`, call `workspace.AddWorktree` only for Git workspaces, then `harness.Launch`.
     - Keep the existing rollback.
     - Return "Created %s (%s) in %s." when there is no branch.
   - `removeAgent`: call `workspace.RemoveWorktree` only when `s.NoGit` is not set.
6. `internal/hook/hook.go` `facts`: when `s.NoGit` is set, write the text below. Otherwise write the current text unchanged.
   - Working directory line: "Your working directory is %s." with no branch. Team lines: `- name (id): role, harness, dir`.
   - The same `send_message` and `list_agents` lines, then:
     - "create_agent(name, dir, harness?, model?, effort?, message?) creates a worker that works directly in dir, an existing directory given as an absolute path, a path starting with ~/, or a path relative to the orchestrator's working directory. Hypercube creates no branch or worktree. The optional message is delivered on the worker's first turn. Orchestrator only."
     - "remove_agent(id) closes a worker's pane and removes it from the team. Its directory is left as it is. Orchestrator only."
   - Orchestrator: "Your working directory is not a Git checkout. Each worker works directly in the directory you give it, usually its own Git checkout, and Hypercube creates no branches or worktrees. Tell each worker how to handle branches and commits in its directory. Workers given the same directory share one checkout. Remove a worker when its work is done. You decide how to divide the work, coordinate the workers, and judge when it is done. The user talks to you in this session." Then the existing sentence about running tasks in sequence or in parallel, and the models text.
   - Worker: "You work directly in your working directory. Hypercube created no branch or worktree for you. Handle branches and commits as the orchestrator tells you. When you finish, or need something, send a message to the orchestrator. A message you send is the only way other agents see your work's status."
7. Add the tests from the test plan, then update AGENTS.md.

## Documentation updates

- `AGENTS.md`:
  - Intro: one orchestrator works in the original checkout and manages workers in Git worktrees. In a directory that is not a Git checkout, workers work directly in existing directories.
  - Principles: "Hypercube supplies identities, worktrees, …" also covers worker directories.
  - Commands:
    - `hypercube` also runs in a directory that is not a Git checkout.
    - `stop` and `down` say "in a workspace" instead of "in a Git checkout".
    - `down` in such a workspace deletes only the state and leaves workers' directories alone.
  - State: the `no_git` field and when it is set.
  - MCP tools: `create_agent`'s `dir`, required and resolved as described, in a `no_git` workspace. `remove_agent` there only closes the pane and deletes the agent and its queued messages. The tool list differs by mode.
  - Message delivery (SessionStart): the facts describe directory workers in a `no_git` workspace.
  - Harnesses:
    - Claude Code: a directory worker's directory must already be trusted, or its pane shows the trust dialog.
    - Codex: a directory outside Git is trusted as it is.
  - Packages: `workspace` also resolves worker directories.
