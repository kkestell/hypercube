// A stock menu anchored to a mouse or keyboard position.
import {useEffect, useRef, useState} from 'react';
import {flushSync} from 'react-dom';
import {DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuItem} from '../components/ui/dropdown-menu';
import {useOverlay} from './overlays.js';

function Items({items, onClose, invoker}) {
  const ref = useOverlay();
  const selected = useRef(false);
  return <DropdownMenuContent ref={ref} className="context-menu" align="start" sideOffset={0}
    onCloseAutoFocus={e => { e.preventDefault(); if (!selected.current) invoker?.focus(); }}>
    {items.map(item => <DropdownMenuItem key={item.label} onSelect={() => { selected.current = true; flushSync(onClose); invoker?.focus(); item.run(); }}>{item.label}</DropdownMenuItem>)}
  </DropdownMenuContent>;
}

export function ContextMenu({x, y, items, onClose}) {
  const [invoker] = useState(() => document.activeElement);
  useEffect(() => {
    addEventListener('blur', onClose);
    return () => removeEventListener('blur', onClose);
  }, [onClose]);
  return <DropdownMenu open onOpenChange={open => { if (!open) onClose(); }}>
    <DropdownMenuTrigger asChild><span aria-hidden="true" className="fixed size-0" style={{left: x, top: y}} /></DropdownMenuTrigger>
    <Items items={items} onClose={onClose} invoker={invoker} />
  </DropdownMenu>;
}
