import {afterEach, expect, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {createStore} from '../store.js';
import {RightPanel} from './right-panel.jsx';

afterEach(unmountAll);

test('the right sidebar offers Files and Changes, initially Files', () => {
  const store = createStore({workbench: {rightTab: 'files'}, session: null, workspaces: []});
  const root = mount(store, <RightPanel />);
  expect([...root.querySelectorAll('vscode-tab-header')].map(tab => tab.title)).toEqual(['Files', 'Changes']);
  const tabs = root.querySelector('vscode-tabs');
  expect(tabs.getAttribute('selected-index')).toBe('0');
  act(() => tabs.dispatchEvent(new CustomEvent('vsc-tabs-select', {detail: {selectedIndex: 1}})));
  expect(store.get().workbench.rightTab).toBe('changes');
});

const merge = extra => ({branch: 'hypercube/session-a', target: 'main', unmerged: 0, committed: false, uncommitted: 0, ...extra});
const changes = shown => {
  const store = createStore({workbench: {rightTab: 'changes'}, selected: 'a', session: null,
    workspaces: [{id: 'w', sessions: [{id: 'a', children: [], ...shown}]}]});
  const root = mount(store, <RightPanel />);
  return [...root.querySelectorAll('.changes-panel p')].map(p => p.textContent);
};

test.each([
  [merge(), ['No commits yet', '0 uncommitted files']],
  [merge({unmerged: 1, committed: true, uncommitted: 1}), ['1 commit not merged into main', '1 uncommitted file']],
  [merge({unmerged: 3, committed: true, uncommitted: 2}), ['3 commits not merged into main', '2 uncommitted files']],
  [merge({committed: true}), ['Merged into main', '0 uncommitted files']],
  [merge({error: 'no such directory'}), ['Merge status unavailable: no such directory']],
])('Changes shows a worktree session\'s branch and %o', (m, want) => {
  expect(changes({worktree: true, merge: m})).toEqual(['hypercube/session-a', ...want]);
});

test('Changes is empty for a session in the checkout', () => {
  expect(changes({})).toEqual([]);
});
