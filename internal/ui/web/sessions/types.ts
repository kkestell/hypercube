// Workspaces and their sessions as the server shows them, and the page's own
// sessions: the unsent one and the attached one.
import type {EditorViewState} from '../editor/types.ts';
import type {Settings} from '../agent/types.ts';
import type {Transcript} from '../transcript/types.ts';

// Workspace is a folder that holds sessions, newest first. noGit marks a
// folder that is not a Git checkout.
export interface Workspace {
  id: string;
  path: string;
  name: string;
  noGit: boolean;
  sessions: Session[];
}

// Activity is the agent's work state; exited also covers startup before the
// plugin accepts the start command and does not establish process existence.
export type Activity = 'busy' | 'idle' | 'exited';

// Usage reports estimated context use, model context window, and cost in US
// dollars, when known. Whether context includes output depends on the harness;
// see docs/harness-support.md for each harness's accounting.
export interface Usage {
  context?: number;
  window?: number;
  cost?: number;
}

// Merge is a worktree session's branch measured against the branch checked
// out in its checkout. error is why it could not be measured; the other fields
// but branch are then empty.
export interface Merge {
  branch: string;
  target: string;
  unmerged: number;
  committed: boolean;
  uncommitted: number;
  error?: string;
}

// Session saves one agent's state. A top-level session lists its children;
// a child names its parent. An unseen idle session carries its last reply and
// that reply's seq. A worktree session carries its measured merge status.
export interface Session {
  id: string;
  parent: string;
  title: string;
  ready: boolean;
  ended: boolean;
  unseen: boolean;
  reply?: string;
  replySeq?: number;
  harness: string;
  model: string;
  effort: string;
  dir: string;
  activity: Activity;
  attention: boolean;
  usage: Usage;
  worktree: boolean;
  merge?: Merge;
  children: Session[];
}

// Unsent is a new session the user has opened but not yet sent a first prompt
// to. It lives in unsent while shown, or unsentDrafts when another view is shown.
// Its identity object tells it apart from a later unsent session. Null
// settings and worktree follow the defaults until the user chooses.
export interface Unsent {
  identity: object;
  workspace: string | null;
  settings: Settings | null;
  worktree: boolean | null;
  sending: boolean;
}

// AttachedSession holds only the real session's transcript and disclosures.
export interface AttachedSession {
  id: string;
  transcript: Transcript;
  transcriptReady: boolean;
  expanded: Record<string, true>;
}

// SessionViewState retains the historical saved format.
export interface SessionViewState extends EditorViewState {
  expanded: Record<string, true>;
}

// SessionSearchResult is a session whose title or conversation matches a
// search, with a snippet of the matching text.
export interface SessionSearchResult {
  id: string;
  title: string;
  workspace: string;
  text?: string;
}
