// The home view: every signed-in account's pull requests across all
// repositories, in collapsible groups by the kind of work, with a warning
// for each account that could not be read. A requested review starts a
// session in the workspace that holds the repository.
import {useState} from 'react';
import {Button} from '../components/ui/button';
import {Icon} from '../shared/icon.tsx';
import {Message} from '../shared/message.tsx';
import {useStore} from '../store.ts';
import {CloneDialog} from './clone-dialog.tsx';
import {tracking} from './clone.ts';
import {PullRequestGroups} from './pull-request-groups.tsx';
import {PullRequestRow} from './pull-request-row.tsx';
import {useReview} from './review.ts';
import {othersMoves, yourMoves} from './turns.ts';
import {GitHubError} from './github-header.tsx';
import {checkingGitHub} from './wording.ts';
import type {PullRequest} from './types.ts';

export function AllPullRequests() {
  const [{accounts, workspaces}, store] = useStore();
  const [cloning, setCloning] = useState<{pr: PullRequest; account: string} | null>(null);
  const [cloneOpen, setCloneOpen] = useState(false);
  const review = useReview(store);
  const failed = accounts.filter(account => account.error);
  const checking = !failed.length && accounts.every(account => !account.updated);
  return <section aria-label="Pull requests across accounts" className="min-h-0 flex-1 overflow-auto p-6">
    {failed.map(account => <GitHubError key={account.login} failure={account} subject={`Could not read pull requests for ${account.login}`} />)}
    {cloning && <CloneDialog key={`${cloning.account}\n${cloning.pr.url}`} open={cloneOpen} pr={cloning.pr} account={cloning.account} onClose={() => setCloneOpen(false)} />}
    {checking ? <Message>{checkingGitHub}</Message>
      : <PullRequestGroups turns={[...yourMoves(accounts), ...othersMoves(accounts)]} row={({account, pr, turn}, implied) => {
        const workspace = tracking(workspaces, pr.repo);
        return <PullRequestRow key={`${account}\n${pr.url}`} pr={pr} turn={turn} account={account} hideReason={implied} place={<>
          <Icon name="repo" className="size-3.5" /><span className="truncate">{pr.repo}</span>
        </>} onReview={workspace && review(workspace.id, pr)} trailing={workspace ? null
          : <Button variant="secondary" size="sm" className="self-center" aria-label={`Clone ${pr.repo}`} onClick={() => { setCloning({pr, account}); setCloneOpen(true); }}>Clone</Button>} />;
      }} />}
  </section>;
}
