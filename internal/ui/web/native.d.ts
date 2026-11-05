// What the Hypercube window adds to the page: message handlers that take the
// page's messages, and events that carry the window's reports. A page outside
// the window has neither.

import type {TitleBar} from './app/types.ts';

interface MessageHandler {
  postMessage(message: object): void;
}

declare global {
  var webkit: {messageHandlers: {window?: MessageHandler}} | undefined;

  interface WindowEventMap {
    'hypercube-notification': CustomEvent<{session: string}>;
    'hypercube-shortcut': CustomEvent<{id: string}>;
    'hypercube-title-bar': CustomEvent<TitleBar>;
  }
}
