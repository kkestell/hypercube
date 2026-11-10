// A GitHub workspace's open pull requests for its account, grouped by the
// kind of work as on the pull requests page, with a Review button that starts
// a review session for a requested review and a Chat button that opens a new
// session on the pull request's branch.
import type {Workspace} from '../sessions/types.ts';
import {Icon} from '../shared/icon.tsx';
import {Message} from '../shared/message.tsx';
import {useStore} from '../store.ts';
import {PullRequestRow} from './pull-request-row.tsx';
import {startChat} from './chat.ts';
import {startReview} from './review.ts';
import {useRowAction} from './row-action.ts';
import {PullRequestGroups} from './pull-request-groups.tsx';
import {othersMoves, yourMoves} from './turns.ts';
import type {GitHubStatus} from './types.ts';
import {checkingGitHub} from './wording.ts';

function emptyText({account, updated, error}: GitHubStatus): string {
  if (!account) return 'Choose an account to see pull requests.';
  if (!updated) return error ? 'Could not load pull requests.' : checkingGitHub;
  return '';
}

export function PullRequests({workspace}: {workspace: Workspace}) {
  const [, store] = useStore();
  const review = useRowAction();
  const chat = useRowAction();
  const status = workspace.github;
  if (!status) return null;
  const empty = emptyText(status);
  const accounts = [{login: status.account, pullRequests: status.pullRequests, updated: status.updated}];
  return <section aria-label="Pull requests">
    <h2 className="group/heading mb-3 text-lg font-semibold">
      <a href={`${status.url}/pulls`} target="_blank" rel="noopener noreferrer" title="All open pull requests on GitHub" className="flex items-center gap-2 hover:underline">
        Pull Requests
        <span className="invisible text-muted-foreground opacity-0 transition-[opacity,visibility] group-hover/heading:visible group-hover/heading:opacity-100 group-focus-within/heading:visible group-focus-within/heading:opacity-100"><Icon name="link-external" /></span>
      </a>
    </h2>
    {empty ? <Message error={!!status.account && !!status.error}>{empty}</Message>
      : <PullRequestGroups turns={[...yourMoves(accounts), ...othersMoves(accounts)]} row={({pr, turn}, implied) =>
        <PullRequestRow key={pr.url} pr={pr} turn={turn} account={status.account} hideReason={implied}
          onReview={review.run(`${workspace.id}\n${pr.number}`, () => startReview(store, workspace.id, pr.number))}
          onChat={chat.run(`${status.account}\n${pr.url}`, () => startChat(store, workspace.id, status.account, pr))} />} />}
  </section>;
}
