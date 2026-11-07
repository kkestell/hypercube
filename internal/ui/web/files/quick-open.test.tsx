import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {find, mount, stubFetch, testBridge, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {receivedWorkspaces} from '../sessions/selection.ts';
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
  act(() => { find(document, '[cmdk-input]').dispatchEvent(event); });
  return event.defaultPrevented;
}

test('Quick Open keeps the shortcut that opened it from the menu', async () => {
  expect(await swallowed('darwin', 'metaKey')).toBe(true);
  unmountAll();
  expect(await swallowed('linux', 'ctrlKey')).toBe(true);
});
