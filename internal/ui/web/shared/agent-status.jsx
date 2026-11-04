import {StatusIcon} from './status-icon.jsx';
import {Icon} from './icon.jsx';

export function AgentStatus({session}) {
  if (session.ended) return null;
  if (session.activity === 'exited') return <StatusIcon kind="exited" title="Exited" />;
  if (session.attention) return <StatusIcon kind="waiting" title="Waiting on you" />;
  if (session.activity === 'busy') return <Icon name="loading" spin role="img" title="Busy" />;
  return null;
}
