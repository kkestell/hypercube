// A keyboard event from the DOM or from React, which keeps the DOM event in
// nativeEvent.
interface SubmitKeyEvent {
  key: string;
  shiftKey?: boolean;
  altKey?: boolean;
  ctrlKey?: boolean;
  metaKey?: boolean;
  isComposing?: boolean;
  nativeEvent?: {isComposing: boolean};
}

// submitsOnKey reports whether a key press sends the message: Enter alone,
// outside of text composition.
export const submitsOnKey = (e: SubmitKeyEvent) => e.key === 'Enter' && !e.shiftKey && !e.altKey && !e.ctrlKey && !e.metaKey && !(e.isComposing || e.nativeEvent?.isComposing);
