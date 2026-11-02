// Thinking: its latest six lines while it streams in, or all of it once the
// user expands it.
import {memo} from 'preact/compat';
import './reasoning-item.css';

export const ReasoningItem = memo(({item, expanded, setOpen}) => (
  <div class="reasoning-item">
    <details class="reasoning-item-disclosure" open={expanded} onToggle={e => setOpen(item.id, e.currentTarget.open)}>
      <summary class="reasoning-item-toggle" title={expanded ? 'Show latest six lines' : 'Show full thinking'}>
        Thinking
        <vscode-icon name="chevron-right" class="reasoning-item-chevron" />
      </summary>
      {expanded && <div class="reasoning-item-text">{item.text ?? ''}</div>}
    </details>
    {!expanded && (
      <div class="reasoning-item-text reasoning-item-preview">
        <div>{item.text ?? ''}</div>
      </div>
    )}
  </div>
));
