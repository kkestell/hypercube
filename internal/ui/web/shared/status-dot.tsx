// A small colored dot whose title says what it means.
const colors = {exited: 'bg-destructive', waiting: 'bg-amber-500', unseen: 'bg-sky-500'};

export function StatusDot({kind, title}: {kind: keyof typeof colors; title: string}) {
  return <span role="img" aria-label={title} title={title} className={`status-dot status-dot-${kind} inline-block size-2 shrink-0 rounded-full ${colors[kind]}`} />;
}
