import {afterEach, beforeEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {clickControl, find, mount, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {startViewStorage} from '../app/view-storage.ts';
import {resizeRegion} from '../app/workbench.ts';
import {receivedWorkspaces} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {Sidebar} from './sidebar.tsx';

beforeEach(() => localStorage.clear());
afterEach(() => {
  unmountAll();
  localStorage.clear();
  mock.restore();
});

const workspaces = [testWorkspace({id: 'w', name: 'repo', sessions: [
  testSession({id: 'parent', children: [testSession({id: 'child', parent: 'parent'})]}),
  testSession({id: 'archived', ended: true}),
]})];
const sidebar = (store: ReturnType<typeof createStore>) => mount(store, <Sidebar searchOpen={false} onSearchOpenChange={() => {}} onSettings={() => {}} />);
const click = (element: HTMLElement) => act(() => clickControl(element));
const tab = (root: HTMLElement, name: string) => find(root, `[role=tab][aria-label="${name}"]`);
const body = (root: HTMLElement, name: string) => find(root, `[data-sidebar-tab=${name}]`);
const scroll = (element: HTMLElement, top: number) => act(() => {
  element.scrollTop = top;
  element.dispatchEvent(new Event('scroll'));
});

test('sidebar choices, independent scroll positions, and panel sizes survive a browser reload', () => {
  const store = createStore(testState({selected: null, loaded: true, workspaces}));
  const stop = startViewStorage(store);
  let root = sidebar(store);
  const active = body(root, 'sessions');
  act(() => find(root, '[data-session=parent]').dispatchEvent(new KeyboardEvent('keydown', {key: 'ArrowLeft', bubbles: true})));
  expect(root.querySelector('[data-session=child]')).toBeNull();
  scroll(active, 110);
  click(tab(root, 'Archived'));
  const archived = body(root, 'archived');
  expect(archived).not.toBe(active);
  scroll(archived, 230);
  click(tab(root, 'Sessions'));
  expect(active.scrollTop).toBe(110);
  expect(store.get().workbench.sidebarScroll).toEqual({sessions: 110, archived: 230});
  click(find(root, '[aria-label="Collapse repo"]'));
  click(tab(root, 'Archived'));
  archived.scrollTop = 250;
  act(() => window.dispatchEvent(new Event('pagehide')));
  act(() => store.update(state => resizeRegion(resizeRegion(resizeRegion(state, 'sidebar', 290), 'rightPanel', 390), 'panel', 210)));
  unmountAll();
  stop();

  const restored = createStore(testState({selected: 'parent', loaded: true, workspaces}));
  const stopRestored = startViewStorage(restored);
  root = sidebar(restored);
  expect(tab(root, 'Archived').getAttribute('aria-selected')).toBe('true');
  expect(body(root, 'archived').scrollTop).toBe(250);
  expect(restored.get().workbench.sizes).toEqual({sidebar: 290, rightPanel: 390, panel: 210});
  expect(find(root, '[aria-label="Expand repo"]').getAttribute('aria-expanded')).toBe('false');
  click(find(root, '[aria-label="Expand repo"]'));
  expect(root.querySelector('[data-session=archived]')).not.toBeNull();
  click(tab(root, 'Sessions'));
  expect(root.querySelector('[data-session=parent]')).not.toBeNull();
  expect(root.querySelector('[data-session=child]')).toBeNull();
  expect(body(root, 'sessions').scrollTop).toBe(110);
  unmountAll();
  stopRestored();
});

test('sidebar scrolling restores after workspace data arrives', () => {
  const state = testState({selected: null});
  const store = createStore({...state, workbench: {...state.workbench, sidebarScroll: {sessions: 180, archived: 40}}});
  const root = sidebar(store);
  act(() => store.update(state => receivedWorkspaces(state, workspaces)));
  expect(body(root, 'sessions').scrollTop).toBe(180);
  scroll(body(root, 'archived'), 0);
  expect(store.get().workbench.sidebarScroll.archived).toBe(40);
  click(tab(root, 'Archived'));
  expect(body(root, 'archived').scrollTop).toBe(40);
});
