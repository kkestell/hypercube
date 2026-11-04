import {afterEach, expect, spyOn, test} from 'bun:test';
import {render} from '../../test-render.jsx';
import {act} from 'react';
import {mount, unmountAll} from '../../test-render.jsx';
import {receivedWorkspaces} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {App} from './app.jsx';
import {covered} from '../shared/overlays.js';

afterEach(() => { delete globalThis.webkit; unmountAll(); });

test('state arriving before passive effects reaches the page', async () => {
  const canvas = spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
  try {
    const store = createStore({
      workspaces: [], limits: [], requests: [], selected: '', pending: null, unsent: null,
      visible: true, drafts: {}, dismissed: {}, errors: [], confirmation: null, session: null, overlays: {},
      workbench: {sidebar: true, rightPanel: true, panel: false, rightTab: 'files'},
    });
    const root = document.createElement('div');
    document.body.append(root);
    render(<App store={store} />, root);
    store.update(state => receivedWorkspaces(state, [{id: 'w', name: 'New workspace', sessions: []}]));
    await act(async () => {});
    expect(root.querySelector('[data-workspace="w"] .workspace-section-name').textContent).toBe('New workspace');
  } finally {
    canvas.mockRestore();
  }
});

test('Escape interrupts only the shown busy session and leaves other Escape actions alone', () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  try {
    const sessions = ['a', 'b'].map(id => ({id, title: id, running: true, activity: 'busy', children: []}));
    const store = createStore(receivedWorkspaces({
      workspaces: [], limits: [], requests: [], selected: 'a', pending: null, unsent: null,
      visible: true, drafts: {}, dismissed: {}, errors: [], confirmation: null, session: null, overlays: {},
      workbench: {sidebar: true, rightPanel: false, panel: false, rightTab: 'changes'},
    }, [{id: 'w', name: 'repo', path: '/repo', sessions}]));
    const root = mount(store, <App store={store} />);
    fetches.mockClear();
    const escape = (target, options = {}) => {
      const event = new KeyboardEvent('keydown', {key: 'Escape', bubbles: true, cancelable: true, ...options});
      act(() => target.dispatchEvent(event));
      return event;
    };
    const field = root.querySelector('.composer textarea');
    expect(escape(field).defaultPrevented).toBe(true);
    expect(fetches).toHaveBeenCalledTimes(1);
    expect(Object.fromEntries(new URL(fetches.mock.calls[0][0], 'http://x/').searchParams)).toEqual({action: 'interrupt', id: 'a'});

    escape(field, {repeat: true});
    escape(field, {isComposing: true});
    escape(field, {metaKey: true});
    escape(root.querySelector('.terminal-panel'));
    const handled = new KeyboardEvent('keydown', {key: 'Escape', bubbles: true, cancelable: true});
    handled.preventDefault();
    act(() => field.dispatchEvent(handled));
    expect(fetches).toHaveBeenCalledTimes(1);

    act(() => root.querySelector('[data-session="a"]').dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})));
    escape(document.querySelector('.context-menu'));
    expect(document.querySelector('.context-menu')).toBeNull();
    expect(fetches).toHaveBeenCalledTimes(1);

    store.update(state => ({...state, confirmation: {message: 'Confirm?', primary: 'OK', accept() {}}}));
    escape(document.querySelector('[role=alertdialog]'));
    expect(store.get().confirmation).toBeNull();
    expect(fetches).toHaveBeenCalledTimes(1);

    store.update(state => receivedWorkspaces(state, [{id: 'w', name: 'repo', path: '/repo', sessions: [{...sessions[0], activity: 'idle'}, sessions[1]]}]));
    escape(field);
    expect(fetches).toHaveBeenCalledTimes(1);
  } finally {
    unmountAll();
    fetches.mockRestore();
  }
});

