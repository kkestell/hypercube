// An icon-only button. Its label is both its accessible name and its hover
// text, followed by the keys of its shortcut when it has one.
import type {ComponentProps} from 'react';
import {Button} from '../components/ui/button';
import {firstKeys, type ShortcutId} from '../app/shortcuts.ts';
import {Icon, type IconName} from './icon.tsx';

interface IconButtonProps extends Omit<ComponentProps<typeof Button>, 'children' | 'size'> {
  icon: IconName;
  label: string;
  shortcut?: ShortcutId;
}

export function IconButton({icon, label, shortcut, variant = 'ghost', ...props}: IconButtonProps) {
  return <Button variant={variant} size="icon-xs" aria-label={label} title={shortcut ? `${label} (${firstKeys(shortcut)})` : label} {...props}>
    <Icon name={icon} />
  </Button>;
}
