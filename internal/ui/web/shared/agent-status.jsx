// An agent's indicator: a dot when it exited or waits on the user, a spinner
// while it works, and nothing while it is idle.
import {StatusDot} from './status-dot.jsx';
import './agent-status.css';

export function AgentStatus({agent}) {
  if (agent.activity === 'exited') return <StatusDot kind="exited" title="Exited" />;
  if (agent.attention) return <StatusDot kind="waiting" title="Waiting on you" />;
  if (agent.activity === 'busy') return <vscode-icon class="agent-status-busy" name="loading" spin role="img" title="Busy" />;
  return null;
}
