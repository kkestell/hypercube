import type {Session} from './types.ts';
import {StatusDot} from '../shared/status-dot.tsx';
import {BusyDot} from '../shared/busy-dot.tsx';

// statusOf returns a session's status dot, hollow when outline is set.
export const statusOf = (session: Session, selected: boolean, waiting: boolean, outline: boolean) => {
  if (session.ended) return <StatusDot kind="ended" title="Ended" />;
  if (waiting) return <StatusDot kind="waiting" title="Waiting on you" outline={outline} />;
  if (session.unseen && !selected) return <StatusDot kind="unseen" title="New reply" outline={outline} />;
  if (session.activity === 'busy') return <BusyDot title="Working" outline={outline} />;
  return <StatusDot kind="idle" title="Idle" outline={outline} />;
};
