import {StatusBadge} from './status-badge.jsx';
import {Icon} from './icon.jsx';

export function AgentStatus({session}) {
  if (session.ended) return null;
  if (session.activity === 'exited') return <StatusBadge kind="exited" title="Exited" />;
  if (session.attention) return <StatusBadge kind="waiting" title="Waiting on you" />;
  if (session.activity === 'busy') return <Icon name="loading" spin role="img" title="Busy" />;
  return null;
}
