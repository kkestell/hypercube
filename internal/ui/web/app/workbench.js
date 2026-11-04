// The page's regions: which ones show, the right panel's tab, and whether the
// page itself is visible.

// toggle shows or hides the 'sidebar', 'rightPanel', or 'panel'.
export const toggle = (state, region) => ({...state, workbench: {...state.workbench, [region]: !state.workbench[region]}});

export const selectRightTab = (state, tab) => ({...state, workbench: {...state.workbench, rightTab: tab}});

export const closePanel = state => state.workbench.panel ? {...state, workbench: {...state.workbench, panel: false}} : state;

export const setVisible = (state, visible) => ({...state, visible});

export const toggleWrap = state => ({...state, workbench: {...state.workbench, wrap: !state.workbench.wrap}});
