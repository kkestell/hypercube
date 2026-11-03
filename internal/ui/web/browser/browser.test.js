import {expect, test} from 'bun:test';
import {browserTabId, closeTab, fileTabId} from '../editor/editor.js';
import {findPane, moveTab, openTab, panes} from '../editor/tab-layout.js';
import {emptySession, receivedWorkspaces, selectSession} from '../sessions/selection.js';
import {receivedBrowser} from './browser.js';

const page = {url: 'https://example.com/', title: 'Example', loading: false, canGoBack: false, canGoForward: false};
const workspaces = [{id: 'w', sessions: [{id: 's', running: true, children: []}, {id: 'other', running: true, children: []}]}];
const attached = () => ({selected: 's', pending: null, unsent: null, drafts: {}, workspaces, browsers: {}, session: emptySession('s')});
const tabsOf = state => panes(state.session.tabs.root).map(pane => pane.tabs);
const activeTabs = state => panes(state.session.tabs.root).map(pane => pane.active);

test('the attached session\'s Browser tab opens beside the agent when the page learns of its browser', () => {
  const state = receivedBrowser(attached(), {session: 's', reveal: false, ...page});
  expect(state.browsers.s).toEqual(page);
  expect(tabsOf(state)).toEqual([['agent'], [browserTabId]]);
});

test('a closed Browser tab stays closed until the agent navigates', () => {
  let state = receivedBrowser(attached(), {session: 's', reveal: false, ...page});
  state = closeTab(state, browserTabId);
  state = receivedBrowser(state, {session: 's', reveal: false, ...page, title: 'Changed'});
  expect(tabsOf(state)).toEqual([['agent']]);
  expect(state.browsers.s.title).toBe('Changed');
  state = receivedBrowser(state, {session: 's', reveal: true, ...page});
  expect(tabsOf(state)).toEqual([['agent'], [browserTabId]]);
});

test('an agent navigation selects the Browser tab', () => {
  let state = receivedBrowser(attached(), {session: 's', reveal: false, ...page});
  const {tabs} = state.session;
  const withFile = moveTab(openTab(tabs, fileTabId('a.go')), fileTabId('a.go'), findPane(tabs.root, browserTabId).id);
  state = {...state, session: {...state.session, tabs: withFile}};
  expect(activeTabs(state)).toEqual(['agent', fileTabId('a.go')]);
  state = receivedBrowser(state, {session: 's', reveal: true, ...page});
  expect(activeTabs(state)).toEqual(['agent', browserTabId]);
});

test('another session\'s browser opens its tab when that session is attached', () => {
  let state = receivedBrowser(attached(), {session: 'other', reveal: true, ...page});
  expect(tabsOf(state)).toEqual([['agent']]);
  state = selectSession(state, 'other');
  expect(state.session.id).toBe('other');
  expect(tabsOf(state)).toEqual([['agent'], [browserTabId]]);
  state = receivedWorkspaces(state, workspaces);
  expect(tabsOf(state)).toEqual([['agent'], [browserTabId]]);
});
