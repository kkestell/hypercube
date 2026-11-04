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
  clickControl(file);
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

test('removing the focused file returns focus to a remaining row', () => {
  const {store, root} = setup();
  act(() => find(root, '[data-file="README.md"]').focus());
  store.update(s => s.session ? {...s, session: {...s.session, files: {...s.session.files, listings: {'': [{name: 'src', dir: true}]}}}} : s);
  expect(document.activeElement === root.querySelector('[data-dir]')).toBe(true);
  expect(root.querySelectorAll('[role=treeitem][tabindex="0"]')).toHaveLength(1);
});

test('tree row menus copy absolute and relative paths', () => {
  const copy = spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
  try {
    const {root} = setup();
    const row = find(root, '[data-file="README.md"]');
    for (const [label, expected] of [['Copy Path', '/repo/README.md'], ['Copy Relative Path', 'README.md']]) {
      act(() => row.dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})));
      const item = [...document.querySelectorAll<HTMLElement>('[role=menuitem]')].find(item => item.textContent === label);
      if (!item) throw new Error(`No ${label} item`);
      clickControl(item);
      expect(copy).toHaveBeenLastCalledWith(expected);
    }
  } finally { copy.mockRestore(); }
});
