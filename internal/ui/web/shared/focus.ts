import {useLayoutEffect, useRef} from 'react';

// focusedElement is the element that has focus, when one does.
export const focusedElement = (): HTMLElement | null => document.activeElement instanceof HTMLElement ? document.activeElement : null;

// Dialogs opened by shortcuts have no Radix trigger to restore focus to.
export function useDialogFocus(open: boolean) {
  const invoker = useRef<HTMLElement | null>(null);
  useLayoutEffect(() => {
    if (open) invoker.current = focusedElement();
  }, [open]);
  return {
    onOpenAutoFocus: () => { invoker.current = focusedElement(); },
    onCloseAutoFocus: (event: Event) => { event.preventDefault(); invoker.current?.focus(); },
  };
}
