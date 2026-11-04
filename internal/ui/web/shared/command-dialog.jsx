import {Command} from '../components/ui/command';
import {Dialog, DialogContent, DialogDescription, DialogTitle} from '../components/ui/dialog';
import {useModal} from './overlays.js';

export function CommandDialog({open, onOpenChange, title, description, commandProps, children}) {
  const modal = useModal(open);
  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent className="overflow-hidden p-0" showCloseButton={false} {...modal}>
      <DialogTitle className="sr-only">{title}</DialogTitle>
      <DialogDescription className="sr-only">{description}</DialogDescription>
      <Command {...commandProps}>{children}</Command>
    </DialogContent>
  </Dialog>;
}
