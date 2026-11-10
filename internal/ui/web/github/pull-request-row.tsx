// A pull request's row: the icon of what it needs, its title with a link to
// GitHub, whose move it is and why, and optionally where it lives, a Review
// button when the account's review was requested, and a Chat button.
import type {ReactNode} from 'react';
import {Button} from '../components/ui/button';
import {canReview} from './review.ts';
import {Icon, type IconName} from '../shared/icon.tsx';
import {tones} from '../shared/tones.ts';
import type {Turn} from './turns.ts';
import type {PullRequest} from './types.ts';

const needColors: Partial<Record<IconName, string>> = {
  error: tones.error,
  warning: tones.warning,
  edit: tones.warning,
  refresh: tones.warning,
  eye: tones.info,
  'comment-discussion': tones.info,
  'git-merge': tones.success,
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
  // onChat opens a new session about the pull request; without it the Chat
  // button is disabled because that is starting.
  onChat?: () => void;
}

export function PullRequestRow({pr, turn, account, onReview, hideReason, place, onChat}: RowProps) {
  const own = pr.author === account;
  return <li className={`group/row flex items-start gap-2 text-sm ${turn.yours ? '' : 'opacity-60'}`}>
    <span className="flex h-5 items-center"><Icon name={turn.icon} className={needColors[turn.icon] ?? tones.muted} title={turn.reason} role="img" /></span>
    <div className="min-w-0 flex-1">
      <p><a href={pr.url} target="_blank" rel="noopener noreferrer" title="Open on GitHub" className="flex items-center gap-1.5 hover:underline">
        <span className="truncate">#{pr.number} {pr.title}</span>
        <span className="invisible shrink-0 text-muted-foreground opacity-0 transition-[opacity,visibility] group-hover/row:visible group-hover/row:opacity-100 group-focus-within/row:visible group-focus-within/row:opacity-100">
          <Icon name="link-external" />
        </span>
      </a></p>
      {(!hideReason || (!own && pr.author)) && <p className="flex flex-wrap gap-x-1.5 text-muted-foreground">
        {!hideReason && <span>{turn.reason}</span>}
        {!own && pr.author && <span>From {pr.author}.</span>}
      </p>}
      {place && <p className="flex min-w-0 items-center gap-1.5 text-muted-foreground">{place}</p>}
    </div>
    {!own && canReview(pr) && <Button variant="secondary" size="sm" className="self-center" disabled={!onReview} onClick={onReview}>Review</Button>}
    <Button variant="secondary" size="sm" className="self-center" disabled={!onChat} onClick={onChat}>Chat</Button>
  </li>;
}
