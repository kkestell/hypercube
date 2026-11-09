// Browser storage is an external boundary; runtime state uses these shapes directly.
import type {EditorViewState, LayoutNode} from '../editor/types.ts';
import type {SessionViewState} from '../sessions/types.ts';
import type {Workbench} from './types.ts';

export interface SavedViews {
  selected: string | null;
  workbench: Workbench;
  sessionViews: Record<string, SessionViewState>;
  workspaceViews: Record<string, EditorViewState>;
}

const record = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null && !Array.isArray(value);
const nonnegative = (value: unknown): value is number => typeof value === 'number' && Number.isFinite(value) && value >= 0;
const entries = (value: unknown, check: (entry: unknown) => boolean) => record(value) && Object.values(value).every(check);
const flags = (value: unknown) => entries(value, entry => entry === true);
const positiveInteger = (value: unknown) => nonnegative(value) && Number.isInteger(value) && value > 0;

function layout(value: unknown): value is LayoutNode | null {
  if (value === null) return true;
  if (!record(value) || !positiveInteger(value.id)) return false;
  if (Array.isArray(value.tabs)) {
    return value.tabs.length > 0 && value.tabs.every(tab => typeof tab === 'string') && typeof value.active === 'string' && value.tabs.includes(value.active);
  }
  return (value.axis === 'x' || value.axis === 'y') && nonnegative(value.ratio) && value.ratio >= 15 && value.ratio <= 85
    && Array.isArray(value.children) && value.children.length === 2 && value.children.every(child => child !== null && layout(child));
}

function editorView(value: unknown) {
  if (!record(value) || typeof value.panel !== 'boolean' || !record(value.tabs) || !layout(value.tabs.root) || !positiveInteger(value.tabs.nextId)
    || !(value.tabs.preview === null || typeof value.tabs.preview === 'string') || value.tabs.dragging !== null
    || !(value.activePane === null || positiveInteger(value.activePane)) || !flags(value.folders)) return false;
  const scroll = entries(value.scroll, entry => record(entry) && nonnegative(entry.top) && nonnegative(entry.left) && typeof entry.following === 'boolean');
  const files = entries(value.files, entry => record(entry) && typeof entry.source === 'boolean' && Array.isArray(entry.lines)
    && entry.lines.every(line => record(line) && positiveInteger(line.start) && (line.end === undefined || positiveInteger(line.end))));
  return scroll && files;
}

const sessionView = (value: unknown) => editorView(value) && record(value) && flags(value.expanded);

function workbench(value: unknown) {
  if (!record(value) || !['sidebar', 'rightPanel', 'wrap', 'fixedWidth'].every(key => typeof value[key] === 'boolean')) return false;
  const sizes = value.sizes;
  return (value.rightTab === 'files' || value.rightTab === 'git' || value.rightTab === 'search') && (value.sidebarTab === 'sessions' || value.sidebarTab === 'archived')
    && record(sizes) && ['sidebar', 'rightPanel', 'panel', 'editor'].every(key => nonnegative(sizes[key]) && sizes[key] > 0)
    && flags(value.collapsedWorkspaces) && flags(value.collapsedSessions)
    && record(value.sidebarScroll) && nonnegative(value.sidebarScroll.sessions) && nonnegative(value.sidebarScroll.archived);
}

export function readSavedViews(text: string): SavedViews {
  const value: unknown = JSON.parse(text);
  if (!record(value) || !(value.selected === null || typeof value.selected === 'string')
    || !workbench(value.workbench) || !entries(value.sessionViews, sessionView) || !entries(value.workspaceViews, editorView)) throw new Error('Invalid saved view state.');
  return value as unknown as SavedViews;
}
