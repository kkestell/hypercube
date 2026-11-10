// A pull request's row, drawn like a session's row in the sidebar: the icon of
// what it needs, which names it, and its title. Its hover text gives its
// reference and who opened it when someone else did. On hover or keyboard
// focus, Review, when the account's review was requested, and Chat show over
// the title's end, which fades out before them. The row's context
// menu, opened by right-clicking it or with the menu key while it has focus,
// offers Review when the account's review was requested, Chat, Open on
// GitHub, and Copy URL.
import {useCallback, useState, type KeyboardEvent} from 'react';
import {ContextMenu, type ContextMenuItem} from '../shared/context-menu.tsx';
import {FadingText} from '../shared/fading-text.tsx';
import {Icon} from '../shared/icon.tsx';
import {IconButton} from '../shared/icon-button.tsx';
import {canReview} from './review.ts';
import type {Turn} from './turns.ts';
import type {PullRequest} from './types.ts';

interface RowProps {
  pr: PullRequest;
  turn: Turn;
  account: string;
  // onReview starts the review; without it Review is disabled because no
  // workspace holds the repository or the review is starting.
  onReview?: () => void;
  // showRepo names the repository in the reference, for lists that span
  // repositories.
  showRepo?: boolean;
  // onChat opens a new session about the pull request; without it Chat is
  // disabled because that is starting.
  onChat?: () => void;
}

export function PullRequestRow({pr, turn, account, onReview, showRepo, onChat}: RowProps) {
  const own = pr.author === account;
  const reviewable = !own && canReview(pr);
  const [menu, setMenu] = useState<{x: number; y: number} | null>(null);
  const [open, setOpen] = useState(false);
  const close = useCallback(() => setOpen(false), []);
  const show = (x: number, y: number) => {
    setMenu({x, y});
    setOpen(true);
  };
  const menuKey = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key !== 'ContextMenu' && !(event.shiftKey && event.key === 'F10')) return;
    event.preventDefault();
    const rect = event.currentTarget.getBoundingClientRect();
    show(rect.left, rect.bottom);
  };
  const items: ContextMenuItem[][] = [[
    ...(reviewable ? [{label: 'Review', icon: 'eye' as const, disabled: !onReview, run: () => onReview?.()}] : []),
    {label: 'Chat', icon: 'comment-discussion', disabled: !onChat, run: () => onChat?.()},
  ], [
    {label: 'Open on GitHub', icon: 'link-external', run: () => { window.open(pr.url, '_blank', 'noopener'); }},
    {label: 'Copy URL', icon: 'copy', run: () => navigator.clipboard.writeText(pr.url)},
  ]];
  const reference = `${showRepo ? pr.repo : ''}#${pr.number}${!own && pr.author ? ` from ${pr.author}` : ''}.`;
  return <li tabIndex={0} aria-haspopup="menu" title={`${pr.title}\n${reference}`}
    className={`group/row relative flex h-8 cursor-default items-center gap-2 rounded-md px-2 text-sm outline-none hover:bg-accent focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:hover:bg-accent/50 ${reviewable ? '[--cover:3.75rem]' : '[--cover:2.25rem]'}`}
    onKeyDown={menuKey} onContextMenu={event => { event.preventDefault(); show(event.clientX, event.clientY); }}>
    <span className={`inline-flex size-4 shrink-0 items-center justify-center text-muted-foreground ${turn.yours ? '' : 'opacity-60'}`}>
      <Icon name={turn.icon} className="size-3.5" title={turn.reason} role="img" />
    </span>
    <FadingText text={pr.title} className={`-mr-2 flex-1 ${turn.yours ? '' : 'opacity-60'}`} />
    <span className="invisible absolute top-1/2 right-1 flex -translate-y-1/2 opacity-0 transition-[opacity,visibility] group-hover/row:visible group-hover/row:opacity-100 group-focus-visible/row:visible group-focus-visible/row:opacity-100 group-has-[:focus-visible]/row:visible group-has-[:focus-visible]/row:opacity-100">
      {reviewable && <IconButton icon="eye" label="Review" disabled={!onReview} onClick={onReview} />}
      <IconButton icon="comment-discussion" label="Chat" disabled={!onChat} onClick={onChat} />
    </span>
    {menu && <ContextMenu open={open} x={menu.x} y={menu.y} items={items} onClose={close} />}
  </li>;
}
