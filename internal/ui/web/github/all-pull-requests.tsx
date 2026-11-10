// The right panel's Pull Requests tab: every signed-in account's pull
// requests across all repositories, in collapsible groups by the kind of work, with a warning
// for each account that could not be read. A requested review starts a
// session in the workspace that holds the repository. Chat opens a new
// session on the pull request's branch there, first adding the workspace when
// none holds the repository. The list keeps the sidebar's spacing; its right
// padding gives up the width of a scrollbar that takes space, the difference
// between the panel's width (100cqw) and the list's (100%).
import {useState} from 'react';
import {Message} from '../shared/message.tsx';
import {useStore} from '../store.ts';
import {CloneDialog} from './clone-dialog.tsx';
import {tracking} from './clone.ts';
import {PullRequestGroups} from './pull-request-groups.tsx';
import {PullRequestRow} from './pull-request-row.tsx';
import {startChat} from './chat.ts';
import {startReview} from './review.ts';
import {useRowAction} from './row-action.ts';
import {othersMoves, yourMoves} from './turns.ts';
import {GitHubError} from './github-header.tsx';
import {checkingGitHub} from './wording.ts';
import type {PullRequest} from './types.ts';

// A pull request is a row per account that has it.
const rowKey = (account: string, pr: PullRequest) => `${account}\n${pr.url}`;

export function AllPullRequests() {
  const [{accounts, workspaces}, store] = useStore();
  const [cloning, setCloning] = useState<{pr: PullRequest; account: string} | null>(null);
  const [cloneOpen, setCloneOpen] = useState(false);
  const review = useRowAction();
  // The chat after an add runs under the row's key, so the row stays disabled
  // until that chat opens, even before the added workspace is listed.
  const chat = useRowAction();
  const chatIn = (workspaceId: string, account: string, pr: PullRequest) =>
    chat.run(rowKey(account, pr), () => startChat(store, workspaceId, account, pr));
  const failed = accounts.filter(account => account.error);
  const checking = !failed.length && accounts.every(account => !account.updated);
  return <section aria-label="Pull requests across accounts" className="pt-1 pb-2 pl-2 pr-[calc(--spacing(2)-(100cqw-100%))]">
    {failed.map(account => <GitHubError key={account.login} failure={account} subject={`Could not read pull requests for ${account.login}`} />)}
    {cloning && <CloneDialog key={rowKey(cloning.account, cloning.pr)} open={cloneOpen} pr={cloning.pr} account={cloning.account}
      onAdded={id => chatIn(id, cloning.account, cloning.pr)?.()} onClose={() => setCloneOpen(false)} />}
    {checking ? <Message>{checkingGitHub}</Message>
      : <PullRequestGroups turns={[...yourMoves(accounts), ...othersMoves(accounts)]} row={({account, pr, turn}) => {
        const workspace = tracking(workspaces, pr.repo);
        return <PullRequestRow key={rowKey(account, pr)} pr={pr} turn={turn} account={account} showRepo onReview={workspace && review.run(`${workspace.id}\n${pr.number}`, () => startReview(store, workspace.id, pr.number))}
          onChat={workspace ? chatIn(workspace.id, account, pr)
            : chat.running(rowKey(account, pr)) ? undefined : () => { setCloning({pr, account}); setCloneOpen(true); }} />;
      }} />}
  </section>;
}
