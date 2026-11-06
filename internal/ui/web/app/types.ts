// The page's layout and the window and account details it shows.
import type {Profile} from '../agent/types.ts';

// Config is config.json: the agent profiles and the user's preferences.
// default_worktree starts new sessions in Git workspaces in a worktree.
export interface Config {
  default_profile: string;
  default_worktree: boolean;
  theme: Theme;
  terminal_font_size: number;
  notifications: Notifications;
  profiles: Profile[];
}

export type Theme = 'system' | 'light' | 'dark';

// Notifications chooses the alerts for sessions the page is not showing:
// requests and replies show as cards and, while the window is not focused,
// as macOS notifications when macos is set, with a sound when sound is set.
export interface Notifications {
  requests: boolean;
  replies: boolean;
  macos: boolean;
  sound: boolean;
}

export type RightTab = 'files' | 'search';
export type SidebarTab = 'sessions' | 'archived';

// Workbench holds global panel sizes and visibility, sidebar choices, and
// whether file views wrap and chats use a fixed reading width.
export interface Workbench {
  sidebar: boolean;
  rightPanel: boolean;
  panel: boolean;
  rightTab: RightTab;
  wrap: boolean;
  fixedWidth: boolean;
  sizes: Record<Region, number>;
  sidebarTab: SidebarTab;
  collapsedWorkspaces: Record<string, true>;
  collapsedSessions: Record<string, true>;
  sidebarScroll: Record<SidebarTab, number>;
}

export type Region = 'sidebar' | 'rightPanel' | 'panel';

// TitleBar is the native title-bar height and button inset in CSS pixels.
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