test('the window header holds the three layout toggles and they hide and show their regions', () => {
  const store = createStore({
    workspaces: [], limits: [], requests: [], selected: null, pending: null, unsent: null,
    visible: true, drafts: {}, dismissed: {}, errors: [], confirmation: null, session: null, overlays: {},
    workbench: {sidebar: true, rightPanel: true, panel: false, rightTab: 'files'},
  });
  const root = mount(store, <App store={store} />);
  const buttons = [...root.querySelectorAll('header.app-header button')];
  expect(buttons.map(button => button.getAttribute('aria-label'))).toEqual([
    'Toggle Primary Side Bar', 'Toggle Panel', 'Toggle Secondary Side Bar',
  ]);
  expect(root.querySelectorAll('.editor-tabs-bar .layout-toggles')).toHaveLength(0);
  for (const [index, region, selector] of [
    [0, 'sidebar', '.sidebar'], [1, 'panel', '.terminal-panel'], [2, 'rightPanel', '.right-panel'],
  ]) {
    const before = store.get().workbench[region];
    const split = root.querySelector(selector).closest('.split');
    act(() => buttons[index].click());
    expect(store.get().workbench[region]).toBe(!before);
    expect(split.classList.contains('split-collapsed')).toBe(before);
    act(() => buttons[index].click());
    expect(store.get().workbench[region]).toBe(before);
    expect(split.classList.contains('split-collapsed')).toBe(!before);
  }
});

test('shortcuts navigate visible sessions, toggle panels and leave terminal and editing keys alone', async () => {
  const messages = [];
  globalThis.webkit = {messageHandlers: {window: {postMessage: message => messages.push(message)}}};
  const sessions = ['a', 'b'].map(id => ({id, title: id, running: id === 'b', activity: 'idle', children: []}));
  const store = createStore(receivedWorkspaces({
    workspaces: [], limits: [], requests: [], selected: 'a', pending: null, unsent: null,
    visible: true, drafts: {}, dismissed: {}, errors: [], confirmation: null, session: null, overlays: {},
    workbench: {sidebar: true, rightPanel: true, panel: false, rightTab: 'files'},
  }, [{id: 'w', name: 'repo', path: '/repo', sessions}]));
  const root = mount(store, <App store={store} />);
  const send = (target, key, mods) => {
    const event = new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true, ...mods});
    act(() => target.dispatchEvent(event));
    return event.defaultPrevented;
  };
  const row = () => root.querySelector('.session-item');
  expect(send(row(), 'Tab', {ctrlKey: true})).toBe(true);
  expect(store.get().selected).toBe('b');
  send(row(), 'ArrowUp', {metaKey: true, altKey: true});
  expect(store.get().selected).toBe('a');
  send(row(), '2', {metaKey: true});
  expect(store.get().selected).toBe('a');
  const field = root.querySelector('.composer-text');
  expect(send(field, 'b', {metaKey: true})).toBe(false);
  const terminal = root.querySelector('.terminal-panel');
  expect(messages.some(message => message.type === 'shortcuts')).toBe(true);
  expect(send(terminal, 'j', {metaKey: true})).toBe(false);
  expect(send(terminal, 'k', {metaKey: true})).toBe(false);
  expect(document.querySelector('[cmdk-input]')).toBeNull();
  act(() => row().focus());
  let searchHandled;
  await act(async () => { searchHandled = send(row(), 'k', {metaKey: true}); });
  expect(searchHandled).toBe(true);
  const search = document.querySelector('[cmdk-input]');
  expect(search).not.toBeNull();
  expect(send(search, 'k', {metaKey: true})).toBe(false);
  await act(async () => search.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true, cancelable: true})));
  expect(document.activeElement).toBe(row());
  act(() => row().dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})));
  const menu = document.querySelector('[role=menu]');
  expect(send(menu, 'k', {metaKey: true})).toBe(false);
  expect(document.querySelector('[cmdk-input]')).toBeNull();
  act(() => menu.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true, cancelable: true})));
  expect(send(field, '0', {metaKey: true})).toBe(true);
  expect(messages.at(-1)).toEqual({type: 'page-zoom', action: 'zoomReset'});
  act(() => dispatchEvent(new CustomEvent('hypercube-shortcut', {detail: {id: 'help'}})));
  const dialog = document.querySelector('[role=dialog]');
  expect(send(dialog, 'j', {metaKey: true})).toBe(false);
  act(() => dialog.querySelector('button').click());
  row().focus();
  send(row(), 'b', {metaKey: true});
  expect(store.get().workbench.sidebar).toBe(false);
  expect(document.activeElement).toBe(field);
  send(field, 'n', {metaKey: true});
  expect(store.get().unsent.workspace).toBe('w');
  act(() => dispatchEvent(new CustomEvent('hypercube-shortcut', {detail: {id: 'sidebar'}})));
  expect(store.get().workbench.sidebar).toBe(true);
});

