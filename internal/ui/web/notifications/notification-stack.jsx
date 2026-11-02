// The notification cards, stacked in the top right corner below the title bar.
import {useStore} from '../store.js';
import {NotificationCard} from './notification-card.jsx';
import {cards} from './notifications.js';
import './notification-stack.css';

export function NotificationStack() {
  const [state] = useStore();
  const shown = cards(state);
  if (!shown.length) return null;
  return <div class="notification-stack">{shown.map(card => <NotificationCard key={card.key} card={card} />)}</div>;
}
