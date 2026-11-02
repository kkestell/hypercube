// A child session's status update in its parent's transcript: the child's
// title, or its ID once it is deleted, the text, and the time.
import {useStore} from '../store.js';
import {sessionOf} from '../sessions/selection.js';
import './status-item.css';

export function StatusItem({item}) {
  const [state] = useStore();
  return (
    <div class="status-item">
      <vscode-icon name="pulse" />
      <span class="status-item-source">{sessionOf(state, item.from)?.session.title || item.from}</span>
      <span>{item.text}</span>
      <time dateTime={new Date(item.at).toISOString()}>{new Date(item.at).toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'})}</time>
    </div>
  );
}
