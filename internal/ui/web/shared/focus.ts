// focusedElement is the element that has focus, when one does.
export const focusedElement = (): HTMLElement | null => document.activeElement instanceof HTMLElement ? document.activeElement : null;
