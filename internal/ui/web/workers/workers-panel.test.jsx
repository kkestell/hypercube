import {afterEach, expect, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {EditorArea} from '../editor/editor-area.jsx';
import {openWorkerTab} from '../editor/editor.js';
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
  selected: 's', unsent: null, drafts: {}, session: emptySession('s'),
});

test('worker cards show their reports and open transcripts', async () => {
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
  await act(() => new Promise(resolve => requestAnimationFrame(resolve)));
});

test('Open Transcript focuses an already active worker', async () => {
  const worker = {id: 'w1', name: 'worker', role: 'work'};
  const store = createStore(openWorkerTab(state([worker]), worker.id));
  const root = mount(store, <><WorkersPanel /><EditorArea agents={[lead, worker]} /></>);
  const button = root.querySelector('[title="Open Transcript"]');
  button.tabIndex = 0;
  button.focus();
  act(() => button.click());
  await act(() => new Promise(resolve => requestAnimationFrame(resolve)));
  expect(document.activeElement).toBe(root.querySelector('.composer-text'));
});
