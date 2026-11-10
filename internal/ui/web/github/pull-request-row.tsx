// A pull request's row: the icon of what it needs, its title with a link to
// GitHub, whose move it is and why, and optionally where it lives, a Review
// button when the account's review was requested, and a trailing control.
import type {ReactNode} from 'react';
import {Button} from '../components/ui/button';
import {canReview} from './review.ts';
import {Icon, type IconName} from '../shared/icon.tsx';
import type {Turn} from './turns.ts';
import type {PullRequest} from './types.ts';

// needColors color what a pull request needs: red for a failed check, amber
// for work on the branch, blue for people waiting on an answer, green for a
// merge, and grey for waiting.
const needColors: Partial<Record<IconName, string>> = {
  error: 'text-destructive',
  warning: 'text-amber-600 dark:text-amber-500',
  edit: 'text-amber-600 dark:text-amber-500',
  refresh: 'text-amber-600 dark:text-amber-500',
  eye: 'text-sky-600 dark:text-sky-500',
  'comment-discussion': 'text-sky-600 dark:text-sky-500',
  'git-merge': 'text-green-600 dark:text-green-500',
};

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
  return <li className={`group/row flex items-start gap-2 text-sm ${turn.yours ? '' : 'opacity-60'}`}>
    <span className="flex h-5 items-center"><Icon name={turn.icon} className={needColors[turn.icon] ?? 'text-muted-foreground'} title={turn.reason} role="img" /></span>
    <div className="min-w-0 flex-1">
      <p className="flex items-center gap-1.5">
        <span className="truncate">#{pr.number} {pr.title}</span>
        <a href={pr.url} target="_blank" rel="noopener noreferrer" aria-label={`Open #${pr.number} on GitHub`} title="Open on GitHub" className="shrink-0 text-muted-foreground opacity-0 focus-visible:opacity-100 group-hover/row:opacity-100 hover:text-foreground">
          <Icon name="link-external" />
        </a>
      </p>
      {(!hideReason || (!own && pr.author)) && <p className="flex flex-wrap gap-x-1.5 text-muted-foreground">
        {!hideReason && <span>{turn.reason}</span>}
        {!own && pr.author && <span>From {pr.author}.</span>}
      </p>}
      {place && <p className="text-muted-foreground">{place}</p>}
    </div>
    {!own && canReview(pr) && <Button variant="secondary" size="sm" className="self-center" disabled={!onReview} onClick={onReview}>Review</Button>}
    {trailing}
  </li>;
}
