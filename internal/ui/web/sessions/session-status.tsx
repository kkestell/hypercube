// A session's status dot: ended, waiting on the user, working, with a new
// reply, or idle, the first that applies. A child's dot is hollow.
import {BusyDot} from '../shared/busy-dot.tsx';
import {StatusDot} from '../shared/status-dot.tsx';
import type {Session} from './types.ts';

export function SessionStatus({session, selected, waiting, outline}: {session: Session; selected: boolean; waiting: boolean; outline: boolean}) {
  if (session.ended) return <StatusDot kind="ended" title="Ended" />;
  if (waiting) return <StatusDot kind="waiting" title="Waiting on you" outline={outline} />;
  if (session.activity === 'busy') return <BusyDot title="Working" outline={outline} />;
  if (session.unseen && !selected) return <StatusDot kind="unseen" title="New reply" outline={outline} />;
  return <StatusDot kind="idle" title="Idle" outline={outline} />;
}
