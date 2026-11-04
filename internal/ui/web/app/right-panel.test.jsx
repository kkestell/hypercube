import {afterEach, expect, spyOn, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {createStore} from '../store.js';
import {emptySession} from '../sessions/selection.js';
import {RightPanel} from './right-panel.jsx';

let fetchMock;
afterEach(() => { unmountAll(); fetchMock?.mockRestore(); });

test('the right sidebar offers Files and Changes, initially Files', () => {
  const store = createStore({workbench: {rightTab: 'files'}, session: null, workspaces: []});
  const root = mount(store, <RightPanel />);
  const tabs = [...root.querySelectorAll('[role="tab"]')];
  expect(tabs.map(tab => tab.title)).toEqual(['Files', 'Changes']);
  expect(tabs.map(tab => tab.getAttribute('aria-selected'))).toEqual(['true', 'false']);
  act(() => tabs[1].click());
  expect(store.get().workbench.rightTab).toBe('changes');
});

test('side-panel tabs support arrow keys, Home, and End, retaining both views', () => {
  const store = createStore({workbench: {rightTab: 'files'}, session: null, workspaces: []});
  const root = mount(store, <RightPanel />);
  const tabs = [...root.querySelectorAll('[role="tab"]')];
  const panels = [...root.querySelectorAll('[role="tabpanel"]')];
  tabs[0].focus();
  for (const [key, index] of [['ArrowRight', 1], ['ArrowRight', 0], ['ArrowLeft', 1], ['Home', 0], ['End', 1]]) {
    act(() => document.activeElement.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true})));
    expect(document.activeElement).toBe(tabs[index]);
    expect(tabs[index].getAttribute('aria-selected')).toBe('true');
    expect(panels[index].hidden).toBe(false);
    expect(panels[1 - index].hidden).toBe(true);
    expect(tabs[index].getAttribute('aria-controls')).toBe(panels[index].id);
    expect(panels[index].getAttribute('aria-labelledby')).toBe(tabs[index].id);
  }
  expect([...root.querySelectorAll('[role="tabpanel"]')]).toEqual(panels);
});

const files = [
  {path: 'foo.py', status: 'modified', added: 4, removed: 3},
  {path: 'utils/helpers.py', status: 'untracked', added: 6, removed: 0},
  {path: 'README.md', status: 'deleted', added: 0, removed: 3},
];

function changes(data = {branch: 'feature', files}, merge = null) {
  fetchMock = spyOn(globalThis, 'fetch').mockImplementation(async url => String(url).startsWith('diff?') ? new Response('patch') : Response.json(data));
  const store = createStore({workbench: {rightTab: 'changes'}, selected: 'a', session: {...emptySession('a'), changes: data},
    workspaces: [{id: 'w', sessions: [{id: 'a', children: [], merge}]}]});
  return {store, root: mount(store, <RightPanel />)};
}

test.each([
  ['file list', {branch: 'feature', files}],
  ['empty list', {branch: 'feature', files: []}],
  ['outside Git', null],
  ['missing folder', {missing: true}],
  ['error', {error: 'Cannot read repository changes.'}],
])('Changes panel follows its selected tab in native tab order: %s', (_, data) => {
  const {root} = changes(data);
  const stops = [...root.querySelectorAll('[role="tab"], [role="tabpanel"]')]
    .filter(element => !element.hidden && element.tabIndex === 0);
  expect(stops.map(element => element.getAttribute('role'))).toEqual(['tab', 'tabpanel']);
  const [tab, panel] = stops;
  expect(panel.getAttribute('tabindex')).toBe('0');
  expect(tab.getAttribute('aria-controls')).toBe(panel.id);
  tab.focus();
  expect(document.activeElement).toBe(tab);
  panel.focus();
  expect(document.activeElement).toBe(panel);
});

test('Changes rows show status, name, folder, counts, and open the selected diff', () => {
  const {root, store} = changes();
  const rows = [...root.querySelectorAll('.changes-panel-file')];
  expect(rows.map(row => row.title)).toEqual(['foo.py • Modified', 'utils/helpers.py • Untracked', 'README.md • Deleted']);
  expect(rows.map(row => row.querySelector('.changes-panel-status').getAttribute('aria-label'))).toEqual(['Modified', 'Untracked', 'Deleted']);
  expect(rows[1].querySelector('.changes-panel-name').textContent).toBe('helpers.py');
  expect(rows[1].querySelector('.changes-panel-description').textContent).toBe('utils');
  expect(rows[0].querySelector('.changes-panel-stat').getAttribute('aria-label')).toBe('4 added, 3 removed');
  expect(rows[0].querySelector('.changes-panel-stat').textContent).toBe('+4−3');
  expect(root.querySelector('.changes-panel-ahead')).toBeNull();
  act(() => rows[1].click());
  expect(store.get().session.tabs.preview).toBe('diff:utils/helpers.py');
  expect(rows[1].getAttribute('aria-current')).toBe('true');
  expect(rows[0].hasAttribute('aria-current')).toBe(false);
});

test('only unmerged commits show an ahead count with the merge sentence as its label', () => {
  const {root} = changes({branch: 'feature', files: []}, {unmerged: 2, target: 'main'});
  const ahead = root.querySelector('.changes-panel-ahead');
  expect(ahead.textContent).toBe('2');
  expect(ahead.getAttribute('role')).toBe('img');
  expect(ahead.getAttribute('aria-label')).toBe('2 commits not merged into main');
  expect(ahead.title).toBe('2 commits not merged into main');
});

test('outside Git shows nothing, and missing folders replace the earlier list with a message', () => {
  expect(changes(null).root.querySelector('.changes-panel')).toBeNull();
  const {root, store} = changes();
  act(() => store.update(state => ({...state, session: {...state.session, changes: {missing: true}}})));
  expect(root.querySelector('.changes-panel-list')).toBeNull();
  expect(root.querySelector('.changes-panel-message').textContent).toBe('The folder no longer exists.');
});
