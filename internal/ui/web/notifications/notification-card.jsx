// One notification card, as VS Code's toasts: an icon, the details, View
// Session and close, and the source below. A request card offers the
// request's answers; a reply card shows the reply's opening lines and closes
// itself after 15 seconds without marking the reply seen. An error card has
// only its message and close, and stays until it is closed.
import {useEffect} from 'preact/hooks';
import {chooseSession} from '../sessions/session-actions.js';
import {useStore} from '../store.js';
import {AttentionForm} from '../transcript/attention-form.jsx';
import {closeCard, dismiss} from './notifications.js';
import './notification-card.css';

const replyLifetime = 15000;
const icons = {error: 'error', request: 'warning', reply: 'info'};

export function NotificationCard({card}) {
  const [, store] = useStore();

  useEffect(() => {
    if (card.kind !== 'reply') return;
    const timer = setTimeout(() => store.update(s => dismiss(s, card.key)), replyLifetime);
    return () => clearTimeout(timer);
  }, [card.key]);

  const icon = icons[card.kind];
  return (
    <div class="notification-card" role="alert">
      <div class="notification-card-main">
        <vscode-icon name={icon} class={'notification-card-' + icon} />
        <div class="notification-card-details">
          {card.kind === 'request' ? (
            <>
              <div class="notification-card-text">{card.item.text ?? ''}</div>
              <AttentionForm agent={card.agent} item={card.item} />
            </>
          ) : <div class={card.kind === 'reply' ? 'notification-card-text notification-card-reply' : 'notification-card-text'}>{card.text}</div>}
        </div>
        <div class="notification-card-actions">
          {card.session && <vscode-toolbar-button icon="eye" label="View Session" title="View Session" onClick={() => chooseSession(store, card.session.id)} />}
          <vscode-toolbar-button icon="close" label="Close" title="Close" onClick={() => closeCard(store, card)} />
        </div>
      </div>
      {card.session && <div class="notification-card-source">{[card.workspace, card.session.title, card.worker].filter(Boolean).join(' — ')}</div>}
    </div>
  );
}
