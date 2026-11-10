// The text field of the mounted composer, which the composer registers while
// it is shown, so shortcuts and actions elsewhere can focus it.
let field: HTMLTextAreaElement | null = null;

// registerComposer makes element the field to focus until the returned
// function removes it.
export function registerComposer(element: HTMLTextAreaElement) {
  field = element;
  return () => { if (field === element) field = null; };
}

export function focusComposerField() {
  field?.focus();
}
