// Shared editor tabs: a tree of panes and two-way splits, and the
// remembered scroll of each tab's view.
import type {ChangeComparison, Changes, OpenDiff} from '../changes/types.ts';
import type {EditorTarget} from './target.ts';
import type {LineRange, SessionFiles} from '../files/types.ts';
import type {IconName} from '../shared/icon.tsx';

// EditorView owns the file tools and tab layout for a session or workspace.
export interface EditorView {
  id: string;
  target: EditorTarget;
  activePane: number | null;
  panel: boolean;
  tabs: TabLayout;
  scroll: Record<string, ScrollMemory>;
  changes: Changes;
  branchChanges: Changes;
  gitComparison: ChangeComparison;
  diffs: Record<string, OpenDiff>;
  files: SessionFiles;
}

export interface EditorViewState {
  tabs: TabLayout;
  activePane: number | null;
  panel: boolean;
  scroll: Record<string, ScrollMemory>;
  folders: Record<string, true>;
  files: Record<string, {source: boolean; lines: LineRange[]}>;
}

// A tab ID is 'main', 'file:<path>', 'diff:<path>', or 'branch-diff:<path>'.
export type TabId = string;

// Pane is a group of tabs with one active tab.
export interface Pane {
  id: number;
  tabs: TabId[];
  active: TabId;
}

// Split divides its space between two nodes along an axis; ratio is the first
// child's percentage.
export interface Split {
  id: number;
  axis: 'x' | 'y';
  ratio: number;
  children: [LayoutNode, LayoutNode];
}

export type LayoutNode = Pane | Split;

// Placement says which edges of the tabbed area a pane touches. The top-left
// pane clears the native window buttons when the sidebar is hidden.
export interface Placement {
  top: boolean;
  left: boolean;
  right: boolean;
}

// Edge is where a dragged tab drops on a pane: into it, or beside it as a new pane.
export type Edge = 'center' | 'left' | 'right' | 'top' | 'bottom';

// TabLayout is the tree, the preview tab that the next opened file replaces,
// the tab being dragged, and the next pane or split ID. root is null once
// every tab is closed.
export interface TabLayout {
  root: LayoutNode | null;
  preview: TabId | null;
  dragging: TabId | null;
  nextId: number;
}

// TabEntry describes a tab for its tab strip. A file's source is null unless
// the file has a preview.
interface TabEntryBase {
  id: TabId;
  label: string;
  title: string;
  preview: boolean;
}

// MainTabEntry is the session's chat or the workspace's dashboard.
export interface MainTabEntry extends TabEntryBase {
  kind: 'main';
  icon: IconName;
}

export interface DiffTabEntry extends TabEntryBase {
  kind: 'diff';
  path: string;
  name: string;
}

export interface FileTabEntry extends TabEntryBase {
  kind: 'file';
  path: string;
  name: string;
  deleted: boolean;
  source: boolean | null;
}

export type TabEntry = MainTabEntry | DiffTabEntry | FileTabEntry;

// ScrollMemory is a view's scroll, and whether it follows its end.
export interface ScrollMemory {
  top: number;
  left: number;
  following: boolean;
}

// FileOpenRequest is an agent's request to open a file in a session. path is
// relative to the session's directory for a file inside it.
export interface FileOpenRequest {
  session: string;
  path: string;
  lines: LineRange[];
}
