// Pull requests in collapsible groups by the kind of work, in turnGroups
// order, saying so when nothing needs the account. The caller draws each row;
// implied says the group's heading already gives the row's reason.
import type {ReactNode} from 'react';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {Button} from '../components/ui/button';
import {Chevron} from '../shared/icon.tsx';
import {Message} from '../shared/message.tsx';
import {slide} from '../shared/motion.ts';
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
    {!groups.some(group => group.id !== 'waiting') && <Message className="mb-4">Nothing needs you.</Message>}
    {groups.map(group => {
      const open = !workbench.collapsedPullRequestGroups[group.id];
      return <Collapsible key={group.id} open={open} onOpenChange={value => store.update(s => expandPullRequestGroup(s, group.id, value))} className="mb-6">
        <CollapsibleTrigger asChild>
          <Button type="button" variant="ghost" size="sm" className="mb-3 w-full justify-start">
            <span>{group.title}</span>
            <span className="text-muted-foreground">{group.turns.length}</span>
            <Chevron open={open} />
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent className={slide}>
          <ul aria-label={group.title} className="space-y-4">
            {group.turns.map(turn => row(turn, turn.turn.reason === group.implied))}
          </ul>
        </CollapsibleContent>
      </Collapsible>;
    })}
  </>;
}
