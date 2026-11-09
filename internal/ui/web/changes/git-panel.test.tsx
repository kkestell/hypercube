import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {find, mount, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {emptyEditor} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {statusColor} from './changes.ts';
import {GitPanel} from './git-panel.tsx';
import {MergeButton} from './merge-button.tsx';
import type {Changes} from './types.ts';

afterEach(() => { unmountAll(); mock.restore(); });

const changes: Changes = {branch: 'main', files: [
  {path: 'src/modified.go', status: 'modified', added: 10, removed: 20},
  {path: 'added.go', status: 'added', added: 0, removed: 0},
  {path: 'gone.go', status: 'deleted', added: 0, removed: 2},
  {path: 'new.png', status: 'untracked'},
]};

test('Git rows show status colors and supplied counts and open diffs from the path or counts', async () => {
  const editor = {...emptyEditor({session: 's'}), changes};
  const store = createStore(testState({editor}));
  const root = mount(store, <GitPanel />);
  const buttons = [...root.querySelectorAll<HTMLButtonElement>('li button')];
  expect(buttons.map(button => button.getAttribute('aria-label'))).toEqual([
    'src/modified.go, Modified, 10 added, 20 removed',
    'added.go, Added, 0 added, 0 removed',
    'gone.go, Deleted, 0 added, 2 removed',
    'new.png, Untracked',
  ]);
  expect(buttons.map(button => button.textContent)).toEqual(['src/modified.go+10−20', 'added.go+0−0', 'gone.go+0−2', 'new.png']);
  for (const [i, change] of changes.files.entries()) {
    const button = buttons[i];
    expect(button.tabIndex).toBe(0);
    expect(button.title).toBe(change.path);
    expect(button.firstElementChild?.className).toContain(statusColor[change.status]);
  }
  expect(find(root, '[title="gone.go"] .line-through').textContent).toBe('gone.go');
  act(() => buttons[0].focus());
  expect(document.activeElement).toBe(buttons[0]);
  await act(async () => buttons[0].click());
  expect(store.get().editor?.tabs.preview).toBe('diff:src/modified.go');
  await act(async () => find(buttons[1], '[aria-hidden="true"] span').click());
  expect(store.get().editor?.tabs.preview).toBe('diff:added.go');
  await act(async () => buttons[3].click());
  expect(store.get().editor?.tabs.preview).toBe('diff:new.png');
});

test('Git activation and target changes refresh immediately while retaining cached rows during a read', async () => {
  const store = createStore(testState({editor: {...emptyEditor({session: 's'}), changes}}));
  const replies: Array<(response: Response) => void> = [];
  const fetches = stubFetch(() => new Promise<Response>(resolve => replies.push(resolve)));
  const root = mount(store, <GitPanel />);
  expect(fetches).not.toHaveBeenCalled();
  act(() => store.update(state => ({...state, workbench: {...state.workbench, rightTab: 'git'}})));
  expect(fetches).toHaveBeenCalledTimes(1);
  expect(root.querySelectorAll('li')).toHaveLength(4);
  await act(async () => replies[0](Response.json({branch: 'main', files: []})));
  expect(root.textContent).toContain('No uncommitted changes.');
  expect(root.querySelector('li')).toBeNull();
  act(() => store.update(state => ({...state, editor: emptyEditor({workspace: 'w'})})));
  expect(fetches).toHaveBeenCalledTimes(2);
  expect(new URL(fetches.mock.calls[1][0], 'http://x/').searchParams.get('workspace')).toBe('w');
  expect(root.textContent).toContain('No Git changes available.');
  await act(async () => replies[1](new Response('gone', {status: 404})));
  expect(root.textContent).toContain('This folder no longer exists.');
  act(() => store.update(state => ({...state, workbench: {...state.workbench, rightPanel: false}})));
  expect(fetches).toHaveBeenCalledTimes(2);
  act(() => store.update(state => ({...state, workbench: {...state.workbench, rightPanel: true}})));
  expect(fetches).toHaveBeenCalledTimes(3);
  await act(async () => replies[2](Response.json(changes)));
  expect([...root.querySelectorAll<HTMLButtonElement>('li button')].map(button => button.title)).toEqual(['added.go', 'gone.go', 'new.png', 'src/modified.go']);
});

test.each([
  [null, 'No Git changes available.'],
  [{branch: 'main', files: []}, 'No uncommitted changes.'],
  [{missing: true}, 'This folder no longer exists.'],
  [{error: 'permission denied'}, 'permission denied'],
] satisfies Array<[Changes, string]>)('Git replaces old rows for result %p', (result, message) => {
  const store = createStore(testState({editor: {...emptyEditor({session: 's'}), changes}}));
  const root = mount(store, <GitPanel />);
  expect(root.querySelectorAll('li')).toHaveLength(4);
  act(() => store.update(state => ({...state, editor: {...state.editor!, changes: result}})));
  expect(root.querySelector('[role="tabpanel"]')?.textContent ?? root.textContent).toContain(message);
  expect(root.querySelector('li')).toBeNull();
});

test('Git without a selection prompts for a session or workspace and does not read', () => {
  const fetches = stubFetch(() => Response.json(null));
  const root = mount(createStore(testState()), <GitPanel />);
  expect(root.textContent).toBe('Select a session or workspace to see its changes.');
  expect(fetches).not.toHaveBeenCalled();
});

test('the comparison tabs appear only for a session with a worktree', () => {
  stubFetch(() => Response.json(null));
  const show = (worktree: boolean) => {
    const session = testSession({id: 's', worktree});
    return mount(createStore(testState({selected: 's', workspaces: [testWorkspace({id: 'w', sessions: [session]})], editor: {...emptyEditor({session: 's'}), changes}, workbench: {...testState().workbench, rightTab: 'git'}})), <GitPanel />);
  };
  expect(show(false).querySelector('[role="tablist"]')).toBeNull();
  expect(show(false).querySelectorAll('li')).toHaveLength(4);
  expect(show(true).querySelectorAll('[role="tab"]')).toHaveLength(2);
});

test('branch selection loads independent rows and the response target, then opens a matching diff', async () => {
  const fetches = stubFetch(() => Response.json({branch: 'feature', target: 'release', files: [{path: 'committed.go', status: 'added', added: 1, removed: 0}]}));
  const session = testSession({id: 's', worktree: true, merge: {branch: 'feature', target: 'main', unmerged: 1, committed: true, uncommitted: 4}});
  const store = createStore(testState({selected: 's', workspaces: [testWorkspace({id: 'w', sessions: [session]})], editor: {...emptyEditor({session: 's'}), changes: {error: 'uncommitted failed'}}, workbench: {...testState().workbench, rightTab: 'git'}}));
  const root = mount(store, <GitPanel />);
  await act(async () => find(root, '[role="tab"][data-state="inactive"]').dispatchEvent(new MouseEvent('mousedown', {bubbles: true, button: 0, ctrlKey: false})));
  expect(fetches.mock.calls.some(([url]) => new URL(url, 'http://x/').searchParams.get('comparison') === 'branch')).toBe(true);
  expect(find(root, '[role="tabpanel"][data-state="active"]').textContent).toContain('1 commit ahead of release');
  expect(root.textContent).not.toContain('preview');
  await act(async () => find(root, 'li button').click());
  expect(store.get().editor?.tabs.preview).toBe('branch-diff:committed.go');
});

test.each([
  [{branch: 'feature', target: 'main', files: []}, 'No branch changes.'],
  [{error: 'no common ancestor'}, 'no common ancestor'],
] satisfies Array<[Changes, string]>)('branch result stays visible independently of an uncommitted error: %p', (branchChanges, message) => {
  const session = testSession({id: 's', worktree: true});
  const store = createStore(testState({selected: 's', workspaces: [testWorkspace({id: 'w', sessions: [session]})],
    editor: {...emptyEditor({session: 's'}), changes: {error: 'uncommitted failed'}, branchChanges}}));
  const root = mount(store, <GitPanel />);
  expect(root.textContent).toContain('uncommitted failed');
  act(() => find(root, '[role="tab"][data-state="inactive"]').dispatchEvent(new MouseEvent('mousedown', {bubbles: true, button: 0, ctrlKey: false})));
  expect(find(root, '[role="tabpanel"][data-state="active"]').textContent).toContain(message);
  expect(root.textContent).not.toContain('uncommitted failed');
});

test('merge sits below both comparisons and asks the agent to merge into the checkout', async () => {
  const fetches = stubFetch(() => new Response(''));
  const session = testSession({id: 's', worktree: true, merge: {branch: 'feature', target: 'main', unmerged: 2, committed: true, uncommitted: 0}});
  const store = createStore(testState({selected: 's', workspaces: [testWorkspace({id: 'w', path: '/repo', sessions: [session]})], editor: {...emptyEditor({session: 's'}), changes}}));
  const root = mount(store, <GitPanel />);
  const selector = 'button[title="Ask the agent to merge this branch into main"]';
  find(root, selector);
  act(() => find(root, '[role="tab"][data-state="inactive"]').dispatchEvent(new MouseEvent('mousedown', {bubbles: true, button: 0, ctrlKey: false})));
  const button = find(root, selector);
  expect(button.textContent).toBe('Merge into main');
  expect(button.querySelector('svg')).not.toBeNull();
  await act(async () => button.click());
  expect(fetches).toHaveBeenCalledTimes(1);
  const [url, init] = fetches.mock.calls[0];
  const params = new URL(url, 'http://x/').searchParams;
  expect(params.get('action')).toBe('input');
  expect(params.get('id')).toBe('s');
  expect(JSON.parse(init!.body as string)).toEqual({
    text: 'Commit any remaining work, then merge your branch into main in the checkout at /repo, resolving any conflicts, and report the result.',
    images: [],
  });
});

test.each([
  {unmerged: 0, committed: false, uncommitted: 0},
  {unmerged: 0, committed: true, uncommitted: 0},
  {unmerged: 0, committed: false, uncommitted: 1},
  {unmerged: 1, committed: true, uncommitted: 0, error: 'x'},
])('merge is absent without unmerged commits or on error: %p', merge => {
  const session = testSession({id: 's', worktree: true, merge: {branch: 'feature', target: 'main', ...merge}});
  const store = createStore(testState({workspaces: [testWorkspace({id: 'w', sessions: [session]})]}));
  expect(mount(store, <MergeButton session={session} />).querySelector('button')).toBeNull();
});
