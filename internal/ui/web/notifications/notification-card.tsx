import {Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle} from '../components/ui/card';
import {Button} from '../components/ui/button';
import {Icon} from '../shared/icon.tsx';
// One notification card: session context and actions above the message.
// A request card offers answers; a reply card shows its opening lines and closes
// itself after 15 seconds, as if closed. An error card has only its message
// and close, and stays until it is closed.
import {useEffect, useRef} from 'react';
import {chooseSession} from '../sessions/session-actions.ts';
import {useStore} from '../store.ts';
import {AttentionForm} from '../transcript/attention-form.tsx';
import {TaskOffer} from '../transcript/task-offer.tsx';
import {Markdown, MarkdownText} from '../transcript/markdown.tsx';
import {closeCard} from './notifications.ts';
import type {Card as CardData} from './types.ts';

const replyLifetime = 15000;

export function NotificationCard({card}: {card: CardData}) {
  const [, store] = useStore();
  const element = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const node = element.current;
    if (card.kind !== 'reply' || !node) return;
    let remaining = replyLifetime;
    let started = performance.now();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let hovered = false;
    let focused = node.contains(document.activeElement);
    const pause = () => {
      if (timer === undefined) return;
      clearTimeout(timer);
      timer = undefined;
      remaining -= performance.now() - started;
    };
    const resume = () => {
      if (hovered || focused || timer !== undefined) return;
      started = performance.now();
      timer = setTimeout(() => closeCard(store, card), remaining);
    };
    const enter = () => { hovered = true; pause(); };
    const leave = () => { hovered = false; resume(); };
    const focus = () => { focused = true; pause(); };
    const blur = (event: FocusEvent) => {
      focused = event.relatedTarget instanceof Node && node.contains(event.relatedTarget);
      resume();
    };
    node.addEventListener('pointerenter', enter);
    node.addEventListener('pointerleave', leave);
    node.addEventListener('focusin', focus);
    node.addEventListener('focusout', blur);
    resume();
    return () => {
      pause();
      node.removeEventListener('pointerenter', enter);
      node.removeEventListener('pointerleave', leave);
      node.removeEventListener('focusin', focus);
      node.removeEventListener('focusout', blur);
    };
  }, [card.key]);

  return (
    <Card ref={element} className="notification-card pointer-events-auto gap-2 py-3 shadow-[0_4px_12px_-2px_rgb(0_0_0/0.12)]" role="alert">
      <CardHeader className="notification-card-header gap-1 px-3">
        <CardTitle className={`min-w-0 ${card.kind === 'error' ? 'text-sm font-normal leading-5' : ''}`}>
          {card.kind === 'error'
            ? <span className="notification-card-text whitespace-pre-wrap break-words">{card.text}</span>
            : <span className="notification-card-session block truncate" title={card.session.title}>{card.session.title}</span>}
        </CardTitle>
        {card.kind !== 'error' && <CardDescription className="notification-card-context col-start-1 flex min-w-0 items-center gap-1">
          <span className="truncate" title={card.workspace}>{card.workspace}</span>
          {card.parent && <><Icon name="chevron-right" /><span className="truncate" title={card.parent.title}>{card.parent.title}</span></>}
        </CardDescription>}
        <CardAction className="notification-card-actions flex items-center gap-1">
          {card.kind !== 'error' && <Button variant="ghost" size="icon-xs" aria-label="View Session" title="View Session" onClick={() => chooseSession(store, card.session.id)}><Icon name="eye" /></Button>}
          <Button variant="ghost" size="icon-xs" aria-label="Close" title="Close" onClick={() => closeCard(store, card)}><Icon name="close" /></Button>
        </CardAction>
      </CardHeader>
      {card.kind !== 'error' && (
        <CardContent className="notification-card-details space-y-2 px-3">
          {card.kind === 'request' ? (
            card.item.offer ? <TaskOffer session={card.session.id} item={card.item} /> : <>
              <div className="notification-card-text"><Markdown text={card.item.text ?? ''} style="chat" /></div>
              <AttentionForm session={card.session.id} item={card.item} />
            </>
          ) : (
            <div className="notification-card-text notification-card-reply line-clamp-4 break-words text-sm">
              <MarkdownText text={card.text} />
            </div>
          )}
        </CardContent>
      )}
    </Card>
  );
}
