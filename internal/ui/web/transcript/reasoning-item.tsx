// Thinking: the model's reasoning text, shown in full.
import {Markdown} from './markdown.tsx';
import type {ReasoningItem as Reasoning} from './types.ts';

export function ReasoningItem({item}: {item: Reasoning}) {
  return item.text ? <div className="reasoning-item"><Markdown text={item.text} style="chat" muted /></div> : null;
}
