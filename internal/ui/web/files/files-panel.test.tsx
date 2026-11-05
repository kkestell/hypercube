import {afterEach, expect, mock, spyOn, test} from 'bun:test';
import {act} from 'react';
import {clickControl, find, mount, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {emptySession} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {FilesPanel} from './files-panel.tsx';

afterEach(() => { unmountAll(); mock.restore(); });

function setup() {
  stubFetch(async url => {
    const params = new URL(url, 'http://localhost/').searchParams;
    if (url.startsWith('file?')) return new Response('source');
    return Response.json(params.get('path') === 'src' ? [{name: 'main.go', dir: false}] : [{name: 'src', dir: true}, {name: 'README.md', dir: false}]);
  });
  const session = emptySession('s');
  session.files.listings[''] = [{name: 'src', dir: true}, {name: 'README.md', dir: false}];
  const store = createStore(testState({session, selected: 's', workspaces: [testWorkspace({id: 'w', sessions: [testSession({id: 's', dir: '/repo'})]})]}));
  return {store, root: mount(store, <FilesPanel />)};
}

const press = (row: HTMLElement, key: string) => act(() => row.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true})));

test('folders load on expansion and keyboard navigation opens and keeps file tabs', async () => {
  const {store, root} = setup();
  const folder = find(root, '[data-dir]');
  act(() => folder.focus());
  press(folder, 'ArrowRight');
  await act(async () => {});
  expect(folder.getAttribute('aria-expanded')).toBe('true');
  const file = find(root, '[data-file="src/main.go"]');
  press(folder, 'ArrowRight');
  expect(document.activeElement === file).toBe(true);
  await act(async () => { clickControl(file); });
  expect(store.get().session?.tabs.preview).toBe('file:src/main.go');
  act(() => file.dispatchEvent(new MouseEvent('dblclick', {bubbles: true})));
  expect(store.get().session?.tabs.preview).toBeNull();
  press(file, 'ArrowLeft');
  expect(document.activeElement === folder).toBe(true);
  press(folder, 'ArrowLeft');
  expect(folder.getAttribute('aria-expanded')).toBe('false');
  press(folder, 'End');
  expect(document.activeElement === root.querySelector('[data-file="README.md"]')).toBe(true);
});

test('removing the focused file returns focus to a remaining row', async () => {
  const {store, root} = setup();
  act(() => find(root, '[data-file="README.md"]').focus());
  act(() => store.update(s => s.session ? {...s, session: {...s.session, files: {...s.session.files, listings: {'': [{name: 'src', dir: true}]}}}} : s));
  expect(document.activeElement === root.querySelector('[data-dir]')).toBe(true);
  expect(root.querySelectorAll('[role=treeitem][tabindex="0"]')).toHaveLength(1);
});

test('tree row menus copy absolute and relative paths', async () => {
  const copy = spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
  try {
    const {root} = setup();
    const row = find(root, '[data-file="README.md"]');
    for (const [label, expected] of [['Copy Path', '/repo/README.md'], ['Copy Relative Path', 'README.md']]) {
      act(() => row.dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})));
      const item = [...document.querySelectorAll<HTMLElement>('[role=menuitem]')].find(item => item.textContent === label);
      if (!item) throw new Error(`No ${label} item`);
      await act(async () => { clickControl(item); });
      expect(copy).toHaveBeenLastCalledWith(expected);
    }
  } finally { copy.mockRestore(); }
});

function changed() {
  stubFetch(url => {
    if (url.startsWith('diff?')) return new Response('patch');
    if (url.startsWith('changes?')) return Response.json(changes);
    if (url.startsWith('file?')) return new Response('source');
    return Response.json(session.files.listings[new URL(url, 'http://x/').searchParams.get('path') ?? '']);
  });
  const session = emptySession('s');
  session.files.listings = {'': [{name: 'src', dir: true}, {name: 'README.md', dir: false}], src: [{name: 'main.go', dir: false}, {name: 'new.go', dir: false}]};
  session.files.expanded = {src: true};
  const changes = {branch: 'feature', files: [
    {path: 'src/main.go', status: 'modified' as const, added: 4, removed: 3},
    {path: 'src/new.go', status: 'untracked' as const, added: 6, removed: 0},
    {path: 'gone/old.go', status: 'deleted' as const, added: 0, removed: 2},
  ]};
  session.changes = changes;
  const store = createStore(testState({session, selected: 's', workspaces: [testWorkspace({id: 'w', sessions: [testSession({id: 's', dir: '/repo'})]})]}));
  return {store, root: mount(store, <FilesPanel />)};
}

test('changed files show their status and line counts, which open the diff', async () => {
  const {store, root} = changed();
  const main = find(root, '[data-file="src/main.go"]');
  expect(main.textContent).toBe('main.go, Modified');
  expect(main.title).toBe('src/main.go • Modified');
  expect(find(root, '[data-file="src/new.go"]').textContent).toBe('new.go, Untracked');
  expect(find(root, '[data-file="README.md"]').textContent).toBe('README.md');
  expect(find(root, '[data-path="src"]').textContent).toBe('src, Modified');
  const stat = find(main.parentElement!, '.files-panel-stat');
  expect(stat.textContent).toBe('+4−3');
  expect(stat.getAttribute('aria-label')).toBe('Open changes: 4 added, 3 removed');
  await act(async () => { clickControl(stat); });
  expect(store.get().session?.tabs.preview).toBe('diff:src/main.go');
  await act(async () => { clickControl(main); });
  expect(store.get().session?.tabs.preview).toBe('file:src/main.go');
});

test('deleted files stay in the tree, inside the folders that held them, and open their diff', async () => {
  const {store, root} = changed();
  const folder = find(root, '[data-path="gone"]');
  expect(folder.textContent).toBe('gone, Deleted');
  await act(async () => { clickControl(folder); });
  const file = find(root, '[data-file="gone/old.go"]');
  expect(file.textContent).toBe('old.go, Deleted');
  await act(async () => { clickControl(file); });
  expect(store.get().session?.tabs.preview).toBe('diff:gone/old.go');
  expect(store.get().session?.files.expanded.gone).toBe(true);
  expect(store.get().session?.files.listings).not.toHaveProperty('gone');
});

test('a changed file menu opens its changes', async () => {
  const {store, root} = changed();
  act(() => find(root, '[data-file="src/new.go"]').dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})));
  const item = [...document.querySelectorAll<HTMLElement>('[role=menuitem]')].find(item => item.textContent === 'Open Changes');
  if (!item) throw new Error('No Open Changes item');
  await act(async () => { clickControl(item); });
  expect(store.get().session?.tabs.preview).toBe('diff:src/new.go');
});
