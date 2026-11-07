// A small dot whose color says what it means. Its title says the same in
// words. An outlined dot is hollow; an ended session's dot is always hollow.
const fills = {waiting: 'bg-amber-500', unseen: 'bg-sky-500', idle: 'bg-neutral-600', ended: 'border border-neutral-500'};
const outlines = {waiting: 'border border-amber-500', unseen: 'border border-sky-500', idle: 'border border-neutral-500', ended: 'border border-neutral-500'};

export function StatusDot({kind, title, outline = false}: {kind: keyof typeof fills; title: string; outline?: boolean}) {
  return <span role="img" aria-label={title} title={title} className={`status-dot status-dot-${kind} inline-block size-2 shrink-0 rounded-full ${(outline ? outlines : fills)[kind]}`} />;
}
