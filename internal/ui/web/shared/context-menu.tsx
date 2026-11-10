// A stock menu anchored to a mouse or keyboard position. Its items come in
// groups, drawn with a separator between each. It stays mounted so it can
// animate closed; each time it opens it remembers the element that had focus,
// which gets the focus back when it closes. While it animates closed it is
// inert, so the pointer cannot focus it away from what an item focused.
import {Fragment, useEffect, useLayoutEffect, useRef, useState, type RefObject} from 'react';
import {flushSync} from 'react-dom';
import {DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator} from '../components/ui/dropdown-menu';
import {focusedElement} from './focus.ts';
import {Icon, type IconName} from './icon.tsx';

// ContextMenuItem is one choice of a context menu. A disabled item shows but
// cannot be chosen.
export interface ContextMenuItem {
  label: string;
  icon: IconName;
  destructive?: boolean;
  disabled?: boolean;
  run: () => void;
}

interface ItemsProps {
  open: boolean;
  items: ContextMenuItem[][];
  onClose: () => void;
  invoker: HTMLElement | null;
  selected: RefObject<boolean>;
}

function Items({open, items, onClose, invoker, selected}: ItemsProps) {
  return <DropdownMenuContent align="start" sideOffset={0} inert={!open}
    onCloseAutoFocus={e => {
      e.preventDefault();
      if (!selected.current) invoker?.focus();
    }}>
    {items.filter(group => group.length).map((group, i) => <Fragment key={group[0].label}>
      {i > 0 && <DropdownMenuSeparator />}
      {group.map(item => <DropdownMenuItem key={item.label} variant={item.destructive ? 'destructive' : 'default'} disabled={item.disabled}
        onSelect={() => { selected.current = true; flushSync(onClose); invoker?.focus(); item.run(); }}>
        <Icon name={item.icon} />{item.label}
      </DropdownMenuItem>)}
    </Fragment>)}
  </DropdownMenuContent>;
}

interface ContextMenuProps {
  open: boolean;
  x: number;
  y: number;
  items: ContextMenuItem[][];
  onClose: () => void;
}

export function ContextMenu({open, x, y, items, onClose}: ContextMenuProps) {
  const [invoker, setInvoker] = useState<HTMLElement | null>(null);
  // selected is set once an item is chosen, which returns the focus itself.
  const selected = useRef(false);
  useLayoutEffect(() => {
    if (!open) return;
    setInvoker(focusedElement());
    selected.current = false;
  }, [open]);
  useEffect(() => {
    if (!open) return;
    addEventListener('blur', onClose);
    return () => removeEventListener('blur', onClose);
  }, [open, onClose]);
  return <DropdownMenu open={open} onOpenChange={next => { if (!next) onClose(); }}>
    <DropdownMenuTrigger asChild><span aria-hidden="true" className="fixed size-0" style={{left: x, top: y}} /></DropdownMenuTrigger>
    <Items open={open} items={items} onClose={onClose} invoker={invoker} selected={selected} />
  </DropdownMenu>;
}
