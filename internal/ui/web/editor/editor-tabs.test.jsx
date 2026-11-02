import {afterEach, expect, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {emptySession} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {EditorArea} from './editor-area.jsx';
import {fileTabId, openWorkerTab} from './editor.js';
import {openTab} from './tab-layout.js';

afterEach(unmountAll);

const agents = [{id: 'lead', name: 'lead', role: 'lead', activity: 'idle'}, {id: 'w1', name: 'one', role: 'work', activity: 'busy'}, {id: 'w2', name: 'two', role: 'work', activity: 'idle'}];
const frame = () => act(() => new Promise(resolve => requestAnimationFrame(resolve)));

function setup(change = state => openWorkerTab(openWorkerTab(state, 'w1'), 'w2')) {
  const lead = document.createElement('form');
  lead.className = 'composer';
  lead.dataset.agent = 'lead';
  lead.append(document.createElement('textarea'));
  document.body.append(lead);
  const store = createStore(change({drafts: {}, unsent: null, session: emptySession('s')}));
  const root = mount(store, <EditorArea agents={agents} />);
  const tabs = () => [...root.querySelectorAll('[role="tab"]')];
  return {store, root, tabs, leadField: lead.firstChild};
}

test('closing a tab focuses the next one, and closing the last focuses the lead', async () => {
  const {root, tabs, leadField} = setup();
  expect(tabs().map(t => t.textContent)).toEqual(['one', 'two']);
  act(() => root.querySelector('[aria-label="Close two"]').click());
  await frame();
  expect(tabs().map(t => t.textContent)).toEqual(['one']);
  expect(document.activeElement === tabs()[0]).toBe(true);
  act(() => root.querySelector('[aria-label="Close one"]').click());
  await frame();
  expect(tabs().length).toBe(0);
  expect(document.activeElement === leadField).toBe(true);
});

test('the arrow keys, Home, and End select tabs and keep the focus on them', async () => {
  const {store, tabs} = setup();
  tabs()[1].focus();
  const press = async key => {
    act(() => document.activeElement.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true})));
    await frame();
  };
  await press('ArrowRight');
  expect(store.get().session.tabs.root.active).toBe('w1');
  expect(document.activeElement === tabs()[0]).toBe(true);
  await press('End');
  expect(document.activeElement === tabs()[1]).toBe(true);
  await press('Home');
  await press('ArrowLeft');
  expect(store.get().session.tabs.root.active).toBe('w2');
  expect(document.activeElement.getAttribute('aria-selected')).toBe('true');
});

test('a preview tab is italic until double-clicked, and a deleted file is struck through', () => {
  const unread = {text: null, message: null, etag: null, deleted: false};
  const {store, root} = setup(state => ({
    ...state,
    session: {
      ...state.session,
      tabs: {...[fileTabId('a.go'), fileTabId('b.go')].reduce(openTab, state.session.tabs), preview: fileTabId('b.go')},
      files: {...state.session.files, open: {'a.go': {...unread, deleted: true}, 'b.go': unread}},
    },
  }));
  const tab = path => root.querySelector(`[data-tab="${fileTabId(path)}"]`);
  expect(tab('a.go').classList.contains('editor-tabs-tab-deleted')).toBe(true);
  expect(tab('a.go').querySelector('[role="tab"]').title).toBe('a.go (deleted)');
  expect(tab('b.go').classList.contains('editor-tabs-tab-preview')).toBe(true);
  act(() => tab('b.go').querySelector('[role="tab"]').dispatchEvent(new MouseEvent('dblclick', {bubbles: true})));
  expect(store.get().session.tabs.preview).toBeNull();
  expect(tab('b.go').classList.contains('editor-tabs-tab-preview')).toBe(false);
});
