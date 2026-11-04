// A session's browser, as the Hypercube window reports it.

export interface BrowserPage {
  url: string;
  title: string;
  loading: boolean;
  canGoBack: boolean;
  canGoForward: boolean;
}

// BrowserReport is the window's report of a browser's state; reveal asks the
// page to show it.
export interface BrowserReport extends BrowserPage {
  session: string;
  reveal: boolean;
}

// BrowserRect is where the window shows a browser, in CSS pixels.
export interface BrowserRect {
  x: number;
  y: number;
  width: number;
  height: number;
}

// BrowserMessage is what the page asks of the window. A place message with no
// session hides every browser.
export type BrowserMessage =
  | {type: 'ready'}
  | {type: 'navigate'; session: string; url: string}
  | {type: 'back' | 'forward' | 'reload' | 'external'; session: string}
  | {type: 'place'; session: string | null; rect: BrowserRect | null};
