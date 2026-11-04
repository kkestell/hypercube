// submitsOnKey reports whether a key press sends the message: Enter alone,
// outside of text composition.
export const submitsOnKey = e => e.key === 'Enter' && !e.shiftKey && !e.altKey && !e.ctrlKey && !e.metaKey && !(e.isComposing || e.nativeEvent?.isComposing);
