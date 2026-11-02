// An session's indicator: a dot when it exited or waits on the user, a spinner
// while it works, and nothing while it is idle.
import {StatusDot} from './status-dot.jsx';
import './agent-status.css';

export function AgentStatus({session}) {
  if (session.ended) return null;
  if (session.activity === 'exited') return <StatusDot kind="exited" title="Exited" />;
  if (session.attention) return <StatusDot kind="waiting" title="Waiting on you" />;
  if (session.activity === 'busy') return <vscode-icon class="agent-status-busy" name="loading" spin role="img" title="Busy" />;
  return null;
}
