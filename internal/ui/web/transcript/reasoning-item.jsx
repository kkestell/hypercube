// Thinking: the model's reasoning text, shown in full.
import './reasoning-item.css';

export function ReasoningItem({item}) {
  return item.text ? <div class="reasoning-item">{item.text}</div> : null;
}
