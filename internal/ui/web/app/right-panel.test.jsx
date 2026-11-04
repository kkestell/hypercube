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
  expect([...root.querySelectorAll('vscode-tab-header')].map(tab => tab.title)).toEqual(['Files', 'Changes']);
  const tabs = root.querySelector('vscode-tabs');
  expect(tabs.getAttribute('selected-index')).toBe('0');
  act(() => tabs.dispatchEvent(new CustomEvent('vsc-tabs-select', {detail: {selectedIndex: 1}})));
  expect(store.get().workbench.rightTab).toBe('changes');
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
