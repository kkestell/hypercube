import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {clickControl, element, find, mount, setValue, stubFetch, unmountAll} from '../../test-render.tsx';
import {testState} from '../../test-state.ts';
import {emptySession} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {SearchPanel} from './search-panel.tsx';

afterEach(() => { unmountAll(); mock.restore(); });

test('search shows matching lines and opens the selected line', async () => {
  const fetches = stubFetch(url => url.startsWith('search-files?')
    ? Response.json([{path: 'src/a.go', lines: [{line: 3, parts: ['a ', 'MATCH', ' here']}]}]) : new Response('source'));
  const store = createStore(testState({session: emptySession('s'), workbench: {...testState().workbench, rightPanel: true, rightTab: 'search'}}));
  const root = mount(store, <SearchPanel />);
  const input = element(root, 'input', HTMLInputElement);
  expect(document.activeElement).toBe(input);
  act(() => { setValue(input, 'match'); input.dispatchEvent(new Event('input', {bubbles: true})); });
  await act(async () => {});
  expect(String(fetches.mock.calls[0]?.[0])).toContain('q=match');
  expect(find(root, 'h3').textContent).toBe('src/a.go');
  expect(find(root, 'mark').textContent).toBe('MATCH');
  act(() => clickControl(find(root, 'button')));
  expect(store.get().session?.files.open['src/a.go']?.lines).toEqual([{start: 3, end: 3}]);
});
