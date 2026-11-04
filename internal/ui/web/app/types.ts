// The page's layout and the window and account details it shows.

export type RightTab = 'files' | 'search';

// Workbench is which regions show, the right panel's tab, the focused editor
// pane, whether file views wrap lines, and whether chats and Markdown
// previews use a fixed reading width.
export interface Workbench {
  sidebar: boolean;
  rightPanel: boolean;
  panel: boolean;
  rightTab: RightTab;
  activePane: number | null;
  wrap: boolean;
  fixedWidth: boolean;
}

export type Region = 'sidebar' | 'rightPanel' | 'panel';

// TitleBar is the window's title bar height and the width of its buttons, in
// CSS pixels.
export interface TitleBar {
  height: number;
  inset: number;
}

// Limit is the newest reading of one usage window of a harness. minutes is
// the window's length; used is the percent of the limit used; resets is when
// the window resets, in Unix seconds, when known; at is when the reading was
// taken, in Unix milliseconds.
export interface Limit {
  harness: string;
  minutes: number;
  used: number;
  resets?: number;
  at: number;
}

// Grant is a saved always-allow choice for a harness's tool, in one session
// or, when session is empty, in all.
export interface Grant {
  session: string;
  title: string;
  harness: string;
  tool: string;
}
