// A stock menu anchored to a mouse or keyboard position. Its items come in
// groups, drawn with a separator between each.
import {Fragment, useEffect, useRef, useState} from 'react';
import {flushSync} from 'react-dom';
import {DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator} from '../components/ui/dropdown-menu';
import {focusedElement} from './focus.ts';
import {Icon, type IconName} from './icon.tsx';
import {useOverlay} from './overlays.ts';

// ContextMenuItem is one choice of a context menu.
export interface ContextMenuItem {
  label: string;
  icon: IconName;
  destructive?: boolean;
  run: () => void;
}

interface ItemsProps {
  items: ContextMenuItem[][];
  onClose: () => void;
  invoker: HTMLElement | null;
}

function Items({items, onClose, invoker}: ItemsProps) {
  const ref = useOverlay();
  const selected = useRef(false);
  return <DropdownMenuContent ref={ref} className="context-menu" align="start" sideOffset={0}
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
  x: number;
  y: number;
  items: ContextMenuItem[][];
  onClose: () => void;
}

export function ContextMenu({x, y, items, onClose}: ContextMenuProps) {
  const [invoker] = useState(focusedElement);
  useEffect(() => {
    addEventListener('blur', onClose);
    return () => removeEventListener('blur', onClose);
  }, [onClose]);
  return <DropdownMenu open onOpenChange={open => { if (!open) onClose(); }}>
    <DropdownMenuTrigger asChild><span aria-hidden="true" className="fixed size-0" style={{left: x, top: y}} /></DropdownMenuTrigger>
    <Items items={items} onClose={onClose} invoker={invoker} />
  </DropdownMenu>;
}
