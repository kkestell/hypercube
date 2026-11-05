// The page's state, which the store holds: the server's workspaces, requests,
// and limits, and what the page itself keeps.
import type {Draft} from './agent/types.ts';
import type {Config, Limit, TitleBar, Workbench} from './app/types.ts';
import type {FileOpenRequest} from './editor/types.ts';
import type {AttentionRequest, PageError} from './notifications/types.ts';
import type {AttachedSession, Unsent, Workspace} from './sessions/types.ts';
import type {Confirmation} from './shared/types.ts';

// selected names the chosen session: null after the shown session stops, or
// an ID that the next workspaces may move to a running session. pending names
// a session chosen before the server listed it. loaded is set once the first
// workspaces arrive. drafts are keyed by draft key, dismissed by card key,
// and fileOpens by session ID. titleBar is null outside the native window.
// config is the most recently fetched or saved configuration.
export interface State {
  workspaces: Workspace[];
  loaded: boolean;
  config: Config | null;
  limits: Limit[];
  requests: AttentionRequest[];
  selected: string | null;
  pending: string | null;
  unsent: Unsent | null;
  visible: boolean;
  drafts: Record<string, Draft>;
  dismissed: Record<string, true>;
  errors: PageError[];
  confirmation: Confirmation | null;
  workbench: Workbench;
  session: AttachedSession | null;
  fileOpens: Record<string, FileOpenRequest[]>;
  titleBar: TitleBar | null;
}

// initialState is the state before the server's first message, with the
// session the page's URL names selected.
export const initialState = (selected: string, visible: boolean): State => ({
  workspaces: [],
  loaded: false,
  config: null,
  limits: [],
  requests: [],
  selected,
  pending: null,
  unsent: null,
  visible,
  drafts: {},
  dismissed: {},
  errors: [],
  confirmation: null,
  workbench: {sidebar: true, rightPanel: true, panel: false, rightTab: 'files', activePane: null, wrap: false, fixedWidth: true},
  session: null,
  fileOpens: {},
  titleBar: null,
});
