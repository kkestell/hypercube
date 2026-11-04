import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {clickControl, element, find, findAll, mount, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import type {Change, Changes} from '../changes/types.ts';
import {emptySession} from '../sessions/selection.ts';
import type {Merge} from '../sessions/types.ts';
import {createStore} from '../store.ts';
import {RightPanel} from './right-panel.tsx';

afterEach(() => { mock.restore(); unmountAll(); });

const attribute = (root: ParentNode, selector: string, name: string) => element(root, selector, Element).getAttribute(name);

test('the right sidebar offers Files and Changes, initially Files', () => {
  const store = createStore(testState({workbench: {...testState().workbench, rightPanel: false}}));
  const root = mount(store, <RightPanel />);
  const tabs = findAll(root, '[role="tab"]');
  expect(tabs.map(tab => tab.title)).toEqual(['Files', 'Changes', 'Search']);
  expect(tabs.map(tab => tab.getAttribute('aria-selected'))).toEqual(['true', 'false', 'false']);
  act(() => clickControl(tabs[1]));
  expect(store.get().workbench.rightTab).toBe('changes');
});

test('side-panel tabs support arrow keys, Home, and End, retaining both views', async () => {
  const store = createStore(testState({workbench: {...testState().workbench, rightPanel: false}}));
  const root = mount(store, <RightPanel />);
  const tabs = findAll(root, '[role="tab"]');
  const panels = findAll(root, '[role="tabpanel"]');
  act(() => tabs[0].focus());
  const steps: [string, number][] = [['ArrowRight', 1], ['ArrowRight', 2], ['ArrowRight', 0], ['ArrowLeft', 2], ['Home', 0], ['End', 2]];
  for (const [key, index] of steps) {
    act(() => document.activeElement?.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true})));
    await act(() => new Promise(resolve => setTimeout(resolve)));
    expect(document.activeElement === tabs[index]).toBe(true);
    expect(tabs[index].getAttribute('aria-selected')).toBe('true');
    expect(panels[index].hidden).toBe(false);
    expect(panels.filter((_, i) => i !== index).every(panel => panel.hidden)).toBe(true);
    expect(tabs[index].getAttribute('aria-controls')).toBe(panels[index].id);
    expect(panels[index].getAttribute('aria-labelledby')).toBe(tabs[index].id);
  }
  expect(findAll(root, '[role="tabpanel"]').every((panel, i) => panel === panels[i])).toBe(true);
});

const files: Change[] = [
  {path: 'foo.py', status: 'modified', added: 4, removed: 3},
  {path: 'utils/helpers.py', status: 'untracked', added: 6, removed: 0},
  {path: 'README.md', status: 'deleted', added: 0, removed: 3},
];

function changes(data: Changes = {branch: 'feature', files}, merge?: Merge) {
  stubFetch(url => url.startsWith('diff?') ? new Response('patch') : Response.json(data));
  const store = createStore(testState({
    workbench: {...testState().workbench, rightTab: 'changes'}, selected: 'a', session: {...emptySession('a'), changes: data},
    workspaces: [testWorkspace({id: 'w', sessions: [testSession({id: 'a', merge})]})],
  }));
  return {store, root: mount(store, <RightPanel />)};
}

test.each<[string, Changes]>([
  ['file list', {branch: 'feature', files}],
  ['empty list', {branch: 'feature', files: []}],
  ['outside Git', null],
  ['missing folder', {missing: true}],
  ['error', {error: 'Cannot read repository changes.'}],
])('Changes panel follows its selected tab in native tab order: %s', (_, data) => {
  const {root} = changes(data);
  const stops = findAll(root, '[role="tablist"], [role="tabpanel"]')
    .filter(element => !element.hidden && element.tabIndex === 0);
  expect(stops.map(element => element.getAttribute('role'))).toEqual(['tablist', 'tabpanel']);
  const [tab, panel] = stops;
  expect(panel.getAttribute('tabindex')).toBe('0');
  expect(attribute(tab, '[aria-selected=true]', 'aria-controls')).toBe(panel.id);
  tab.focus();
  expect(tab.contains(document.activeElement)).toBe(true);
  panel.focus();
  expect(document.activeElement === panel).toBe(true);
});

test('Changes rows show status, name, folder, counts, and open the selected diff', () => {
  const {root, store} = changes();
  const rows = findAll(root, '.changes-panel-file');
  expect(rows.map(row => row.title)).toEqual(['foo.py • Modified', 'utils/helpers.py • Untracked', 'README.md • Deleted']);
  expect(rows.map(row => attribute(row, '.changes-panel-status', 'aria-label'))).toEqual(['Modified', 'Untracked', 'Deleted']);
  expect(find(rows[1], '.changes-panel-name').textContent).toBe('helpers.py');
  expect(rows[1].title).toContain('utils/helpers.py');
  expect(rows[1].querySelector('.changes-panel-description')).toBeNull();
  expect(attribute(rows[0], '.changes-panel-stat', 'aria-label')).toBe('4 added, 3 removed');
  expect(find(rows[0], '.changes-panel-stat').textContent).toBe('+4−3');
  expect(root.querySelector('.changes-panel-ahead')).toBeNull();
  act(() => clickControl(rows[1]));
  expect(store.get().session?.tabs.preview).toBe('diff:utils/helpers.py');
  expect(rows[1].getAttribute('aria-current')).toBe('true');
  expect(rows[0].hasAttribute('aria-current')).toBe(false);
});

test('only unmerged commits show an ahead count with the merge sentence as its label', () => {
  const {root} = changes({branch: 'feature', files: []}, {branch: 'feature', target: 'main', unmerged: 2, committed: true, uncommitted: 0});
  const ahead = find(root, '.changes-panel-ahead');
  expect(ahead.textContent).toBe('2');
  expect(ahead.getAttribute('role')).toBe('img');
  expect(ahead.getAttribute('aria-label')).toBe('2 commits not merged into main');
  expect(ahead.title).toBe('2 commits not merged into main');
});

test('outside Git shows nothing, and missing folders replace the earlier list with a message', () => {
  expect(changes(null).root.querySelector('.changes-panel')).toBeNull();
  const {root, store} = changes();
  act(() => store.update(state => state.session ? {...state, session: {...state.session, changes: {missing: true}}} : state));
  expect(root.querySelector('.changes-panel-list')).toBeNull();
  expect(find(root, '.changes-panel-message').textContent).toBe('The folder no longer exists.');
});
