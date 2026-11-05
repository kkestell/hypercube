// The notification cards, stacked in the top right corner below the tab bars.
import {useStore} from '../store.ts';
import {NotificationCard} from './notification-card.tsx';
import {cards} from './notifications.ts';
import type {Card} from './types.ts';

function Stack({shown}: {shown: Card[]}) {
  return <div className="notification-stack fixed right-4 top-16 z-50 flex max-h-3/4 w-sm max-w-full flex-col gap-3 overflow-auto">{shown.map(card => <NotificationCard key={card.key} card={card} />)}</div>;
}

export function NotificationStack() {
  const [state] = useStore();
  const shown = cards(state);
  return shown.length ? <Stack shown={shown} /> : null;
}
