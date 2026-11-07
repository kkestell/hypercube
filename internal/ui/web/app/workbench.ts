// The page's regions: which ones show, the right panel's tab, and whether the
// page itself is visible.

import type {State} from '../state.ts';
import type {Region, RightTab, SidebarTab} from './types.ts';

// toggle shows or hides the 'sidebar', 'rightPanel', or 'panel'.
export const toggle = (state: State, region: Region): State => region === 'panel'
  ? state.editor ? {...state, editor: {...state.editor, panel: !state.editor.panel}} : state
  : {...state, workbench: {...state.workbench, [region]: !state.workbench[region]}};

export const selectRightTab = (state: State, tab: RightTab): State => ({...state, workbench: {...state.workbench, rightTab: tab}});

export const closePanel = (state: State): State => state.editor?.panel ? {...state, editor: {...state.editor, panel: false}} : state;

export const setVisible = (state: State, visible: boolean): State => ({...state, visible});

export const toggleWrap = (state: State): State => ({...state, workbench: {...state.workbench, wrap: !state.workbench.wrap}});

export const toggleFixedWidth = (state: State): State => ({...state, workbench: {...state.workbench, fixedWidth: !state.workbench.fixedWidth}});

// column is the class that lines the composer and pending questions up with the
// transcript's messages: the panels' inset on both sides, and the fixed reading
// width when it is on.
export const column = (state: State): string =>
  `px-4 ${state.workbench.fixedWidth ? 'mx-auto w-full max-w-[50.6rem]' : ''}`;

export const resizeRegion = (state: State, region: Region, size: number): State => state.workbench.sizes[region] === size
  ? state : {...state, workbench: {...state.workbench, sizes: {...state.workbench.sizes, [region]: size}}};

export const selectSidebarTab = (state: State, sidebarTab: SidebarTab): State => state.workbench.sidebarTab === sidebarTab
  ? state : {...state, workbench: {...state.workbench, sidebarTab}};

export const scrollSidebar = (state: State, tab: SidebarTab, top: number): State => state.workbench.sidebarScroll[tab] === top
  ? state : {...state, workbench: {...state.workbench, sidebarScroll: {...state.workbench.sidebarScroll, [tab]: top}}};

function expansion(state: State, field: 'collapsedWorkspaces' | 'collapsedSessions', id: string, open: boolean): State {
  if (!state.workbench[field][id] === open) return state;
  const collapsed = {...state.workbench[field]};
  if (open) delete collapsed[id];
  else collapsed[id] = true;
  return {...state, workbench: {...state.workbench, [field]: collapsed}};
}

export const expandWorkspace = (state: State, id: string, open: boolean): State => expansion(state, 'collapsedWorkspaces', id, open);
export const expandSession = (state: State, id: string, open: boolean): State => expansion(state, 'collapsedSessions', id, open);
