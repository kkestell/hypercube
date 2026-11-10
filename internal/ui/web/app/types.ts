// The page's layout and the window and account details it shows.
import type {ModelEntry, Settings} from '../agent/types.ts';

// Config is config.json: the user's preferences. default_selection starts new
// sessions with its settings; child_model_guidance tells agents when a child
// should use other settings than theirs; default_worktree starts new sessions
// in Git workspaces in a worktree; review_selection, when set, starts pull
// request reviews with its settings, and review_prompt is their first prompt;
// utility_selection, when set, names new sessions after their first message;
// models lists the model picker's entries.
export interface Config {
  default_selection?: Settings;
  child_model_guidance: string;
  default_worktree: boolean;
  review_selection?: Settings;
  review_prompt: string;
  utility_selection?: Settings;
  theme: Theme;
  terminal_font_size: number;
  notifications: Notifications;
  models: ModelEntry[];
}

export type Theme = 'system' | 'light' | 'dark';

// Notifications chooses the alerts for sessions the page is not showing:
// requests and replies show as cards and, while the window is not focused,
// as desktop notifications when desktop is set, with a sound when sound is set.
export interface Notifications {
  requests: boolean;
  replies: boolean;
  desktop: boolean;
  sound: boolean;
}

export type RightTab = 'files' | 'git' | 'search';
export type SidebarTab = 'sessions' | 'archived';

// Workbench holds panel sizes, sidebar and right-panel visibility and choices, and
// whether source views wrap and Markdown file previews use a fixed reading width.
export interface Workbench {
  sidebar: boolean;
  rightPanel: boolean;
  rightTab: RightTab;
  wrap: boolean;
  fixedWidth: boolean;
  sizes: Record<Region | 'editor', number>;
  sidebarTab: SidebarTab;
  collapsedWorkspaces: Record<string, true>;
  collapsedSessions: Record<string, true>;
  collapsedPullRequestGroups: Record<string, true>;
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
export const sections = ['General', 'Agents', 'Review', 'Notifications', 'Permissions'] as const;
export type SettingsSection = typeof sections[number];
