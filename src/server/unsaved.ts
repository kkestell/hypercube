import { relative } from "node:path";
import type { UnsavedWork } from "../model";
import type { Environment } from "./environment";

/**
 * What deleting a session's clone would lose: in the clone and in each git
 * worktree inside it, changes `git status` shows and commits that are on no
 * remote branch. Stashes and ignored files aren't looked at.
 */
export async function findUnsavedWork(env: Environment, clonePath: string): Promise<UnsavedWork[]> {
  let root: string;
  try {
    root = await env.resolveDir(clonePath, ".");
  } catch {
    return []; // The clone is already gone.
  }
  const git = async (cwd: string, ...args: string[]) => (await env.run(["git", "--no-optional-locks", ...args], cwd)).stdout.split("\n").filter(Boolean);

  try {
    // The first worktree listed is the clone itself. Worktrees outside the clone aren't removed with it.
    const listed = (await git(clonePath, "worktree", "list", "--porcelain")).filter(line => line.startsWith("worktree ")).map(line => line.slice("worktree ".length));
    const dirs = [{ path: ".", dir: clonePath }];
    for (const listedPath of listed.slice(1)) {
      try {
        dirs.push({ path: relative(root, await env.resolveDir(clonePath, listedPath)), dir: listedPath });
      } catch {
        // Outside the clone, or gone.
      }
    }

    // A worktree inside the clone shows up there as an untracked directory; it's checked on its own.
    const nested = new Set(dirs.slice(1).map(other => `?? ${other.path}/`));
    const found: UnsavedWork[] = [];
    const seen = new Set<string>();
    for (const { path, dir } of dirs) {
      const uncommitted = (await git(dir, "status", "--porcelain")).filter(line => !(path === "." && nested.has(line)));
      // Branches are shared by all worktrees, so the clone checks them all and a worktree only its own HEAD (which may be detached).
      const log = await git(dir, "log", ...(path === "." ? ["--branches"] : []), "HEAD", "--not", "--remotes", "--format=%h %s");
      const unpushed = log.filter(line => !seen.has(line.split(" ")[0]!));
      for (const line of unpushed) seen.add(line.split(" ")[0]!);
      if (uncommitted.length || unpushed.length) found.push({ path, uncommitted, unpushed });
    }
    return found;
  } catch (error) {
    return [{ path: ".", uncommitted: [], unpushed: [], error: error instanceof Error ? error.message : String(error) }];
  }
}
