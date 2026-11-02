// A small colored dot whose title says what it means.
import './status-dot.css';

// kind is 'waiting', 'exited', or 'unseen'.
export const StatusDot = ({kind, title}) => <span role="img" class={`status-dot status-dot-${kind}`} title={title} />;
