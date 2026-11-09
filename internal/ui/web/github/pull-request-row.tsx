// A pull request's row: its icon, title link, whose move it is and why, and
// optionally where it lives, a Review button when the account's review was
// requested, and a trailing control.
import type {ReactNode} from 'react';
import {Button} from '../components/ui/button';
import {canReview} from './review.ts';
import {PullRequestIcon} from './pull-request-icon.tsx';
import type {Turn} from './turns.ts';
import type {PullRequest} from './types.ts';

interface RowProps {
  pr: PullRequest;
  turn: Turn;
  account: string;
  // onReview starts the review; without it the Review button is disabled
  // because no workspace holds the repository or the review is starting.
  onReview?: () => void;
  // hideReason leaves the reason to a heading that already says it.
  hideReason?: boolean;
  place?: ReactNode;
  trailing?: ReactNode;
}

export function PullRequestRow({pr, turn, account, onReview, hideReason, place, trailing}: RowProps) {
  const own = pr.author === account;
  return <li className={`flex items-center gap-2 px-3 text-sm ${turn.yours ? '' : 'opacity-60'}`}>
    <PullRequestIcon pr={pr} />
    <div className="min-w-0 flex-1">
      <a href={pr.url} target="_blank" rel="noopener noreferrer" className="block truncate underline underline-offset-2">#{pr.number} {pr.title}</a>
      {(!hideReason || (!own && pr.author)) && <p className="flex flex-wrap gap-x-1.5 text-muted-foreground">
        {!hideReason && <span>{turn.reason}</span>}
        {!own && pr.author && <span>From {pr.author}.</span>}
      </p>}
      {place && <p className="text-muted-foreground">{place}</p>}
    </div>
    {!own && canReview(pr) && <Button variant="secondary" size="sm" disabled={!onReview} onClick={onReview}>Review</Button>}
    {trailing}
  </li>;
}
