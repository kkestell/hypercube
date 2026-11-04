import {expect, test} from 'bun:test';
import {browserTabId, closeTab, fileTabId} from '../editor/editor.ts';
import {openTab, panes} from '../editor/tab-layout.ts';
import {emptySession, receivedWorkspaces, selectSession} from '../sessions/selection.ts';
import type {AttachedSession} from '../sessions/types.ts';
import type {State} from '../state.ts';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {closedBrowser, receivedBrowser} from './browser.ts';
import type {BrowserPage} from './types.ts';

const page: BrowserPage = {url: 'https://example.com/', title: 'Example', loading: false, canGoBack: false, canGoForward: false};
const workspaces = [testWorkspace({id: 'w', sessions: [testSession({id: 's', running: true}), testSession({id: 'other', running: true})]})];
const attached = () => testState({selected: 's', workspaces, session: emptySession('s')});
const shown = (state: State): AttachedSession => {
  if (!state.session) throw new Error('No session is attached.');
  return state.session;
};
const tabsOf = (state: State) => panes(shown(state).tabs.root).map(pane => pane.tabs);
const activeTabs = (state: State) => panes(shown(state).tabs.root).map(pane => pane.active);

test('the attached session\'s Browser tab opens as a tab beside the agent\'s tab when the page learns of its browser', () => {
  const state = receivedBrowser(attached(), {session: 's', reveal: false, ...page});
  expect(state.browsers.s).toEqual(page);
  expect(tabsOf(state)).toEqual([['agent', browserTabId]]);
});

test('a closed Browser tab stays closed until the agent navigates', () => {
  let state = receivedBrowser(attached(), {session: 's', reveal: false, ...page});
  state = closeTab(state, browserTabId);
  state = receivedBrowser(state, {session: 's', reveal: false, ...page, title: 'Changed'});
  expect(tabsOf(state)).toEqual([['agent']]);
  expect(state.browsers.s.title).toBe('Changed');
  state = receivedBrowser(state, {session: 's', reveal: true, ...page});
  expect(tabsOf(state)).toEqual([['agent', browserTabId]]);
});

test('an agent navigation selects the Browser tab', () => {
  let state = receivedBrowser(attached(), {session: 's', reveal: false, ...page});
  state = {...state, session: {...shown(state), tabs: openTab(shown(state).tabs, fileTabId('a.go'))}};
  expect(activeTabs(state)).toEqual([fileTabId('a.go')]);
  state = receivedBrowser(state, {session: 's', reveal: true, ...page});
  expect(activeTabs(state)).toEqual([browserTabId]);
});

test('an agent navigation leaves a shown chat in front of the Browser tab', () => {
  let state = receivedBrowser(attached(), {session: 's', reveal: true, ...page});
  expect(tabsOf(state)).toEqual([['agent', browserTabId]]);
  expect(activeTabs(state)).toEqual(['agent']);
  state = receivedBrowser(state, {session: 's', reveal: true, ...page});
  expect(activeTabs(state)).toEqual(['agent']);
});

test('another session\'s browser opens its tab when that session is attached', () => {
  let state = receivedBrowser(attached(), {session: 'other', reveal: true, ...page});
  expect(tabsOf(state)).toEqual([['agent']]);
  state = selectSession(state, 'other');
  expect(state.session?.id).toBe('other');
  expect(tabsOf(state)).toEqual([['agent', browserTabId]]);
  state = receivedWorkspaces(state, workspaces);
  expect(tabsOf(state)).toEqual([['agent', browserTabId]]);
});

test('a closed browser closes its tab and stays closed when the session is attached again', () => {
  let state = receivedBrowser(attached(), {session: 's', reveal: true, ...page});
  state = closedBrowser(state, {session: 's'});
  expect(state.browsers.s).toBeUndefined();
  expect(tabsOf(state)).toEqual([['agent']]);
  state = selectSession(selectSession(state, 'other'), 's');
  expect(tabsOf(state)).toEqual([['agent']]);
});
