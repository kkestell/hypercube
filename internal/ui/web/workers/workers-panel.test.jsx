import {afterEach, expect, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {emptySession} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {WorkersPanel, age} from './workers-panel.jsx';
import {resizeWorkers} from './worker-sizing.js';

afterEach(unmountAll);

const lead = {id: 'l', name: 'lead', role: 'lead'};
const state = agents => ({
  workspaces: [{id: 'w', name: 'repo', sessions: [{id: 's', running: true, agents: [lead, ...agents]}]}],
  selected: 's', unsent: null, drafts: {}, session: emptySession('s'),
});
const worker = id => ({id, name: id, role: 'work', activity: 'idle'});

test('a report age reads as now, 3m, 2h, 1d', () => {
  const now = 10 * 86400000;
  expect([now - 30000, now - 3 * 60000, now - 2 * 3600000, now - 86400000].map(ms => age(ms, now))).toEqual(['now', '3m', '2h', '1d']);
});

test('workers toggle independently in place, preserving drafts and scroll without tabs', () => {
  const store = createStore(state([
    {...worker('auditor'), status: 'Reading the code', statusAt: Date.now(), attention: true},
    worker('builder'),
  ]));
  const root = mount(store, <WorkersPanel />);
  const [first, second] = root.querySelectorAll('section');
  const header = first.querySelector('button');
  expect(first.textContent).toContain('Reading the codenow');
  expect(first.querySelector('.status-dot')).not.toBeNull();
  expect(second.textContent).toContain('No update yet.');
  expect(header.getAttribute('aria-expanded')).toBe('false');
  act(() => header.click());
  const text = first.querySelector('textarea');
  text.value = 'keep this draft';
  act(() => text.dispatchEvent(new Event('input', {bubbles: true})));
  const view = first.querySelector('.transcript');
  Object.defineProperty(view, 'scrollHeight', {value: 1000});
  Object.defineProperty(view, 'clientHeight', {value: 300});
  for (const top of [900, 500]) {
    view.scrollTop = top;
    view.dispatchEvent(new Event('scroll'));
  }
  act(() => header.click());
  expect(first.querySelector('.agent-pane')).toBeNull();
  act(() => header.click());
  expect(first.querySelector('textarea').value).toBe('keep this draft');
  expect(first.querySelector('.transcript').scrollTop).toBe(500);
  expect(second.querySelector('.agent-pane')).toBeNull();
  expect(store.get().session.tabs.root).toBeNull();
});

test('dividers resize nearest expanded workers across collapsed cards and clean up on unmount', () => {
  const root = mount(createStore(state(['a', 'b', 'c', 'd'].map(worker))), <WorkersPanel />);
  const cards = [...root.querySelectorAll('section')];
  const toggle = index => act(() => cards[index].querySelector('button').click());
  toggle(0);
  expect(root.querySelector('[role="separator"]')).toBeNull();
  toggle(2);
  toggle(3);
  for (const [index, height] of [[0, 200], [2, 300], [3, 400]]) {
    cards[index].querySelector('.workers-panel-body').getBoundingClientRect = () => ({height});
  }
  act(() => root.querySelector('[role="separator"]').dispatchEvent(new PointerEvent('pointerdown', {button: 0, clientY: 100, bubbles: true})));
  act(() => window.dispatchEvent(new PointerEvent('pointermove', {clientY: 150})));
  expect(cards.map(card => card.style.flexGrow)).toEqual(['250', '', '250', '400']);
  act(() => window.dispatchEvent(new PointerEvent('pointermove', {clientY: 1000})));
  expect(cards.map(card => card.style.flexGrow)).toEqual(['380', '', '120', '400']);
  act(() => window.dispatchEvent(new PointerEvent('pointerup')));
  act(() => window.dispatchEvent(new PointerEvent('pointermove', {clientY: 0})));
  expect(cards[0].style.flexGrow).toBe('380');
  unmountAll();
  window.dispatchEvent(new PointerEvent('pointermove', {clientY: 100}));
});

test('sizing preserves the total and other expanded heights and bounds both ends', () => {
  expect(resizeWorkers({a: 200, c: 300, d: 400}, 'a', 'c', -500)).toEqual({a: 120, c: 380, d: 400});
  expect(resizeWorkers({a: 60, c: 60}, 'a', 'c', 100)).toEqual({a: 60, c: 60});
});
