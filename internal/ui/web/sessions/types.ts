// Workspaces and their sessions as the server shows them, and the page's own
// sessions: the unsent one and the attached one.
import type {Changes, OpenDiff} from '../changes/types.ts';
import type {ScrollMemory, TabLayout} from '../editor/types.ts';
import type {SessionFiles} from '../files/types.ts';
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

// Activity is a session agent's process state.
export type Activity = 'busy' | 'idle' | 'exited';

// Usage is what a session's conversation has used: the tokens of its latest
// request, its model's context window, and its cost in US dollars, when known.
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

// Session is one agent. A top-level session lists its children; a child names
// its parent. An unseen idle session carries its last reply and that reply's
// seq. A worktree session carries its merge status when it has been measured.
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
// to. Its identity object tells it apart from a later unsent session.
export interface Unsent {
  identity: object;
  workspace: string | null;
  profile: string | null;
  worktree: boolean;
  sending: boolean;
}

// AttachedSession is the page's state for the session it shows: its
// transcript, the activities and messages the user expanded, its tabs and
// their remembered scroll, and its changes, diffs, and files.
export interface AttachedSession {
  id: string;
  transcript: Transcript;
  expanded: Record<string, true>;
  tabs: TabLayout;
  scroll: Record<string, ScrollMemory>;
  changes: Changes;
  diffs: Record<string, OpenDiff>;
  files: SessionFiles;
}

// SessionSearchResult is a session whose title or conversation matches a
// search, with a snippet of the matching text.
export interface SessionSearchResult {
  id: string;
  title: string;
  workspace: string;
  text?: string;
}
