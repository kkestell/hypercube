// Focus the mounted composer, if its chat is visible.
export function focusComposerField() {
  const field = document.querySelector('.composer-text');
  if (!field) return false;
  field.focus();
  return true;
}

