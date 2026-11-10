import {afterEach, expect, mock, spyOn, test} from 'bun:test';
import {act} from 'react';
import {find, findAll, mount, stubFetch, testBridge, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import type {ChangesResult} from '../changes/types.ts';
import {receivedWorkspaces} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {FilesPanel} from './files-panel.tsx';

afterEach(() => { unmountAll(); mock.restore(); delete globalThis.hypercube; });

function setup() {
  const listings = {'': [{name: 'src', dir: true}], src: [{name: 'file with spaces.go', dir: false}]};
  const changes: ChangesResult = {branch: 'main', files: [{path: 'src/file with spaces.go', status: 'modified'}]};
  const fetches = stubFetch(url => {
    const {pathname} = new URL(url, 'http://x/');
    if (pathname === '/files/tree') return Response.json(listings);
    if (pathname === '/changes') return Response.json(changes);
    return new Response('');
  });
  const store = createStore(receivedWorkspaces(testState({selected: 's'}), [testWorkspace({id: 'w', sessions: [
    testSession({id: 's', dir: '/worktree with spaces'}),
  ]})]));
  store.update(state => ({...state, editor: {...state.editor!, changes,
    files: {...state.editor!.files, listings, expanded: {src: true}}}}));
  const root = mount(store, <FilesPanel />);
  return {root, fetches};
}

const key = (target: HTMLElement, key: string, shiftKey = false) => act(() => {
  target.dispatchEvent(new KeyboardEvent('keydown', {key, shiftKey, bubbles: true, cancelable: true}));
});
const item = (label: string) => findAll(document, '[role=menuitem]').find(item => item.textContent === label)!;

test.each([
  ['darwin', 'Show in Finder'],
  ['linux', 'Show in Files'],
])('on %s the file menu reveals a file through the desktop app', async (platform, label) => {
  const open = mock(async (_app: string, _path: string) => '');
  globalThis.hypercube = testBridge({platform, open});
  const {root, fetches} = setup();
  const row = find(root, '[role="treeitem"][aria-label="file with spaces.go, Modified"]');
  act(() => row.focus());
  key(row, 'F10', true);
  expect(item(label)).toBeDefined();
  expect(item('Copy Name')).toBeDefined();
  expect(item('Open Changes')).toBeDefined();
  act(() => item(label).focus());
  await act(async () => key(item(label), 'Enter'));
  expect(open.mock.calls).toEqual([['reveal', '/worktree with spaces/src/file with spaces.go']]);
  expect(fetches.mock.calls.filter(([url]) => url.startsWith('open'))).toEqual([]);
  expect(document.querySelector('[role=menu]')).toBeNull();
  expect(document.activeElement).toBe(row);
});

test('the file menu has no reveal item in a browser', () => {
  const {root} = setup();
  const row = find(root, '[role="treeitem"][aria-label="src, Modified"]');
  act(() => row.focus());
  key(row, 'F10', true);
  expect(findAll(document, '[role=menuitem]').map(item => item.textContent)).toEqual(['Copy Name', 'Copy Path', 'Copy Relative Path']);
});

test('file menu copies names and paths, and Escape returns focus', async () => {
  const copy = spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
  const {root} = setup();
  const row = find(root, '[role="treeitem"][aria-label="file with spaces.go, Modified"]');
  const open = () => {
    act(() => row.focus());
    key(row, 'ContextMenu');
  };
  for (const [label, value] of [
    ['Copy Name', 'file with spaces.go'],
    ['Copy Path', '/worktree with spaces/src/file with spaces.go'],
    ['Copy Relative Path', 'src/file with spaces.go'],
  ]) {
    open();
    act(() => item(label).focus());
    await act(async () => key(item(label), 'Enter'));
    expect(copy).toHaveBeenLastCalledWith(value);
    expect(document.activeElement).toBe(row);
  }
  open();
  await act(async () => {
    key(find(document, '[role=menu]'), 'Escape');
    // Radix returns focus after the menu's focus scope unmounts.
    await new Promise<void>(resolve => setTimeout(resolve));
  });
  expect(document.querySelector('[role=menu]')).toBeNull();
  expect(document.activeElement).toBe(row);
});
