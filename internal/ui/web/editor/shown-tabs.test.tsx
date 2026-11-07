import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {mount, stubFetch, unmountAll} from '../../test-render.tsx';
import {testState, unread} from '../../test-state.ts';
import {emptySession} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {EditorArea} from './editor-area.tsx';
import {diffTabId, fileTabId, openDiffTab, openFileTab, openRequestedFile, selectTab} from './editor.ts';
import {openTab} from './tab-layout.ts';

afterEach(() => { unmountAll(); mock.restore(); });

const patch = '@@ -1 +1 @@\n-old\n+new\n';

test.each(['file', 'diff'] as const)('a restored unread %s loads when shown, without polling', async kind => {
  const session = emptySession('s');
  const id = kind === 'file' ? fileTabId('a.md') : diffTabId('a.md');
  session.tabs = openTab(session.tabs, id);
  if (kind === 'file') session.files.open['a.md'] = {...unread};
  else session.diffs['a.md'] = {text: null, message: null};
  const fetch = stubFetch(() => new Response(kind === 'file' ? '# Loaded file' : patch));
  const root = mount(createStore(testState({session})), <EditorArea />);
  await act(async () => {});
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(new URL(fetch.mock.calls[0][0], 'http://localhost').pathname).toBe(kind === 'file' ? '/file' : '/diff');
  expect(root.textContent).toContain(kind === 'file' ? 'Loaded file' : 'new');
});

test.each(['file', 'diff'] as const)('an inactive restored %s loads on selection', async kind => {
  const session = emptySession('s');
  const id = kind === 'file' ? fileTabId('a.md') : diffTabId('a.md');
  session.tabs = openTab(session.tabs, id);
  session.tabs = openTab(session.tabs, fileTabId('shown.md'));
  session.files.open['shown.md'] = {...unread, text: 'Already shown'};
  if (kind === 'file') session.files.open['a.md'] = {...unread};
  else session.diffs['a.md'] = {text: null, message: null};
  const fetch = stubFetch(() => new Response(kind === 'file' ? 'Selected file' : patch));
  const store = createStore(testState({session}));
  const root = mount(store, <EditorArea />);
  expect(fetch).toHaveBeenCalledTimes(0);
  await act(async () => { store.update(state => selectTab(state, 1, id)); });
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(root.textContent).toContain(kind === 'file' ? 'Selected file' : 'new');
});

test.each(['file', 'diff'] as const)('opening a new %s reads once and clicking it again refreshes', async kind => {
  const session = emptySession('s');
  session.tabs = openTab(session.tabs, fileTabId('shown.md'));
  session.files.open['shown.md'] = {...unread, text: 'Already shown'};
  const fetch = stubFetch(() => new Response(kind === 'file' ? 'Loaded file' : patch));
  const store = createStore(testState({session}));
  const root = mount(store, <EditorArea />);
  const open = () => kind === 'file' ? openFileTab(store, 'a.md') : openDiffTab(store, 'a.md');
  await act(async () => { open(); });
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(root.textContent).toContain(kind === 'file' ? 'Loaded file' : 'new');
  await act(async () => { open(); });
  expect(fetch).toHaveBeenCalledTimes(2);
});

test('an agent request refreshes an already loaded file', async () => {
  const session = emptySession('s');
  session.tabs = openTab(session.tabs, fileTabId('a.md'));
  session.files.open['a.md'] = {...unread, text: 'Before'};
  const fetch = stubFetch(() => new Response('After'));
  const store = createStore(testState({session}));
  const root = mount(store, <EditorArea />);
  expect(fetch).toHaveBeenCalledTimes(0);
  await act(async () => { openRequestedFile(store, {session: 's', path: 'a.md', lines: []}); });
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(root.textContent).toContain('After');
});

test.each([
  ['empty', {text: ''}],
  ['error', {message: 'Read failed'}],
  ['deleted', {deleted: true}],
  ['preview', {preview: {mime: 'image/png', url: '/image.png'}}],
])('a file with %s content waits for an explicit refresh', async (_label, content) => {
  const session = emptySession('s');
  session.tabs = openTab(session.tabs, fileTabId('a.md'));
  session.files.open['a.md'] = {...unread, ...content};
  const fetch = stubFetch(() => new Response('Refreshed'));
  const store = createStore(testState({session}));
  mount(store, <EditorArea />);
  expect(fetch).toHaveBeenCalledTimes(0);
  await act(async () => { openFileTab(store, 'a.md'); });
  expect(fetch).toHaveBeenCalledTimes(1);
});

test.each([{text: '', message: null}, {text: null, message: 'Read failed'}])('a loaded diff waits for an explicit refresh', async diff => {
  const session = emptySession('s');
  session.tabs = openTab(session.tabs, diffTabId('a.md'));
  session.diffs['a.md'] = diff;
  const fetch = stubFetch(() => new Response(patch));
  const store = createStore(testState({session}));
  mount(store, <EditorArea />);
  expect(fetch).toHaveBeenCalledTimes(0);
  await act(async () => { openDiffTab(store, 'a.md'); });
  expect(fetch).toHaveBeenCalledTimes(1);
});