test('composer shortcuts activate the chat when a file shares its pane', () => {
  const sessions = [{id: 'a', title: 'a', running: true, activity: 'idle', children: []}];
  const store = createStore(receivedWorkspaces({
    workspaces: [], limits: [], requests: [], selected: 'a', pending: null, unsent: null,
    visible: true, drafts: {}, dismissed: {}, errors: [], confirmation: null, session: null, overlays: {},
    workbench: {sidebar: true, rightPanel: true, panel: false, rightTab: 'files'},
  }, [{id: 'w', name: 'repo', path: '/repo', sessions}]));
  const showFile = () => store.update(s => ({...s, session: {...s.session,
    tabs: {...s.session.tabs, root: {id: 1, tabs: ['agent', 'file:test.txt'], active: 'file:test.txt'}},
    files: {...s.session.files, open: {'test.txt': {text: 'hello', lines: []}}},
  }}));
  showFile();
  const root = mount(store, <App store={store} />);
  const send = (target, key) => act(() => target.dispatchEvent(new KeyboardEvent('keydown', {key, metaKey: true, bubbles: true, cancelable: true})));
  const fileTab = root.querySelector('[data-tab="file:test.txt"]');
  fileTab.focus();
  send(fileTab, 'l');
  expect(store.get().session.tabs.root.active).toBe('agent');
  expect(document.activeElement).toBe(root.querySelector('.composer-text'));

  act(showFile);
  const row = root.querySelector('.session-item');
  row.focus();
  send(row, 'b');
  expect(store.get().workbench.sidebar).toBe(false);
  expect(store.get().session.tabs.root.active).toBe('agent');
  expect(document.activeElement).toBe(root.querySelector('.composer-text'));
});

test('Command+P opens Quick Open from the terminal for the shown session', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async url => String(url).startsWith('mention-files?') ? Response.json(['src/main.go']) : new Response(''));
  try {
    const store = createStore(receivedWorkspaces({
      workspaces: [], limits: [], requests: [], selected: 'a', pending: null, unsent: null,
      visible: true, drafts: {}, dismissed: {}, errors: [], confirmation: null, session: null, overlays: {},
      workbench: {sidebar: true, rightPanel: false, panel: false, rightTab: 'files'},
    }, [{id: 'w', name: 'repo', path: '/repo', sessions: [{id: 'a', dir: '/repo', title: 'a', running: true, activity: 'idle', children: []}]}]));
    const root = mount(store, <App store={store} />);
    const event = new KeyboardEvent('keydown', {key: 'p', metaKey: true, bubbles: true, cancelable: true});
    act(() => root.querySelector('.terminal-panel').dispatchEvent(event));
    await act(async () => {});
    expect(event.defaultPrevented).toBe(true);
    expect(document.querySelector('[aria-label="Search files"]')).not.toBeNull();
    expect(document.querySelector('[cmdk-item]').textContent).toBe('src/main.go');
    act(() => document.querySelector('[aria-label="Search files"]').dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true, cancelable: true})));
    expect(store.get().session.tabs.preview).toBe('file:src/main.go');
  } finally { unmountAll(); fetches.mockRestore(); }
});

