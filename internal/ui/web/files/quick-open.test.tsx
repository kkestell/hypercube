import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {find, mount, stubFetch, testBridge, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {receivedWorkspaces, selectWorkspace} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {QuickOpen} from './quick-open.tsx';

afterEach(() => {
  mock.restore();
  unmountAll();
  delete globalThis.hypercube;
});

// swallowed says whether Quick Open keeps a key press from reaching the menu.
async function swallowed(platform: string, modifier: 'metaKey' | 'ctrlKey') {
  globalThis.hypercube = testBridge({platform});
  stubFetch(() => Response.json([]));
  const store = createStore(receivedWorkspaces(testState({selected: 'a'}), [testWorkspace({id: 'w', sessions: [testSession({id: 'a'})]})]));
  mount(store, <QuickOpen open onOpenChange={() => {}} />);
  await act(async () => {});
  const event = new KeyboardEvent('keydown', {key: 'p', [modifier]: true, bubbles: true, cancelable: true});
  act(() => { find(document, '[aria-label="Search files"]').dispatchEvent(event); });
  return event.defaultPrevented;
}

test('Quick Open keeps the shortcut that opened it from the menu', async () => {
  expect(await swallowed('darwin', 'metaKey')).toBe(true);
  unmountAll();
  expect(await swallowed('linux', 'ctrlKey')).toBe(true);
});


test('Quick Open lists files for the selected workspace', async () => {
  const requests = stubFetch(() => Response.json(['main.go']));
  const store = createStore(receivedWorkspaces(testState(), [testWorkspace({id: 'w'})]));
  store.update(state => selectWorkspace(state, 'w'));
  mount(store, <QuickOpen open onOpenChange={() => {}} />);
  await act(async () => {});
  const url = new URL(requests.mock.calls[0][0], 'http://localhost/');
  expect(url.pathname).toBe('/mention-files');
  expect(Object.fromEntries(url.searchParams)).toEqual({workspace: 'w'});
  expect(document.body.textContent).toContain('main.go');
});

test('Quick Open announces a failed file listing', async () => {
  stubFetch(() => new Response('permission denied', {status: 403}));
  const store = createStore(receivedWorkspaces(testState(), [testWorkspace({id: 'w'})]));
  store.update(state => selectWorkspace(state, 'w'));
  mount(store, <QuickOpen open onOpenChange={() => {}} />);
  await act(async () => {});
  expect(find(document, '[role="alert"]').textContent).toBe('Could not list files: permission denied');
});
