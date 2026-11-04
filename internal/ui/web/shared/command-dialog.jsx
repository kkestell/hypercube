import {useRef} from 'react';
import {Command} from '../components/ui/command';
import {Dialog, DialogContent, DialogDescription, DialogTitle} from '../components/ui/dialog';

export function CommandDialog({open, onOpenChange, title, description, commandProps, children}) {
  const invoker = useRef(null);
  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent className="overflow-hidden p-0" showCloseButton={false}
      onOpenAutoFocus={() => { invoker.current = document.activeElement; }}
      onCloseAutoFocus={event => { event.preventDefault(); invoker.current?.focus(); }}>
      <DialogTitle className="sr-only">{title}</DialogTitle>
      <DialogDescription className="sr-only">{description}</DialogDescription>
      <Command {...commandProps}>{children}</Command>
    </DialogContent>
  </Dialog>;
}
