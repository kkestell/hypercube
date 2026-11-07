// The page's state, which the store holds: the server's workspaces, requests,
// and limits, and what the page itself keeps.
import type {CatalogEntry} from './agent/catalogs.ts';
import type {Draft} from './agent/types.ts';
import type {Config, Limit, TitleBar, Workbench} from './app/types.ts';
import type {FileOpenRequest} from './editor/types.ts';
import type {AttentionRequest, PageError} from './notifications/types.ts';
import type {AttachedSession, SessionViewState, Unsent, Workspace} from './sessions/types.ts';
import type {Submission} from './sessions/submissions.ts';
import type {Changes} from './changes/types.ts';
import type {FileEntry} from './files/types.ts';
import type {Confirmation} from './shared/types.ts';

// selected names the chosen session: null after the shown session stops, or
// an ID that the next workspaces may move to a running session. pending names
// a session chosen before the server listed it. loaded is set once the first
// workspaces arrive. drafts are keyed by draft key, dismissed by card key,
// and fileOpens by session ID. unsentDrafts holds parked new-session contexts
// by draft key; unsent holds the one currently shown. titleBar is null outside
// the native window.
// config is the most recently fetched or saved configuration. systemDark is
// whether the Mac's appearance is dark.
// sessionViews retains inactive sessions' view choices without server data.
// fileTrees keeps the last shown tree until its session is selected again.
// Read versions discard superseded responses; per-path versions belong to the
// current attachment and reset when it changes.
export interface State {
  workspaces: Workspace[];
  catalogs: Record<string, CatalogEntry>;
  loaded: boolean;
  config: Config | null;
  systemDark: boolean;
  limits: Limit[];
  requests: AttentionRequest[];
  selected: string | null;
  pending: string | null;
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
  sessionVersion: number;
  treeVersion: number;
  changesVersion: number;
  fileReadVersions: Record<string, number>;
  diffReadVersions: Record<string, number>;
  sessionViews: Record<string, SessionViewState>;
  fileTrees: Record<string, {listings: Record<string, FileEntry[] | null>; changes: Changes}>;
  fileOpens: Record<string, FileOpenRequest[]>;
  titleBar: TitleBar | null;
}

// initialState is the state before the server's first message, with the
// session the page's URL names selected.
export const initialState = (selected: string, visible: boolean): State => ({
  workspaces: [],
  catalogs: {},
  loaded: false,
  config: null,
  systemDark: false,
  limits: [],
  requests: [],
  selected,
  pending: null,
  unsent: null,
  unsentDrafts: {},
  visible,
  drafts: {},
  submissions: {},
  dismissed: {},
  errors: [],
  confirmation: null,
  workbench: {
    sidebar: true, rightPanel: true, rightTab: 'files', wrap: false, fixedWidth: true,
    sizes: {sidebar: 290, rightPanel: 320, panel: 240}, sidebarTab: 'sessions',
    collapsedWorkspaces: {}, collapsedSessions: {}, sidebarScroll: {sessions: 0, archived: 0},
  },
  session: null,
  sessionVersion: 0,
  treeVersion: 0,
  changesVersion: 0,
  fileReadVersions: {},
  diffReadVersions: {},
  sessionViews: {},
  fileTrees: {},
  fileOpens: {},
  titleBar: null,
});
