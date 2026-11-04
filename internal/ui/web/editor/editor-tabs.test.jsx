import {clickControl} from '../../test-render.jsx';
import {afterEach, expect, spyOn, test} from 'bun:test';
import {act} from 'react';
import {mount, unmountAll} from '../../test-render.jsx';
import {emptySession} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {EditorArea} from './editor-area.jsx';
import {browserTabId, fileTabId, moveTab, selectTab} from './editor.js';
import {findPane, openTab} from './tab-layout.js';

afterEach(unmountAll);

const frame = () => act(() => new Promise(resolve => setTimeout(resolve)));

const openTestTab = (state, id) => ({...state, session: {...state.session, tabs: openTab(state.session.tabs, id)}});

function setup(change = state => openTestTab(openTestTab(state, 'file:one'), 'file:two')) {
  const store = createStore(change({
    overlays: {}, selected: 's', workspaces: [{sessions: [{id: 's', dir: '/repo', title: 'Chat', children: [], activity: 'idle'}]}], drafts: {}, unsent: null,
    workbench: {sidebar: true, rightPanel: true, panel: false}, session: emptySession('s'),
  }));
  const root = mount(store, <EditorArea />);
  const tabs = () => [...root.querySelectorAll('[role="tab"]')];
  const labels = () => tabs().map(t => t.querySelector('.editor-tabs-label').textContent);
  return {store, root, tabs, labels};
}

test('the agent\'s tab shows its title and has no close button', () => {
  const {root, labels} = setup();
  expect(labels()).toEqual(['Chat', 'one', 'two']);
  expect(root.querySelector('[data-tab="agent"] .editor-tabs-close')).toBeNull();
});

test('closing a tab focuses the next one, and closing a pane\'s last focuses another pane\'s tab', async () => {
  const {store, root, tabs, labels} = setup();
  act(() => clickControl(root.querySelector('[aria-label="Close two"]')));
  await frame();
  expect(labels()).toEqual(['Chat', 'one']);
  expect(document.activeElement === tabs()[1]).toBe(true);
  act(() => store.update(state => moveTab(state, 'file:one', state.session.tabs.root.id, 'right')));
  act(() => clickControl(root.querySelector('[aria-label="Close one"]')));
  await frame();
  expect(labels()).toEqual(['Chat']);
  expect(document.activeElement === tabs()[0]).toBe(true);
});

test('each pane shows the mode button only for its active Markdown file', () => {
  const {store, root} = setup(state => ({
    ...state,
    browsers: {s: {url: 'https://example.com/', title: 'Example'}},
    session: {
      ...state.session,
      tabs: ['file:guide.md', 'file:notes.md', 'file:code.go', browserTabId].reduce(openTab, state.session.tabs),
      files: {...state.session.files, open: {
        'guide.md': {text: '# Title', message: null, source: false},
        'notes.md': {text: '# Notes', message: null, source: false},
      }},
    },
  }));
  const buttons = () => [...root.querySelectorAll('.editor-tabs-actions')];
  expect(buttons()).toHaveLength(0); // browser
  const select = id => act(() => store.update(state => selectTab(state, findPane(state.session.tabs.root, id).id, id)));
  select('agent');
  expect(buttons()).toHaveLength(0);
  select('file:code.go');
  expect(buttons()).toHaveLength(0);
  select('file:guide.md');
  expect(buttons()).toHaveLength(1);
  expect(buttons()[0].getAttribute('aria-label')).toBe('Show Source');
  act(() => buttons()[0].click());
  expect(store.get().session.files.open['guide.md'].source).toBe(true);
  expect(buttons()[0].getAttribute('aria-label')).toBe('Show Preview');
  act(() => buttons()[0].click());
  expect(store.get().session.files.open['guide.md'].source).toBe(false);
  act(() => store.update(state => moveTab(state, 'file:notes.md', state.session.tabs.root.id, 'right')));
  expect(buttons()).toHaveLength(2);
});

test('the arrow keys, Home, and End select tabs and keep the focus on them', async () => {
  const {store, tabs} = setup();
  tabs()[2].focus();
  const press = async key => {
    act(() => document.activeElement.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true})));
    await frame();
  };
  await press('ArrowRight');
  expect(store.get().session.tabs.root.active).toBe('agent');
  expect(document.activeElement === tabs()[0]).toBe(true);
  await press('End');
  expect(document.activeElement === tabs()[2]).toBe(true);
  await press('Home');
  await press('ArrowLeft');
  expect(store.get().session.tabs.root.active).toBe('file:two');
  expect(document.activeElement.getAttribute('aria-selected')).toBe('true');
});

