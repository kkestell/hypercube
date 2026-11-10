// A GitHub workspace's open pull requests for its account, grouped by the
// kind of work as on the pull requests page, with a Review button that starts
// a review session for a requested review.
import type {Workspace} from '../sessions/types.ts';
import {Icon} from '../shared/icon.tsx';
import {useStore} from '../store.ts';
import {PullRequestRow} from './pull-request-row.tsx';
import {useReview} from './review.ts';
import {PullRequestGroups} from './pull-request-groups.tsx';
import {othersMoves, yourMoves} from './turns.ts';
import type {GitHubStatus} from './types.ts';

function emptyText({account, updated, error}: GitHubStatus): string {
  if (!account) return 'Choose an account to see pull requests.';
  if (!updated) return error ? 'Could not load pull requests.' : 'Checking GitHub.';
  return '';
}

export function PullRequests({workspace}: {workspace: Workspace}) {
  const [, store] = useStore();
  const review = useReview(store);
  const status = workspace.github;
  if (!status) return null;
  const empty = emptyText(status);
  const accounts = [{login: status.account, pullRequests: status.pullRequests, updated: status.updated}];
  return <section aria-label="Pull requests">
    <h2 className="group/heading mb-3 flex items-center gap-2 text-lg font-semibold">
      Pull Requests
      <a href={`${status.url}/pulls`} target="_blank" rel="noopener noreferrer" aria-label="All open pull requests on GitHub" title="All open pull requests on GitHub" className="text-muted-foreground opacity-0 focus-visible:opacity-100 group-hover/heading:opacity-100 hover:text-foreground">
        <Icon name="link-external" />
      </a>
    </h2>
    {empty ? <p className="text-sm text-muted-foreground">{empty}</p>
      : <PullRequestGroups turns={[...yourMoves(accounts), ...othersMoves(accounts)]} row={({pr, turn}, implied) =>
        <PullRequestRow key={pr.url} pr={pr} turn={turn} account={status.account} hideReason={implied} onReview={review(workspace.id, pr)} />} />}
  </section>;
}
