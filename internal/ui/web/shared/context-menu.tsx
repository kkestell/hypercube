// A stock menu anchored to a mouse or keyboard position. Its items come in
// groups, drawn with a separator between each.
import {Fragment, useEffect, useRef, useState} from 'react';
import {flushSync} from 'react-dom';
import {DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator} from '../components/ui/dropdown-menu';
import {focusedElement} from './focus.ts';
import {Icon, type IconName} from './icon.tsx';

// ContextMenuItem is one choice of a context menu.
export interface ContextMenuItem {
  label: string;
  icon: IconName;
  destructive?: boolean;
  run: () => void;
}

interface ItemsProps {
  open: boolean;
  items: ContextMenuItem[][];
  onClose: () => void;
  invoker: HTMLElement | null;
}

function Items({open, items, onClose, invoker}: ItemsProps) {
  const selected = useRef(false);
  useEffect(() => { if (open) selected.current = false; }, [open]);
  return <DropdownMenuContent className="context-menu" align="start" sideOffset={0}
    onCloseAutoFocus={e => { e.preventDefault(); if (!selected.current) invoker?.focus(); }}>
    {items.filter(group => group.length).map((group, i) => <Fragment key={group[0].label}>
      {i > 0 && <DropdownMenuSeparator />}
      {group.map(item => <DropdownMenuItem key={item.label} variant={item.destructive ? 'destructive' : 'default'}
        onSelect={() => { selected.current = true; flushSync(onClose); invoker?.focus(); item.run(); }}>
        <Icon name={item.icon} />{item.label}
      </DropdownMenuItem>)}
    </Fragment>)}
  </DropdownMenuContent>;
}

interface ContextMenuProps {
  open?: boolean;
  x: number;
  y: number;
  items: ContextMenuItem[][];
  onClose: () => void;
}

export function ContextMenu({open = true, x, y, items, onClose}: ContextMenuProps) {
  const [invoker] = useState(focusedElement);
  useEffect(() => {
    addEventListener('blur', onClose);
    return () => removeEventListener('blur', onClose);
  }, [onClose]);
  return <DropdownMenu open={open} onOpenChange={open => { if (!open) onClose(); }}>
    <DropdownMenuTrigger asChild><span aria-hidden="true" className="fixed size-0" style={{left: x, top: y}} /></DropdownMenuTrigger>
    <Items open={open} items={items} onClose={onClose} invoker={invoker} />
  </DropdownMenu>;
}
