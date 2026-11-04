// The notification cards, stacked in the top right corner below the tab bars.
import {useOverlay} from '../shared/overlays.js';
import {useStore} from '../store.js';
import {NotificationCard} from './notification-card.jsx';
import {cards} from './notifications.js';

function Stack({shown}) {
  const ref = useOverlay();
  return <div ref={ref} className="notification-stack fixed right-4 top-16 z-50 flex max-h-3/4 w-sm max-w-full flex-col gap-3 overflow-auto">{shown.map(card => <NotificationCard key={card.key} card={card} />)}</div>;
}

export function NotificationStack() {
  const [state] = useStore();
  const shown = cards(state);
  return shown.length ? <Stack shown={shown} /> : null;
}
