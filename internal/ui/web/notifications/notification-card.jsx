import {Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle} from '../components/ui/card';
import {Button} from '../components/ui/button';
import {Icon} from '../shared/icon.jsx';
// One notification card: session context and actions above the message.
// A request card offers answers; a reply card shows its opening lines and closes
// itself after 15 seconds, as if closed. An error card has only its message
// and close, and stays until it is closed.
import {useEffect} from 'react';
import {chooseSession} from '../sessions/session-actions.js';
import {useStore} from '../store.js';
import {AttentionForm} from '../transcript/attention-form.jsx';
import {MarkdownText} from '../transcript/markdown.jsx';
import {closeCard} from './notifications.js';

const replyLifetime = 15000;

export function NotificationCard({card}) {
  const [, store] = useStore();

  useEffect(() => {
    if (card.kind !== 'reply') return;
    const timer = setTimeout(() => closeCard(store, card), replyLifetime);
    return () => clearTimeout(timer);
  }, [card.key]);

  return (
    <Card className="notification-card gap-2 py-3" role="alert">
      <CardHeader className="notification-card-header gap-1 px-3">
        <CardTitle className="min-w-0">
          {card.session
            ? <span className="notification-card-session block truncate" title={card.session.title}>{card.session.title}</span>
            : <span className="notification-card-text whitespace-pre-wrap break-words">{card.text}</span>}
        </CardTitle>
        {card.session && <CardDescription className="notification-card-context col-start-1 flex min-w-0 items-center gap-1">
          <span className="truncate" title={card.workspace}>{card.workspace}</span>
          {card.parent && <><Icon name="chevron-right" /><span className="truncate" title={card.parent.title}>{card.parent.title}</span></>}
        </CardDescription>}
        <CardAction className="notification-card-actions flex items-center gap-1">
          {card.session && <Button variant="ghost" size="icon-xs" aria-label="View Session" title="View Session" onClick={() => chooseSession(store, card.session.id)}><Icon name="eye" /></Button>}
          <Button variant="ghost" size="icon-xs" aria-label="Close" title="Close" onClick={() => closeCard(store, card)}><Icon name="close" /></Button>
        </CardAction>
      </CardHeader>
      {card.session && (
        <CardContent className="notification-card-details space-y-2 px-3">
          {card.kind === 'request' ? (
            <>
              <div className="notification-card-text whitespace-pre-wrap break-words">{card.item.text ?? ''}</div>
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
