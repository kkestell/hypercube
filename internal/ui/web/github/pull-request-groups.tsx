// Pull requests in collapsible groups by the kind of work, in turnGroups
// order, saying so when nothing needs the account. The caller draws each row.
import type {ReactNode} from 'react';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {Chevron} from '../shared/icon.tsx';
import {Message} from '../shared/message.tsx';
import {expandPullRequestGroup} from '../app/workbench.ts';
import {useStore} from '../store.ts';
import {turnGroups, type Turn} from './turns.ts';
import type {PullRequest} from './types.ts';

const shown = 'transition-[opacity,visibility] group-hover/heading:visible group-hover/heading:opacity-100 group-focus-visible/heading:visible group-focus-visible/heading:opacity-100';

export interface AccountTurn {
  account: string;
  pr: PullRequest;
  turn: Turn;
}

export function PullRequestGroups({turns, row}: {turns: AccountTurn[]; row: (turn: AccountTurn) => ReactNode}) {
  const [{workbench}, store] = useStore();
  const groups = turnGroups.map(group => ({...group, turns: turns.filter(({turn}) => turn.group === group.id)})).filter(group => group.turns.length);
  return <>
    {!groups.some(group => group.id !== 'waiting') && <Message className="mb-2 px-2">Nothing needs you.</Message>}
    {groups.map(group => {
      const open = !workbench.collapsedPullRequestGroups[group.id];
      return <Collapsible key={group.id} open={open} onOpenChange={value => store.update(s => expandPullRequestGroup(s, group.id, value))} asChild>
        <section className={open ? 'mb-2 last:mb-0' : ''}>
          <CollapsibleTrigger asChild>
            <button type="button" className="group/heading flex h-7 w-full items-center gap-2 rounded-md pr-1 pl-2 text-xs uppercase text-muted-foreground outline-none hover:bg-accent focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:hover:bg-accent/50">
              <span className="truncate">{group.title}</span>
              <span className={`invisible flex shrink-0 opacity-0 ${shown}`}><Chevron open={open} /></span>
              <span className="ml-auto pr-1">{group.turns.length}</span>
            </button>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <ul aria-label={group.title} className="flex flex-col">
              {group.turns.map(row)}
            </ul>
          </CollapsibleContent>
        </section>
      </Collapsible>;
    })}
  </>;
}
