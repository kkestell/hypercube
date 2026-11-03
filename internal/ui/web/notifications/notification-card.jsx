// One notification card: session context and actions above the message.
// A request card offers answers; a reply card shows its opening lines and closes
// itself after 15 seconds, as if closed. An error card has only its message
// and close, and stays until it is closed.
import {useEffect} from 'preact/hooks';
import {chooseSession} from '../sessions/session-actions.js';
import {useStore} from '../store.js';
import {AttentionForm} from '../transcript/attention-form.jsx';
import {MarkdownText} from '../transcript/markdown.jsx';
import {closeCard} from './notifications.js';
import './notification-card.css';

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
    <div class="notification-card" role="alert">
      <div class="notification-card-header">
        <vscode-icon name={icon} class={'notification-card-' + icon} />
        {!card.session && <div class="notification-card-text">{card.text}</div>}
        {card.session && (
          <div class="notification-card-source">
            <span class="notification-card-session" title={card.session.title}>{card.session.title}</span>
            <div class="notification-card-context">
              <span title={card.workspace}>{card.workspace}</span>
              {card.parent && <><vscode-icon name="chevron-right" /><span title={card.parent.title}>{card.parent.title}</span></>}
            </div>
          </div>
        )}
        <div class="notification-card-actions">
          {card.session && <vscode-toolbar-button icon="eye" label="View Session" title="View Session" onClick={() => chooseSession(store, card.session.id)} />}
          <vscode-toolbar-button icon="close" label="Close" title="Close" onClick={() => closeCard(store, card)} />
        </div>
      </div>
      {card.session && (
        <div class="notification-card-details">
          {card.kind === 'request' ? (
            <>
              <div class="notification-card-text">{card.item.text ?? ''}</div>
              <AttentionForm session={card.session.id} item={card.item} />
            </>
          ) : (
            <div class="notification-card-text notification-card-reply">
              <MarkdownText text={card.text} />
            </div>
          )}
        </div>
      )}
    </div>
  );
}
