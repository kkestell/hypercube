// Thinking: its latest six lines while it streams in, or all of it once the
// user expands it.
import {Disclosure} from './disclosure.jsx';
import './reasoning-item.css';

export function ReasoningItem({item, expanded, setOpen}) {
  return (
    <div class="reasoning-item">
      <Disclosure open={expanded} onToggle={open => setOpen(item.id, open)} title={expanded ? 'Show latest six lines' : 'Show full thinking'} summary="Thinking">
        {expanded && <div class="reasoning-item-text">{item.text ?? ''}</div>}
      </Disclosure>
      {!expanded && (
        <div class="reasoning-item-text reasoning-item-preview">
          <div>{item.text ?? ''}</div>
        </div>
      )}
    </div>
  );
}
