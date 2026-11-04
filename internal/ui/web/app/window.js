// The native window's message handler is absent in a browser.
export const windowHandler = () => globalThis.webkit?.messageHandlers?.window;
