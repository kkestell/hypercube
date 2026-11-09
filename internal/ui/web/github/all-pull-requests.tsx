// The home view: every signed-in account's pull requests across all
// repositories, in collapsible groups by the kind of work, with a warning
// for each account that could not be read. A requested review goes to a
// session in the workspace that holds the repository.
import {useState} from 'react';
import {Button} from '../components/ui/button';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {Chevron, Icon} from '../shared/icon.tsx';
import {expandPullRequestGroup} from '../app/workbench.ts';
import type {Workspace} from '../sessions/types.ts';
import {useStore} from '../store.ts';
import {CloneDialog} from './clone-dialog.tsx';
import {tracking} from './clone.ts';
import {PullRequestRow} from './pull-request-row.tsx';
import {SendDialog} from './send-dialog.tsx';
import {othersMoves, turnGroups, yourMoves} from './turns.ts';
import {failureText} from './wording.ts';
import type {PullRequest} from './types.ts';

export function AllPullRequests() {
  const [{accounts, workspaces, workbench}, store] = useStore();
  const [cloning, setCloning] = useState<{pr: PullRequest; account: string} | null>(null);
  const [reviewing, setReviewing] = useState<{pr: PullRequest; workspace: Workspace} | null>(null);
  const all = [...yourMoves(accounts), ...othersMoves(accounts)];
  const failed = accounts.filter(account => account.error);
  const checking = !failed.length && accounts.every(account => !account.updated);
  const groups = turnGroups.map(group => ({...group, turns: all.filter(({turn}) => turn.group === group.id)})).filter(group => group.turns.length);
  return <section aria-label="Pull requests across accounts" className="min-h-0 flex-1 overflow-auto p-6">
    <h1 className="mb-4 text-lg font-medium">Pull Requests</h1>
    {failed.map(({login, error, updated}) => <p key={login} className="mb-2 flex items-center gap-1.5 text-sm text-muted-foreground">
      <Icon name="warning" className="text-destructive" />
      <span>{failureText(`Could not read pull requests for ${login}`, error!, updated)}</span>
    </p>)}
    {reviewing && <SendDialog {...reviewing} onClose={() => setReviewing(null)} />}
    {cloning && <CloneDialog pr={cloning.pr} account={cloning.account} onClose={() => setCloning(null)} />}
    {checking ? <p className="text-sm text-muted-foreground">Checking GitHub.</p> : <>
      {!groups.some(group => group.id !== 'waiting') && <p className="mb-4 text-sm text-muted-foreground">Nothing needs you.</p>}
      {groups.map(group => {
        const open = !workbench.collapsedPullRequestGroups[group.id];
        return <Collapsible key={group.id} open={open} onOpenChange={value => store.update(s => expandPullRequestGroup(s, group.id, value))} className="mb-4">
          <CollapsibleTrigger asChild>
            <Button variant="ghost" size="sm" className="-ml-2 mb-1 h-7 gap-1.5 px-2 text-sm font-medium">
              <Chevron open={open} />
              <span>{group.title}</span>
              <span className="text-muted-foreground">{group.turns.length}</span>
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <ul aria-label={group.title} className="space-y-2">
              {group.turns.map(({account, pr, turn}) => {
                const workspace = tracking(workspaces, pr.repo);
                return <PullRequestRow key={`${account}\n${pr.url}`} pr={pr} turn={turn} account={account} hideReason={turn.reason === group.implied} place={<>
                  In <a href={`https://github.com/${pr.repo}`} target="_blank" rel="noopener noreferrer" className="underline underline-offset-2">{pr.repo}</a> for {account}.
                </>} onReview={workspace && (() => setReviewing({pr, workspace}))} trailing={workspace ? null
                  : <Button variant="secondary" size="sm" aria-label={`Clone ${pr.repo}`} onClick={() => setCloning({pr, account})}>Clone</Button>} />;
              })}
            </ul>
          </CollapsibleContent>
        </Collapsible>;
      })}
    </>}
  </section>;
}
