import {afterEach, expect, spyOn, test} from 'bun:test';
import {act} from 'react';
import {clickControl, mount, unmountAll} from '../../test-render.jsx';
import {emptySession} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {FilesPanel} from './files-panel.jsx';

let fetches;
afterEach(() => { unmountAll(); fetches?.mockRestore(); });

function setup() {
  fetches = spyOn(globalThis, 'fetch').mockImplementation(async url => {
    const params = new URL(url, 'http://localhost/').searchParams;
    if (String(url).startsWith('file?')) return new Response('source');
    return Response.json(params.get('path') === 'src' ? [{name: 'main.go', dir: false}] : [{name: 'src', dir: true}, {name: 'README.md', dir: false}]);
  });
  const session = emptySession('s');
  session.files.listings[''] = [{name: 'src', dir: true}, {name: 'README.md', dir: false}];
  const store = createStore({session, workspaces: []});
  return {store, root: mount(store, <FilesPanel />)};
}

const press = (row, key) => act(() => row.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true})));

test('folders load on expansion and keyboard navigation opens and keeps file tabs', async () => {
  const {store, root} = setup();
  const folder = root.querySelector('[data-dir]');
  act(() => folder.focus());
  press(folder, 'ArrowRight');
  await act(async () => {});
  expect(folder.getAttribute('aria-expanded')).toBe('true');
  const file = root.querySelector('[data-file="src/main.go"]');
  expect(file).not.toBeNull();
  press(folder, 'ArrowRight');
  expect(document.activeElement === file).toBe(true);
  clickControl(file);
  expect(store.get().session.tabs.preview).toBe('file:src/main.go');
  act(() => file.dispatchEvent(new MouseEvent('dblclick', {bubbles: true})));
  expect(store.get().session.tabs.preview).toBeNull();
  press(file, 'ArrowLeft');
  expect(document.activeElement === folder).toBe(true);
  press(folder, 'ArrowLeft');
  expect(folder.getAttribute('aria-expanded')).toBe('false');
  press(folder, 'End');
  expect(document.activeElement === root.querySelector('[data-file="README.md"]')).toBe(true);
});

test('removing the focused file returns focus to a remaining row', () => {
  const {store, root} = setup();
  act(() => root.querySelector('[data-file="README.md"]').focus());
  store.update(s => ({...s, session: {...s.session, files: {...s.session.files, listings: {'': [{name: 'src', dir: true}]}}}}));
  expect(document.activeElement === root.querySelector('[data-dir]')).toBe(true);
  expect(root.querySelectorAll('[role=treeitem][tabindex="0"]')).toHaveLength(1);
});
