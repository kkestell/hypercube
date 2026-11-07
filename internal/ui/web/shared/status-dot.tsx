// A small dot whose color, or outline for an ended session, says what it
// means. Its title says the same in words.
const colors = {waiting: 'bg-amber-500', unseen: 'bg-sky-500', idle: 'bg-neutral-600', ended: 'border border-neutral-500'};

export function StatusDot({kind, title}: {kind: keyof typeof colors; title: string}) {
  return <span role="img" aria-label={title} title={title} className={`status-dot status-dot-${kind} inline-block size-2 shrink-0 rounded-full ${colors[kind]}`} />;
}
