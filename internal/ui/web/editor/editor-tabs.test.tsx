import {afterEach, expect, mock, spyOn, test} from 'bun:test';
import {act} from 'react';
import {clickControl, element, find, mount, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace, unread} from '../../test-state.ts';
import {emptySession} from '../sessions/selection.ts';
import type {AttachedSession} from '../sessions/types.ts';
import type {State} from '../state.ts';
import {createStore} from '../store.ts';
import {EditorArea} from './editor-area.tsx';
import {browserTabId, fileTabId, moveTab, selectTab} from './editor.ts';
import {findPane, openTab} from './tab-layout.ts';
import type {LayoutNode, TabId} from './types.ts';

afterEach(() => {
  mock.restore();
  unmountAll();
});

const frame = () => act(() => new Promise<void>(resolve => setTimeout(resolve)));

const attached = (state: State): AttachedSession => state.session ?? expect.unreachable('no attached session');
const treeOf = (state: State): LayoutNode => attached(state).tabs.root ?? expect.unreachable('no tabs');
const openTestTab = (state: State, id: TabId): State => ({...state, session: {...attached(state), tabs: openTab(attached(state).tabs, id)}});

function setup(change: (state: State) => State = state => openTestTab(openTestTab(state, 'file:one'), 'file:two')) {
  const store = createStore(change(testState({
    selected: 's',
    workspaces: [testWorkspace({id: 'w', sessions: [testSession({id: 's', dir: '/repo', title: 'Chat'})]})],
    session: emptySession('s'),
  })));
  const root = mount(store, <EditorArea />);
  const tabs = () => [...root.querySelectorAll<HTMLElement>('[role="tab"]')];
  const labels = () => tabs().map(t => t.querySelector('.editor-tabs-label')?.textContent);
  return {store, root, tabs, labels};
}

test('the agent\'s tab shows its title and has no close button', () => {
  const {root, labels} = setup();
  expect(labels()).toEqual(['Chat', 'one', 'two']);
  expect(root.querySelector('[data-tab="agent"] .editor-tabs-close')).toBeNull();
});

test('closing a tab focuses the next one, and closing a pane\'s last focuses another pane\'s tab', async () => {
  const {store, root, tabs, labels} = setup();
  clickControl(find(root, '[aria-label="Close two"]'));
  await frame();
  expect(labels()).toEqual(['Chat', 'one']);
  expect(document.activeElement === tabs()[1]).toBe(true);
  act(() => store.update(state => moveTab(state, 'file:one', treeOf(state).id, 'right')));
  clickControl(find(root, '[aria-label="Close one"]'));
  await frame();
  expect(labels()).toEqual(['Chat']);
  expect(document.activeElement === tabs()[0]).toBe(true);
});

test('each pane shows the mode button only for its active Markdown file', () => {
  const {store, root} = setup(state => ({
    ...state,
    browsers: {s: {url: 'https://example.com/', title: 'Example', loading: false, canGoBack: false, canGoForward: false}},
    session: {
      ...attached(state),
      tabs: ['file:guide.md', 'file:notes.md', 'file:code.go', browserTabId].reduce(openTab, attached(state).tabs),
      files: {...attached(state).files, open: {
        'guide.md': {...unread, text: '# Title'},
        'notes.md': {...unread, text: '# Notes'},
      }},
    },
  }));
  const buttons = () => [...root.querySelectorAll('.editor-tabs-actions')];
  expect(buttons()).toHaveLength(0); // browser
  const select = (id: TabId) => act(() => store.update(state => selectTab(state, findPane(treeOf(state), id)?.id ?? expect.unreachable(id), id)));
  select('agent');
  expect(buttons()).toHaveLength(0);
  select('file:code.go');
  expect(buttons()).toHaveLength(0);
  select('file:guide.md');
  expect(buttons()).toHaveLength(1);
  expect(buttons()[0].getAttribute('aria-label')).toBe('Show Source');
  clickControl(find(root, '.editor-tabs-actions'));
  expect(attached(store.get()).files.open['guide.md'].source).toBe(true);
  expect(buttons()[0].getAttribute('aria-label')).toBe('Show Preview');
  clickControl(find(root, '.editor-tabs-actions'));
  expect(attached(store.get()).files.open['guide.md'].source).toBe(false);
  act(() => store.update(state => moveTab(state, 'file:notes.md', treeOf(state).id, 'right')));
  expect(buttons()).toHaveLength(2);
});

test('the arrow keys, Home, and End select tabs and keep the focus on them', async () => {
  const {store, tabs} = setup();
  tabs()[2].focus();
  const press = async (key: string) => {
    act(() => { document.activeElement?.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true})); });
    await frame();
  };
  await press('ArrowRight');
  expect(treeOf(store.get())).toMatchObject({active: 'agent'});
  expect(document.activeElement === tabs()[0]).toBe(true);
  await press('End');
  expect(document.activeElement === tabs()[2]).toBe(true);
  await press('Home');
  await press('ArrowLeft');
  expect(treeOf(store.get())).toMatchObject({active: 'file:two'});
  expect(document.activeElement?.getAttribute('aria-selected')).toBe('true');
});

