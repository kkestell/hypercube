// Pull requests in collapsible groups by the kind of work, in turnGroups
// order, saying so when nothing needs the account. The caller draws each row;
// implied says the group's heading already gives the row's reason.
import type {ReactNode} from 'react';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {Chevron} from '../shared/icon.tsx';
import {expandPullRequestGroup} from '../app/workbench.ts';
import {useStore} from '../store.ts';
import {turnGroups, type Turn} from './turns.ts';
import type {PullRequest} from './types.ts';

export interface AccountTurn {
  account: string;
  pr: PullRequest;
  turn: Turn;
}

export function PullRequestGroups({turns, row}: {turns: AccountTurn[]; row: (turn: AccountTurn, implied: boolean) => ReactNode}) {
  const [{workbench}, store] = useStore();
  const groups = turnGroups.map(group => ({...group, turns: turns.filter(({turn}) => turn.group === group.id)})).filter(group => group.turns.length);
  return <>
    {!groups.some(group => group.id !== 'waiting') && <p className="mb-4 text-sm text-muted-foreground">Nothing needs you.</p>}
    {groups.map(group => {
      const open = !workbench.collapsedPullRequestGroups[group.id];
      return <Collapsible key={group.id} open={open} onOpenChange={value => store.update(s => expandPullRequestGroup(s, group.id, value))} className="mb-6">
        <CollapsibleTrigger asChild>
          <button type="button" className="mb-3 flex h-8 w-full items-center gap-1.5 border-b text-sm font-medium outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50">
            <span>{group.title}</span>
            <span className="text-muted-foreground">{group.turns.length}</span>
            <Chevron open={open} />
          </button>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <ul aria-label={group.title} className="space-y-4">
            {group.turns.map(turn => row(turn, turn.turn.reason === group.implied))}
          </ul>
        </CollapsibleContent>
      </Collapsible>;
    })}
  </>;
}