test('splitting a pane restores the scroll saved by the departing view', () => {
  const {store, root} = setup(state => openTestTab(openTestTab(state, 'file:two'), 'file:one'));
  const view = root.querySelector('.file-view');
  Object.defineProperty(view, 'scrollHeight', {value: 1000});
  Object.defineProperty(view, 'clientHeight', {value: 300});
  for (const top of [900, 500]) {
    view.scrollTop = top;
    view.dispatchEvent(new Event('scroll'));
  }
  act(() => store.update(state => moveTab(state, 'file:two', state.session.tabs.root.id, 'right')));
  const restored = root.querySelector('.file-view');
  expect(restored).not.toBe(view);
  expect(restored.scrollTop).toBe(500);
  expect(store.get().session.scroll['file:one'].top).toBe(500);
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
  expect(tab('a.go').querySelector('.editor-tabs-label').classList.contains('line-through')).toBe(true);
  expect(tab('a.go').querySelector('[role="tab"]').title).toBe('a.go (deleted)');
  expect(tab('b.go').querySelector('.editor-tabs-label').classList.contains('italic')).toBe(true);
  act(() => tab('b.go').querySelector('[role="tab"]').dispatchEvent(new MouseEvent('dblclick', {bubbles: true})));
  expect(store.get().session.tabs.preview).toBeNull();
  expect(tab('b.go').querySelector('.editor-tabs-label').classList.contains('italic')).toBe(false);
});

test('the browser\'s tab shows its page\'s title and closes without ending the browser', () => {
  const {store, root, labels} = setup(state => ({
    ...openTestTab(state, browserTabId),
    browsers: {s: {url: 'https://example.com/', title: 'Example', loading: false, canGoBack: false, canGoForward: false}},
  }));
  const tab = root.querySelector(`[data-tab="${browserTabId}"]`);
  expect(labels()).toEqual(['Chat', 'Example']);
  expect(tab.querySelector('svg[data-icon]').getAttribute('data-icon')).toBe('globe');
  expect(tab.querySelector('[role="tab"]').title).toBe('https://example.com/');
  act(() => clickControl(root.querySelector('[aria-label="Close Example"]')));
  expect(labels()).toEqual(['Chat']);
  expect(store.get().browsers.s.title).toBe('Example');
});

test('file tab context menu copies its path', () => {
  const copy = spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
  try {
    const {root} = setup();
    const tab = root.querySelector('[data-tab="file:one"] [role=tab]');
    act(() => tab.dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})));
    clickControl([...document.querySelectorAll('[role=menuitem]')].find(item => item.textContent === 'Copy Path'));
    expect(copy).toHaveBeenCalledWith('/repo/one');
  } finally { copy.mockRestore(); }
});

test('diff tabs do not offer session-relative path copying', () => {
  const {root} = setup(state => openTestTab(state, 'diff:src/main.go'));
  const tab = root.querySelector('[data-tab="diff:src/main.go"] [role=tab]');
  act(() => tab.dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})));
  expect(document.querySelector('[role=menu]')).toBeNull();
});

test('find on the page body opens only the last used split pane', () => {
  const {store, root} = setup();
  act(() => store.update(state => moveTab(state, 'file:one', state.session.tabs.root.id, 'right')));
  const panes = [...root.querySelectorAll('.editor-pane')];
  const lastView = panes[1].querySelector('.file-view');
  act(() => lastView.focus());
  act(() => lastView.blur());
  act(() => document.body.dispatchEvent(new KeyboardEvent('keydown', {key: 'f', metaKey: true, bubbles: true, cancelable: true})));
  expect(root.querySelectorAll('[role=search]')).toHaveLength(1);
  expect(panes[1].querySelector('[role=search]')).not.toBeNull();
});


test('find leaves shortcuts alone when focus moves outside the editor', () => {
  const {root} = setup();
  const view = root.querySelector('.file-view');
  const outside = document.createElement('button');
  document.body.append(outside);
  act(() => view.focus());
  act(() => outside.focus());
  const event = new KeyboardEvent('keydown', {key: 'f', metaKey: true, bubbles: true, cancelable: true});
  act(() => outside.dispatchEvent(event));
  expect(event.defaultPrevented).toBe(false);
  expect(root.querySelector('[role=search]')).toBeNull();
  act(() => outside.blur());
  const bodyEvent = new KeyboardEvent('keydown', {key: 'f', metaKey: true, bubbles: true, cancelable: true});
  act(() => document.body.dispatchEvent(bodyEvent));
  expect(root.querySelector('[role=search]')).not.toBeNull();
  outside.remove();
});
