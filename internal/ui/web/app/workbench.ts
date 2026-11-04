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

export const toggleFixedWidth = (state: State): State => ({...state, workbench: {...state.workbench, fixedWidth: !state.workbench.fixedWidth}});

// column is the class that gives the composer and pending questions the chat's
// fixed reading width, centered between the gutters that hold the messages'
// copy buttons.
export const column = (state: State): string => state.workbench.fixedWidth ? 'mx-auto w-[min(calc(100%-5rem),48.6rem)]' : 'mx-10';

// startWorkbenchStorage restores the saved wrap and fixed-width settings and
// saves each change.
export function startWorkbenchStorage(store: Store) {
  const keys = {wrap: 'hypercube.wrap', fixedWidth: 'hypercube.fixedWidth'} as const;
  const saved = {wrap: localStorage.getItem(keys.wrap) === 'true', fixedWidth: localStorage.getItem(keys.fixedWidth) !== 'false'};
  store.update(state => ({...state, workbench: {...state.workbench, ...saved}}));
  return store.subscribe(state => {
    for (const name of ['wrap', 'fixedWidth'] as const) {
      if (state.workbench[name] === saved[name]) continue;
      saved[name] = state.workbench[name];
      localStorage.setItem(keys[name], String(saved[name]));
    }
  });
}
