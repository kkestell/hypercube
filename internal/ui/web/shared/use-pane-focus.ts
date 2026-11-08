// Find belongs to the pane containing keyboard focus. An unfocused window or
// the page background leaves the main view's shortcuts available.
import {useEffect, useState, type RefObject} from 'react';

export function usePaneFocus(pane: RefObject<HTMLElement | null>, background = true) {
  const [focused, setFocused] = useState(background);
  useEffect(() => {
    const focusIn = (event: FocusEvent) => {
      const target = event.target;
      setFocused((target === document.body && background) || (target instanceof Node && !!pane.current?.contains(target)));
    };
    const focusOut = (event: FocusEvent) => { if (!event.relatedTarget) setFocused(background); };
    document.addEventListener('focusin', focusIn);
    document.addEventListener('focusout', focusOut);
    return () => {
      document.removeEventListener('focusin', focusIn);
      document.removeEventListener('focusout', focusOut);
    };
  }, [background]);
  return focused;
}
