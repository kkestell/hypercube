// Thinking: the model's reasoning text, shown in full.

export function ReasoningItem({item}) {
  return item.text ? <div className="reasoning-item whitespace-pre-wrap break-words text-sm text-muted-foreground">{item.text}</div> : null;
}