test('splitting a pane restores the scroll saved by the departing view', () => {
  const {store, root} = setup(state => openTestTab(openTestTab(state, 'file:two'), 'file:one'));
  const view = find(root, '.file-view');
  Object.defineProperty(view, 'scrollHeight', {value: 1000});
  Object.defineProperty(view, 'clientHeight', {value: 300});
  for (const top of [900, 500]) {
    view.scrollTop = top;
    view.dispatchEvent(new Event('scroll'));
  }
  act(() => store.update(state => moveTab(state, 'file:two', treeOf(state).id, 'right')));
  const restored = find(root, '.file-view');
  expect(restored).not.toBe(view);
  expect(restored.scrollTop).toBe(500);
  expect(attached(store.get()).scroll['file:one'].top).toBe(500);
});

test('a preview tab is italic until double-clicked, and a deleted file is struck through', () => {
  const {store, root} = setup(state => ({
    ...state,
    session: {
      ...attached(state),
      tabs: {...[fileTabId('a.go'), fileTabId('b.go')].reduce(openTab, attached(state).tabs), preview: fileTabId('b.go')},
      files: {...attached(state).files, open: {'a.go': {...unread, deleted: true}, 'b.go': unread}},
    },
  }));
  const tab = (path: string) => find(root, `[data-tab="${fileTabId(path)}"]`);
  expect(find(tab('a.go'), '.editor-tabs-label').classList.contains('line-through')).toBe(true);
  expect(find(tab('a.go'), '[role="tab"]').title).toBe('a.go (deleted)');
  expect(find(tab('b.go'), '.editor-tabs-label').classList.contains('italic')).toBe(true);
  act(() => { find(tab('b.go'), '[role="tab"]').dispatchEvent(new MouseEvent('dblclick', {bubbles: true})); });
  expect(attached(store.get()).tabs.preview).toBeNull();
  expect(find(tab('b.go'), '.editor-tabs-label').classList.contains('italic')).toBe(false);
});

test('the browser\'s tab shows its page\'s title and closes without ending the browser', () => {
  const {store, root, labels} = setup(state => ({
    ...openTestTab(state, browserTabId),
    browsers: {s: {url: 'https://example.com/', title: 'Example', loading: false, canGoBack: false, canGoForward: false}},
  }));
  const tab = find(root, `[data-tab="${browserTabId}"]`);
  expect(labels()).toEqual(['Chat', 'Example']);
  expect(element(tab, 'svg[data-icon]', SVGElement).getAttribute('data-icon')).toBe('globe');
  expect(find(tab, '[role="tab"]').title).toBe('https://example.com/');
  clickControl(find(root, '[aria-label="Close Example"]'));
  expect(labels()).toEqual(['Chat']);
  expect(store.get().browsers.s.title).toBe('Example');
});

test('file tab context menu copies its path', () => {
  const copy = spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
  const {root} = setup();
  const tab = find(root, '[data-tab="file:one"] [role=tab]');
  act(() => { tab.dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})); });
  const items = [...document.querySelectorAll<HTMLElement>('[role=menuitem]')];
  clickControl(items.find(item => item.textContent === 'Copy Path') ?? expect.unreachable('Copy Path'));
  expect(copy).toHaveBeenCalledWith('/repo/one');
});

test('diff tabs do not offer session-relative path copying', () => {
  const {root} = setup(state => openTestTab(state, 'diff:src/main.go'));
  const tab = find(root, '[data-tab="diff:src/main.go"] [role=tab]');
  act(() => { tab.dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})); });
  expect(document.querySelector('[role=menu]')).toBeNull();
});

test('find on the page body opens only the last used split pane', () => {
  const {store, root} = setup();
  act(() => store.update(state => moveTab(state, 'file:one', treeOf(state).id, 'right')));
  const panes = [...root.querySelectorAll<HTMLElement>('.editor-pane')];
  const lastView = find(panes[1], '.file-view');
  act(() => lastView.focus());
  act(() => lastView.blur());
  act(() => { document.body.dispatchEvent(new KeyboardEvent('keydown', {key: 'f', metaKey: true, bubbles: true, cancelable: true})); });
  expect(root.querySelectorAll('[role=search]')).toHaveLength(1);
  expect(panes[1].querySelector('[role=search]')).not.toBeNull();
});

test('find leaves shortcuts alone when focus moves outside the editor', () => {
  const {root} = setup();
  const view = find(root, '.file-view');
  const outside = document.createElement('button');
  document.body.append(outside);
  act(() => view.focus());
  act(() => outside.focus());
  const event = new KeyboardEvent('keydown', {key: 'f', metaKey: true, bubbles: true, cancelable: true});
  act(() => { outside.dispatchEvent(event); });
  expect(event.defaultPrevented).toBe(false);
  expect(root.querySelector('[role=search]')).toBeNull();
  act(() => outside.blur());
  const bodyEvent = new KeyboardEvent('keydown', {key: 'f', metaKey: true, bubbles: true, cancelable: true});
  act(() => { document.body.dispatchEvent(bodyEvent); });
  expect(root.querySelector('[role=search]')).not.toBeNull();
  outside.remove();
});
