// Ordinary session states stay neutral and carry their names on hover.
import {Circle, CircleHelp, CircleStop} from 'lucide-react';

const icons = {exited: CircleStop, waiting: CircleHelp, unseen: Circle};

export function StatusIcon({kind, title}) {
  const Icon = icons[kind];
  return <span title={title} className="inline-flex text-muted-foreground">
    <Icon role="img" aria-label={title} aria-hidden={false} className={kind === 'unseen' ? 'size-2 fill-current' : 'size-4'} />
  </span>;
}
