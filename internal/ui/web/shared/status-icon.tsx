// An icon whose shape and color give a status, with the status in words as
// its accessible name and hover text.
import {Icon, type IconName} from './icon.tsx';

export function StatusIcon({icon, color, title, className = ''}: {icon: IconName; color: string; title: string; className?: string}) {
  return <span role="img" aria-label={title} title={title} className={`inline-flex ${className}`}>
    <Icon name={icon} className={color} />
  </span>;
}
