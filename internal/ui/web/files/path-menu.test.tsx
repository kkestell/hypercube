import {afterEach, expect, mock, spyOn, test} from 'bun:test';
import {act} from 'react';
import {find, findAll, mount, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import type {ChangesResult} from '../changes/types.ts';
import {receivedWorkspaces} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {FilesPanel} from './files-panel.tsx';

afterEach(() => { unmountAll(); mock.restore(); });

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
  store.update(state => ({...state, session: {...state.session!, changes,
    files: {...state.session!.files, listings, expanded: {src: true}}}}));
  const root = mount(store, <FilesPanel />);
  return {root, fetches};
}

const key = (target: HTMLElement, key: string, shiftKey = false) => act(() => {
  target.dispatchEvent(new KeyboardEvent('keydown', {key, shiftKey, bubbles: true, cancelable: true}));
});
const item = (label: string) => findAll(document, '[role=menuitem]').find(item => item.textContent === label)!;

test.each(['src', 'src/file with spaces.go'])('Show in Finder reveals %s from a keyboard-opened file menu', async path => {
  const {root, fetches} = setup();
  const row = find(root, `[data-path="${path}"]`);
  act(() => row.focus());
  key(row, 'F10', true);
  expect(item('Show in Finder')).toBeDefined();
  expect(item('Copy Name')).toBeDefined();
  expect(!!item('Open Changes')).toBe(path !== 'src');
  act(() => item('Show in Finder').focus());
  await act(async () => key(item('Show in Finder'), 'Enter'));
  const opened = fetches.mock.calls.filter(([url]) => url.startsWith('open?'));
  expect(opened).toHaveLength(1);
  expect(Object.fromEntries(new URL(opened[0][0], 'http://x/').searchParams)).toEqual({
    kind: 'path', id: 's', app: 'finder', path,
  });
  expect(opened[0][1]?.method).toBe('POST');
  expect(document.querySelector('[role=menu]')).toBeNull();
  expect(document.activeElement).toBe(row);
});

test('file menu copies names and paths, and Escape returns focus', async () => {
  const copy = spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
  const {root} = setup();
  const row = find(root, '[data-path="src/file with spaces.go"]');
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
