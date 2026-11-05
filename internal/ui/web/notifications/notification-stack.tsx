// The notification cards, stacked in the top right corner below the tab bars.
// A top-row tab bar is as tall as the title bar or its 2.25rem of tabs and
// padding, whichever is taller; the stack sits as far below it as it sits from
// the right edge.
import {useStore} from '../store.ts';
import {NotificationCard} from './notification-card.tsx';
import {cards} from './notifications.ts';
import type {Card} from './types.ts';

function Stack({shown}: {shown: Card[]}) {
  return <div className="notification-stack fixed right-4 top-[calc(max(var(--title-bar-height,0px),2.25rem)+1rem)]z-50 flex max-h-3/4 w-sm max-w-full flex-col gap-3 overflow-auto">{shown.map(card => <NotificationCard key={card.key} card={card} />)}</div>;
}

export function NotificationStack() {
  const [state] = useStore();
  const shown = cards(state);
  return shown.length ? <Stack shown={shown} /> : null;
}
