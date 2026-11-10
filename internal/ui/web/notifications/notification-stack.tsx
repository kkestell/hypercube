// The notification cards, stacked in the top right corner of the window.
import {useStore} from '../store.ts';
import {NotificationCard} from './notification-card.tsx';
import {cards} from './notifications.ts';

export function NotificationStack() {
  const [state] = useStore();
  const shown = cards(state);
  if (!shown.length) return null;
  return <div className="pointer-events-none [app-region:no-drag] fixed right-1 top-1 z-50 flex max-h-3/4 max-w-full flex-col gap-3 overflow-auto p-3">
    {shown.map(card => <NotificationCard key={card.key} card={card} />)}
  </div>;
}
