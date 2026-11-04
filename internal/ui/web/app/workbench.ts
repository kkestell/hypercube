// The page's regions: which ones show, the right panel's tab, and whether the
// page itself is visible.

import type {State} from '../state.ts';
import type {Store} from '../store.ts';
import type {Region, RightTab} from './types.ts';

// toggle shows or hides the 'sidebar', 'rightPanel', or 'panel'.
export const toggle = (state: State, region: Region): State => ({...state, workbench: {...state.workbench, [region]: !state.workbench[region]}});

export const selectRightTab = (state: State, tab: RightTab): State => ({...state, workbench: {...state.workbench, rightTab: tab}});

export const closePanel = (state: State): State => state.workbench.panel ? {...state, workbench: {...state.workbench, panel: false}} : state;

export const setVisible = (state: State, visible: boolean): State => ({...state, visible});

export const activatePane = (state: State, id: number | null): State => state.workbench.activePane === id ? state : {...state, workbench: {...state.workbench, activePane: id}};

export const toggleWrap = (state: State): State => ({...state, workbench: {...state.workbench, wrap: !state.workbench.wrap}});

export function startWrapStorage(store: Store) {
  const wrap = localStorage.getItem('hypercube.wrap') === 'true';
  store.update(state => ({...state, workbench: {...state.workbench, wrap}}));
  let saved = wrap;
  return store.subscribe(state => {
    if (state.workbench.wrap === saved) return;
    saved = state.workbench.wrap;
    localStorage.setItem('hypercube.wrap', String(saved));
  });
}
