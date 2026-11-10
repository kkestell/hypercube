// The page's regions: which ones show, the right panel's tab, and whether the
// page itself is visible.

import type {State} from '../state.ts';
import type {Dialog, Region, RightTab, SettingsSection, SidebarTab} from './types.ts';

export const openDialog = (state: State, dialog: Dialog): State => ({...state, dialog});

export const closeDialog = (state: State): State => state.dialog ? {...state, dialog: null} : state;

// setDialog opens dialog or closes it, as a dialog's onOpenChange reports.
export const setDialog = (state: State, dialog: Dialog, open: boolean): State =>
  open ? openDialog(state, dialog) : state.dialog === dialog ? closeDialog(state) : state;

export const openSettings = (state: State, section: SettingsSection = 'General'): State => ({...state, dialog: 'settings', settingsSection: section});

export const showSettingsSection = (state: State, section: SettingsSection): State => ({...state, settingsSection: section});

// showRightTab shows the right panel at tab.
export const showRightTab = (state: State, tab: RightTab): State => ({...state, workbench: {...state.workbench, rightPanel: true, rightTab: tab}});

// focusFileSearch shows file search and asks its field for focus.
export const focusFileSearch = (state: State): State => ({...showRightTab(state, 'search'), searchFocus: state.searchFocus + 1});

// toggle shows or hides the 'sidebar', 'rightPanel', or 'panel'.
export const toggle = (state: State, region: Region): State => region === 'panel'
  ? state.editor ? {...state, editor: {...state.editor, panel: !state.editor.panel}} : state
  : {...state, workbench: {...state.workbench, [region]: !state.workbench[region]}};

// hideRegion hides the 'sidebar', 'rightPanel', or 'panel', as when the user
// collapses it.
export const hideRegion = (state: State, region: Region): State => region === 'panel'
  ? closePanel(state)
  : {...state, workbench: {...state.workbench, [region]: false}};

export const selectRightTab = (state: State, tab: RightTab): State => ({...state, workbench: {...state.workbench, rightTab: tab}});

export const closePanel = (state: State): State => state.editor?.panel ? {...state, editor: {...state.editor, panel: false}} : state;

export const setVisible = (state: State, visible: boolean): State => ({...state, visible});

export const toggleWrap = (state: State): State => ({...state, workbench: {...state.workbench, wrap: !state.workbench.wrap}});

export const toggleFixedWidth = (state: State): State => ({...state, workbench: {...state.workbench, fixedWidth: !state.workbench.fixedWidth}});

export const resizeRegion = (state: State, region: Region | 'editor', size: number): State => state.workbench.sizes[region] === size
  ? state : {...state, workbench: {...state.workbench, sizes: {...state.workbench.sizes, [region]: size}}};

export const selectSidebarTab = (state: State, sidebarTab: SidebarTab): State => state.workbench.sidebarTab === sidebarTab
  ? state : {...state, workbench: {...state.workbench, sidebarTab}};

export const scrollSidebar = (state: State, tab: SidebarTab, top: number): State => state.workbench.sidebarScroll[tab] === top
  ? state : {...state, workbench: {...state.workbench, sidebarScroll: {...state.workbench.sidebarScroll, [tab]: top}}};

function expansion(state: State, field: 'collapsedWorkspaces' | 'collapsedSessions' | 'collapsedPullRequestGroups', id: string, open: boolean): State {
  if (!state.workbench[field][id] === open) return state;
  const collapsed = {...state.workbench[field]};
  if (open) delete collapsed[id];
  else collapsed[id] = true;
  return {...state, workbench: {...state.workbench, [field]: collapsed}};
}

export const expandWorkspace = (state: State, id: string, open: boolean): State => expansion(state, 'collapsedWorkspaces', id, open);
export const expandSession = (state: State, id: string, open: boolean): State => expansion(state, 'collapsedSessions', id, open);
export const expandPullRequestGroup = (state: State, id: string, open: boolean): State => expansion(state, 'collapsedPullRequestGroups', id, open);
