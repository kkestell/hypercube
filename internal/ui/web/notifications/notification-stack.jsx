// The notification cards, stacked in the top right corner below the tab bars.
import {useRef} from 'preact/hooks';
import {useOverlay} from '../shared/overlays.js';
import {useStore} from '../store.js';
import {NotificationCard} from './notification-card.jsx';
import {cards} from './notifications.js';
import './notification-stack.css';

function Stack({shown}) {
  const ref = useRef(null);
  useOverlay(ref);
  return <div ref={ref} class="notification-stack">{shown.map(card => <NotificationCard key={card.key} card={card} />)}</div>;
}

export function NotificationStack() {
  const [state] = useStore();
  const shown = cards(state);
  return shown.length ? <Stack shown={shown} /> : null;
}
