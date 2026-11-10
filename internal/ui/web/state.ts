// The page's state, which the store holds: the server's workspaces, GitHub
// accounts, requests, and limits, and what the page itself keeps.
import type {Draft} from './agent/types.ts';
import type {Config, Dialog, Limit, SettingsSection, TitleBar, Workbench} from './app/types.ts';
import type {EditorView, EditorViewState, FileOpenRequest} from './editor/types.ts';
import type {AttentionRequest, PageError} from './notifications/types.ts';
import type {AttachedSession, SessionViewState, Unsent, Workspace} from './sessions/types.ts';
import type {Submission} from './sessions/submissions.ts';
import type {Changes} from './changes/types.ts';
import type {FileEntry} from './files/types.ts';
import type {Confirmation} from './shared/types.ts';
import type {GitHubAccount} from './github/types.ts';

// selected names a session or workspace dashboard: null after the shown session stops, or
// an ID that the next workspaces may move to a running session. pending names
// a session chosen before the server listed it. endedShown names the active
// parents whose ended children the sidebar lists. loaded is set once the first
// workspaces arrive. dialog names the open dialog; settingsSection is the
// Settings section shown, kept while the dialog closes. renaming names the
// session whose title is being edited and the sidebar's order of top-level
// sessions when editing started, which the sidebar keeps until it ends.
// searchFocus counts requests to focus the file search field. drafts are
// keyed by draft key, dismissed by card key, and fileOpens by session ID. unsentDrafts holds parked new-session contexts
// by draft key; unsent holds the one currently shown. titleBar is null outside
// the native window.
// config is the most recently fetched or saved configuration. systemDark is
// whether the Mac's appearance is dark.
// sessionViews retains inactive sessions' view choices without server data.
// workspaceViews retains inactive dashboard choices; fileTrees caches listings
// until their view is selected again.
// Read versions discard superseded responses; per-path versions belong to the
// current attachment and reset when it changes.
export interface State {
  workspaces: Workspace[];
  accounts: GitHubAccount[];
  dialog: Dialog | null;
  settingsSection: SettingsSection;
  renaming: {session: string; order: Record<string, string[]>} | null;
  searchFocus: number;
  loaded: boolean;
  config: Config | null;
  systemDark: boolean;
  limits: Limit[];
  requests: AttentionRequest[];
  selected: string | null;
  pending: string | null;
  endedShown: Record<string, true>;
  unsent: Unsent | null;
  unsentDrafts: Record<string, Unsent>;
  visible: boolean;
  drafts: Record<string, Draft>;
  submissions: Record<string, Submission[]>;
  dismissed: Record<string, true>;
  errors: PageError[];
  confirmation: Confirmation | null;
  workbench: Workbench;
  session: AttachedSession | null;
  editor: EditorView | null;
  editorVersion: number;
  treeVersion: number;
  changesVersion: number;
  branchChangesVersion: number;
  fileReadVersions: Record<string, number>;
  diffReadVersions: Record<string, number>;
  sessionViews: Record<string, SessionViewState>;
  workspaceViews: Record<string, EditorViewState>;
  fileTrees: Record<string, {listings: Record<string, FileEntry[] | null>; changes: Changes}>;
  fileOpens: Record<string, FileOpenRequest[]>;
  titleBar: TitleBar | null;
}

// initialState is the state before the server's first message, with the
// session the page's URL names selected.
export const initialState = (selected: string, visible: boolean): State => ({
  workspaces: [],
  accounts: [],
  dialog: null,
  settingsSection: 'General',
  renaming: null,
  searchFocus: 0,
  loaded: false,
  config: null,
  systemDark: false,
  limits: [],
  requests: [],
  selected,
  pending: null,
  endedShown: {},
  unsent: null,
  unsentDrafts: {},
  visible,
  drafts: {},
  submissions: {},
  dismissed: {},
  errors: [],
  confirmation: null,
  workbench: {
    sidebar: true, rightPanel: false, rightTab: 'files', wrap: false, fixedWidth: true,
    sizes: {sidebar: 290, rightPanel: 320, panel: 240, editor: 560}, sidebarTab: 'sessions',
    collapsedWorkspaces: {}, collapsedSessions: {}, collapsedPullRequestGroups: {}, sidebarScroll: {sessions: 0, archived: 0},
  },
  session: null,
  editor: null,
  editorVersion: 0,
  treeVersion: 0,
  changesVersion: 0,
  branchChangesVersion: 0,
  fileReadVersions: {},
  diffReadVersions: {},
  sessionViews: {},
  workspaceViews: {},
  fileTrees: {},
  fileOpens: {},
  titleBar: null,
});
