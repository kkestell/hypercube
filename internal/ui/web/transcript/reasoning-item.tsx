// Thinking: the model's reasoning text, shown in full.
import {Markdown} from './markdown.tsx';
import type {ReasoningItem as Reasoning} from './types.ts';

export function ReasoningItem({item}: {item: Reasoning}) {
  return item.text ? <div className="reasoning-item text-muted-foreground [&_.markdown]:text-muted-foreground"><Markdown text={item.text} style="chat" /></div> : null;
}
