// Focus the mounted composer, if its chat is visible.
export function focusComposerField(): boolean {
  const field = document.querySelector('.composer-text');
  if (!(field instanceof HTMLElement)) return false;
  field.focus();
  return true;
}
