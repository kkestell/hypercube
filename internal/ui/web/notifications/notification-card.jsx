import {Card, CardContent, CardHeader} from '../components/ui/card';
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
const icons = {error: 'error', request: 'warning', reply: 'info'};

export function NotificationCard({card}) {
  const [, store] = useStore();

  useEffect(() => {
    if (card.kind !== 'reply') return;
    const timer = setTimeout(() => closeCard(store, card), replyLifetime);
    return () => clearTimeout(timer);
  }, [card.key]);

  const icon = icons[card.kind];
  return (
    <Card className="notification-card" role="alert">
      <CardHeader className="notification-card-header flex flex-row items-start gap-2">
        <Icon name={icon} className={'notification-card-' + icon} />
        {!card.session && <div className="notification-card-text whitespace-pre-wrap break-words">{card.text}</div>}
        {card.session && (
          <div className="notification-card-source min-w-0 flex-1">
            <span className="notification-card-session block truncate font-medium" title={card.session.title}>{card.session.title}</span>
            <div className="notification-card-context flex items-center gap-1 truncate text-xs text-muted-foreground">
              <span title={card.workspace}>{card.workspace}</span>
              {card.parent && <><Icon name="chevron-right" /><span title={card.parent.title}>{card.parent.title}</span></>}
            </div>
          </div>
        )}
        <div className="notification-card-actions ml-auto flex shrink-0 items-center gap-1">
          {card.session && <Button variant="ghost" size="icon" aria-label="View Session" title="View Session" onClick={() => chooseSession(store, card.session.id)}><Icon name="eye" /></Button>}
          <Button variant="ghost" size="icon" aria-label="Close" title="Close" onClick={() => closeCard(store, card)}><Icon name="close" /></Button>
        </div>
      </CardHeader>
      {card.session && (
        <CardContent className="notification-card-details space-y-3">
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
