import {afterEach, expect, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {emptySession} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {WorkersPanel, age} from './workers-panel.jsx';

afterEach(unmountAll);

test('a report\'s age reads as now, 3m, 2h, 1d', () => {
  const now = 10 * 86400000;
  expect([now - 30000, now - 3 * 60000, now - 2 * 3600000, now - 86400000].map(ms => age(ms, now))).toEqual(['now', '3m', '2h', '1d']);
});

const lead = {id: 'l', name: 'lead', role: 'lead'};
const state = agents => ({
  workspaces: [{id: 'w', name: 'repo', sessions: [{id: 's', running: true, agents: [lead, ...agents]}]}],
  selected: 's', unsent: null, session: emptySession('s'),
});

test('worker cards show their reports and open transcripts', () => {
  expect(mount(createStore(state([])), <WorkersPanel />).textContent).toBe('No workers yet.');
  unmountAll();
  const store = createStore(state([
    {id: 'w1', name: 'auditor', role: 'work', status: 'Reading the code', statusAt: Date.now()},
    {id: 'w2', name: 'builder', role: 'work'},
  ]));
  const root = mount(store, <WorkersPanel />);
  const [first, second] = root.querySelectorAll('article');
  expect(first.textContent).toBe('auditorReading the codenow');
  expect(second.textContent).toBe('builderNo update yet.');
  act(() => second.querySelector('[title="Open Transcript"]').click());
  expect(store.get().session.tabs.root.tabs).toEqual(['w2']);
});
