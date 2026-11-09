// Where a pull request's repository would be cloned, and whether a workspace
// already tracks it.
import type {Workspace} from '../sessions/types.ts';

const same = (a: string, b: string) => a.toLowerCase() === b.toLowerCase();

// tracking is the first workspace whose GitHub repository is repo.
export const tracking = (workspaces: Workspace[], repo: string) => workspaces.find(w => w.github && same(w.github.repo, repo));
const parent = (path: string) => path.slice(0, Math.max(path.lastIndexOf('/'), 0));

// suggestedFolder puts the clone beside the first workspace owned by the same
// GitHub owner, or beside the first workspace.
export function suggestedFolder(workspaces: Workspace[], repo: string) {
  const [owner, name] = repo.split('/');
  const sibling = workspaces.find(w => w.github && same(w.github.repo.split('/')[0], owner)) ?? workspaces[0];
  return `${sibling ? parent(sibling.path) : ''}/${name}`;
}
