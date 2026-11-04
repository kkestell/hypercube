import {afterEach, expect, spyOn, test} from 'bun:test';
import {act} from 'react';
import {clickControl, mount, setValue, unmountAll} from '../../test-render.jsx';
import {emptySession} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {SearchPanel} from './search-panel.jsx';

afterEach(unmountAll);

test('search shows matching lines and opens the selected line', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async url => String(url).startsWith('search-files?')
    ? Response.json([{path: 'src/a.go', lines: [{line: 3, parts: ['a ', 'MATCH', ' here']}]}]) : new Response('source'));
  try {
    const store = createStore({session: emptySession('s'), workspaces: [], workbench: {rightPanel: true, rightTab: 'search'}});
    const root = mount(store, <SearchPanel />);
    const input = root.querySelector('input');
    expect(document.activeElement).toBe(input);
    act(() => { setValue(input, 'match'); input.dispatchEvent(new Event('input', {bubbles: true})); });
    await act(async () => {});
    expect(fetches.mock.calls[0][0]).toContain('q=match');
    expect(root.querySelector('h3').textContent).toBe('src/a.go');
    expect(root.querySelector('mark').textContent).toBe('MATCH');
    act(() => clickControl(root.querySelector('button')));
    expect(store.get().session.files.open['src/a.go'].lines).toEqual([{start: 3, end: 3}]);
  } finally { fetches.mockRestore(); }
});
