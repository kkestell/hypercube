// The notification cards, stacked in the top right corner of the window.
import {useStore} from '../store.ts';
import {NotificationCard} from './notification-card.tsx';
import {cards} from './notifications.ts';
import type {Card} from './types.ts';

function Stack({shown}: {shown: Card[]}) {
  return <div className="notification-stack pointer-events-none [app-region:no-drag] fixed right-1 top-1 z-50 flex max-h-3/4 w-[25.5rem] max-w-full flex-col gap-3 overflow-auto p-3">{shown.map(card => <NotificationCard key={card.key} card={card} />)}</div>;
}

export function NotificationStack() {
  const [state] = useStore();
  const shown = cards(state);
  return shown.length ? <Stack shown={shown} /> : null;
}
