// Thinking: its latest six lines while it streams in, or all of it once the
// user expands it.
import {memo} from 'preact/compat';
import './reasoning-item.css';

export const ReasoningItem = memo(({item, expanded, setOpen}) => (
  <div class="reasoning-item">
    <button
      type="button"
      class="reasoning-item-toggle"
      aria-expanded={expanded}
      title={expanded ? 'Show latest six lines' : 'Show full thinking'}
      onClick={() => setOpen(item.id, !expanded)}
    >
      Thinking
      <vscode-icon name="chevron-right" class="reasoning-item-chevron" />
    </button>
    <div class={expanded ? 'reasoning-item-text' : 'reasoning-item-text reasoning-item-preview'}>
      <div>{item.text ?? ''}</div>
    </div>
  </div>
));