test('keyboard shortcuts dialog opens from the keyboard and the native menu', async () => {
  const store = createStore({
    workspaces: [], limits: [], requests: [], selected: null, pending: null, unsent: null,
    visible: true, drafts: {}, dismissed: {}, errors: [], confirmation: null, session: null, overlays: {},
    workbench: {sidebar: true, rightPanel: true, panel: false, rightTab: 'files'},
  });
  const root = mount(store, <App store={store} />);
  const button = root.querySelector('header button');
  button.focus();
  act(() => button.dispatchEvent(new KeyboardEvent('keydown', {key: '/', metaKey: true, bubbles: true, cancelable: true})));
  const dialog = document.querySelector('[role=dialog]');
  expect(dialog.textContent).toContain('Keyboard shortcuts');
  expect(dialog.textContent).toContain('New Session');
  expect(dialog.textContent).toContain('Toggle Terminal');
  expect(covered(store.get(), {x: 10000, y: 10000, width: 100, height: 100})).toBe(true);
  const focused = new Promise(resolve => button.addEventListener('focus', resolve, {once: true}));
  act(() => dialog.querySelector('button').click());
  await focused;
  expect(document.querySelector('[role=dialog]')).toBeNull();
  expect(covered(store.get(), {x: 10000, y: 10000, width: 100, height: 100})).toBe(false);
  expect(document.activeElement === button).toBe(true);
  act(() => dispatchEvent(new CustomEvent('hypercube-shortcut', {detail: {id: 'help'}})));
  expect(document.querySelector('[role=dialog]').textContent).toContain('Keyboard shortcuts');
  act(() => document.querySelector('[role=dialog] button').click());
  act(() => dispatchEvent(new CustomEvent('hypercube-shortcut', {detail: {id: 'panel'}})));
  expect(store.get().workbench.panel).toBe(true);
});

test('Settings opens from the sidebar, shortcut and menu and removes saved choices', async () => {
  const choices = [{session: '', title: '', harness: 'codex', tool: 'shell'}, {session: 'a', title: 'My session', harness: 'claude', tool: 'Bash'}];
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async (url, init) => {
    if (init?.method === 'POST') return new Response('');
    return Response.json(choices);
  });
  try {
    const store = createStore({
      workspaces: [], limits: [], requests: [], selected: null, pending: null, unsent: null,
      visible: true, drafts: {}, dismissed: {}, errors: [], confirmation: null, session: null, overlays: {},
      workbench: {sidebar: true, rightPanel: true, panel: false, rightTab: 'files'},
    });
    const root = mount(store, <App store={store} />);
    await act(async () => root.querySelector('[aria-label="Settings"]').click());
    let dialog = document.querySelector('[role=dialog]');
    expect(dialog.textContent).toContain('Always Allowed Tools');
    expect(dialog.textContent).toContain('codex · All sessions');
    expect(dialog.textContent).toContain('claude · My session');
    await act(async () => dialog.querySelector('[aria-label="Remove shell, codex, all sessions"]').click());
    expect(dialog.textContent).not.toContain('shell');
    expect(Object.fromEntries(new URL(fetches.mock.calls.at(-1)[0], 'http://x/').searchParams)).toEqual({session: '', harness: 'codex', tool: 'shell'});
    act(() => dialog.querySelector('[data-slot="dialog-close"]').click());
    const event = new KeyboardEvent('keydown', {key: ',', metaKey: true, bubbles: true, cancelable: true});
    await act(async () => root.querySelector('.terminal-panel').dispatchEvent(event));
    expect(event.defaultPrevented).toBe(true);
    dialog = document.querySelector('[role=dialog]');
    expect(dialog.textContent).toContain('Settings');
    act(() => dialog.querySelector('[data-slot="dialog-close"]').click());
    await act(async () => dispatchEvent(new CustomEvent('hypercube-shortcut', {detail: {id: 'settings'}})));
    expect(document.querySelector('[role=dialog]').textContent).toContain('Settings');
  } finally { unmountAll(); fetches.mockRestore(); }
});
