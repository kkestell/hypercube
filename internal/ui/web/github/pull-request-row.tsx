// A pull request's row: its icon, title link, whose move it is and why, and
// optionally where it lives and the actions that send work on it to a session.
import type {ReactNode} from 'react';
import {Button} from '../components/ui/button';
import {actionLabels, actionsFor, type PullRequestAction} from './instructions.ts';
import {PullRequestIcon} from './pull-request-icon.tsx';
import type {Turn} from './turns.ts';
import type {PullRequest} from './types.ts';

export type Send = (pr: PullRequest, action: PullRequestAction) => void;

interface RowProps {
  pr: PullRequest;
  turn: Turn;
  account: string;
  send?: Send;
  place?: ReactNode;
}

export function PullRequestRow({pr, turn, account, send, place}: RowProps) {
  const own = pr.author === account;
  return <li className={`flex items-center gap-2 px-3 text-sm ${turn.yours ? '' : 'opacity-60'}`}>
    <PullRequestIcon pr={pr} />
    <div className="min-w-0 flex-1">
      <a href={pr.url} target="_blank" rel="noopener noreferrer" className="block truncate underline underline-offset-2">#{pr.number} {pr.title}</a>
      <p className="flex flex-wrap gap-x-1.5 text-muted-foreground">
        <span>{turn.reason}</span>
        {!own && pr.author && <span>From {pr.author}.</span>}
      </p>
      {place && <p className="text-muted-foreground">{place}</p>}
    </div>
    {send && own && actionsFor(pr).map(action => <Button key={action} variant="secondary" size="sm" onClick={() => send(pr, action)}>{actionLabels[action]}</Button>)}
  </li>;
}
