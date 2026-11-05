import {afterEach, beforeEach, expect, mock, spyOn, test} from 'bun:test';
import {act} from 'react';
import {find, mount, render, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace, unread} from '../../test-state.ts';
import {panes} from '../editor/tab-layout.ts';
import {receivedWorkspaces} from '../sessions/selection.ts';
import type {Session} from '../sessions/types.ts';
import {covered} from '../shared/overlays.ts';
import type {State} from '../state.ts';
import {createStore, type Store} from '../store.ts';
import {App} from './app.tsx';
import type {Grant, Region, Workbench} from './types.ts';

beforeEach(() => {
  stubFetch(() => Response.json([]));
});
afterEach(() => {
  delete globalThis.webkit;
  mock.restore();
  unmountAll();
});

const workbench = (overrides: Partial<Workbench>): Workbench => ({...testState().workbench, ...overrides});
const workspaces = (...sessions: Session[]) => [testWorkspace({id: 'w', name: 'repo', path: '/repo', sessions})];
const appStore = (overrides: Partial<State> = {}, ...sessions: Session[]) =>
  createStore(receivedWorkspaces(testState(overrides), workspaces(...sessions)));

test('an empty workspace view offers to add a project without an empty tab list', async () => {
  const store = createStore(testState({workspaces: [], selected: null}));
  const requests = answer(() => new Response('new-workspace'));
  const root = mount(store, <App store={store} />);
  const empty = find(root, 'section[aria-label="No workspaces"]');
  expect(empty.textContent).toContain('Choose a project folder');
  expect(root.querySelector('.editor-tabs[role=tablist]')).toBeNull();
  await act(async () => find(empty, 'button').click());
  expect(requests).toContainEqual({params: {action: 'new'}, method: 'POST'});
  expect(store.get().unsent?.workspace).toBe('new-workspace');
});

interface Sent {
  params: Record<string, string>;
  method?: string;
}

// answer replies to every request with respond(), and returns the requests as
// they arrive.
function answer(respond: (url: string, init?: RequestInit) => Response) {
  const requests: Sent[] = [];
  stubFetch((url, init) => {
    requests.push({params: Object.fromEntries(new URL(url, 'http://x/').searchParams), method: init?.method});
    return respond(url, init);
  });
  return requests;
}

function buttonNamed(root: ParentNode, name: string): HTMLElement {
  const button = [...root.querySelectorAll('button')].find(item => item.textContent === name);
  if (!button) throw new Error('No button named ' + name);
  return button;
}

const sendDisabled = (root: ParentNode) => {
  const button = find(root, '.composer [title=Send]');
  return button instanceof HTMLButtonElement && button.disabled;
};

const keydown = (target: EventTarget, key: string, options: KeyboardEventInit = {}) => {
  const event = new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true, ...options});
  act(() => { target.dispatchEvent(event); });
  return event;
};

const activeTab = (store: Store) => panes(store.get().session?.tabs.root ?? null)[0]?.active;

const nativeShortcut = (id: string) => act(() => { dispatchEvent(new CustomEvent('hypercube-shortcut', {detail: {id}})); });

test('state arriving before passive effects reaches the page', async () => {
  spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
  const store = createStore(testState());
  const root = document.createElement('div');
  document.body.append(root);
  render(<App store={store} />, root);
  store.update(state => receivedWorkspaces(state, [testWorkspace({id: 'w', name: 'New workspace'})]));
  await act(async () => {});
  expect(find(root, '[data-workspace="w"] .workspace-section-name').textContent).toBe('New workspace');
});

test('Escape interrupts only the shown busy session and leaves other Escape actions alone', () => {
  const requests = answer(() => new Response(''));
  const sessions = ['a', 'b'].map(id => testSession({id, title: id, running: true, activity: 'busy'}));
  const store = appStore({selected: 'a', workbench: workbench({rightPanel: false, rightTab: 'search'})}, ...sessions);
  const root = mount(store, <App store={store} />);
  requests.length = 0;
  const field = find(root, '.composer textarea');
  expect(keydown(field, 'Escape').defaultPrevented).toBe(true);
  expect(requests).toHaveLength(1);
  expect(requests[0].params).toEqual({action: 'interrupt', id: 'a'});

  keydown(field, 'Escape', {repeat: true});
  keydown(field, 'Escape', {isComposing: true});
  keydown(field, 'Escape', {metaKey: true});
  keydown(find(root, '.terminal-panel'), 'Escape');
  const handled = new KeyboardEvent('keydown', {key: 'Escape', bubbles: true, cancelable: true});
  handled.preventDefault();
  act(() => { field.dispatchEvent(handled); });
  expect(requests).toHaveLength(1);

  act(() => { find(root, '[data-session="a"]').dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})); });
  keydown(find(document, '.context-menu'), 'Escape');
  expect(document.querySelector('.context-menu')).toBeNull();
  expect(requests).toHaveLength(1);

  store.update(state => ({...state, confirmation: {message: 'Confirm?', detail: '', primary: 'OK', accept() {}}}));
  keydown(find(document, '[role=alertdialog]'), 'Escape');
  expect(store.get().confirmation).toBeNull();
  expect(requests).toHaveLength(1);

  store.update(state => receivedWorkspaces(state, workspaces({...sessions[0], activity: 'idle'}, sessions[1])));
  keydown(field, 'Escape');
  expect(requests).toHaveLength(1);
});

test('the header holds the three layout toggles and they hide and show their regions', () => {
  const store = createStore(testState({selected: null}));
  const root = mount(store, <App store={store} />);
  const buttons = [...root.querySelectorAll('.layout-toggles button')];
  expect(buttons.map(button => button.getAttribute('aria-label'))).toEqual([
    'Toggle Primary Side Bar', 'Toggle Panel', 'Toggle Secondary Side Bar',
  ]);
  const regions: [number, Region, string][] = [
    [0, 'sidebar', '.sidebar'], [1, 'panel', '.terminal-panel'], [2, 'rightPanel', '.right-panel'],
  ];
  for (const [index, region, selector] of regions) {
    const before = store.get().workbench[region];
    const split = find(root, selector).closest('.split');
    act(() => { find(root, `.layout-toggles button:nth-child(${index + 1})`).click(); });
    expect(store.get().workbench[region]).toBe(!before);
    expect(split?.classList.contains('split-collapsed')).toBe(before);
    act(() => { find(root, `.layout-toggles button:nth-child(${index + 1})`).click(); });
    expect(store.get().workbench[region]).toBe(before);
    expect(split?.classList.contains('split-collapsed')).toBe(!before);
  }
});

test('shortcuts navigate visible sessions, toggle panels and leave terminal and editing keys alone', async () => {
  const messages: object[] = [];
  globalThis.webkit = {messageHandlers: {window: {postMessage: message => { messages.push(message); }}}};
  const sessions = ['a', 'b'].map(id => testSession({id, title: id, running: true}));
  const store = appStore({selected: 'a'}, ...sessions);
  const root = mount(store, <App store={store} />);
  const send = (target: EventTarget, key: string, mods: KeyboardEventInit = {}) => keydown(target, key, mods).defaultPrevented;
  const row = () => find(root, '.session-item');
  expect(send(row(), 'Tab', {ctrlKey: true})).toBe(true);
  expect(store.get().selected).toBe('b');
  send(row(), 'ArrowUp', {metaKey: true, altKey: true});
  expect(store.get().selected).toBe('a');
  send(row(), '2', {metaKey: true});
  expect(store.get().selected).toBe('b');
  const field = find(root, '.composer-text');
  expect(send(field, 'b', {metaKey: true})).toBe(false);
  const terminal = find(root, '.terminal-panel');
  expect(messages.some(message => 'type' in message && message.type === 'shortcuts')).toBe(true);
  expect(send(terminal, 'j', {metaKey: true})).toBe(false);
  expect(send(terminal, 'k', {metaKey: true})).toBe(false);
  expect(document.querySelector('[cmdk-input]')).toBeNull();
  act(() => row().focus());
  let searchHandled = false;
  await act(async () => { searchHandled = send(row(), 'k', {metaKey: true}); });
  expect(searchHandled).toBe(true);
  const search = find(document, '[cmdk-input]');
  expect(send(search, 'k', {metaKey: true})).toBe(false);
  await act(async () => { search.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true, cancelable: true})); });
  expect(document.activeElement).toBe(row());
  act(() => { row().dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})); });
  const menu = find(document, '[role=menu]');
  expect(send(menu, 'k', {metaKey: true})).toBe(false);
  expect(document.querySelector('[cmdk-input]')).toBeNull();
  keydown(menu, 'Escape');
  expect(send(field, '0', {metaKey: true})).toBe(true);
  expect(messages.at(-1)).toEqual({type: 'page-zoom', action: 'zoomReset'});
  nativeShortcut('help');
  const dialog = find(document, '[role=dialog]');
  expect(send(dialog, 'j', {metaKey: true})).toBe(false);
  act(() => { find(dialog, 'button').click(); });
  row().focus();
  send(row(), 'b', {metaKey: true});
  expect(store.get().workbench.sidebar).toBe(false);
  expect(document.activeElement).toBe(field);
  send(field, 'n', {metaKey: true});
  expect(store.get().unsent?.workspace).toBe('w');
  nativeShortcut('sidebar');
  expect(store.get().workbench.sidebar).toBe(true);
});

test('composer shortcuts activate the chat when a file shares its pane', () => {
  const store = appStore({selected: 'a'}, testSession({id: 'a', title: 'a', running: true}));
  const showFile = () => store.update(s => s.session ? {...s, session: {...s.session,
    tabs: {...s.session.tabs, root: {id: 1, tabs: ['agent', 'file:test.txt'], active: 'file:test.txt'}},
    files: {...s.session.files, open: {'test.txt': {...unread, text: 'hello'}}},
  }} : s);
  showFile();
  const root = mount(store, <App store={store} />);
  const send = (target: EventTarget, key: string) => keydown(target, key, {metaKey: true});
  const fileTab = find(root, '[data-tab="file:test.txt"]');
  fileTab.focus();
  send(fileTab, 'l');
  expect(activeTab(store)).toBe('agent');
  expect(document.activeElement).toBe(find(root, '.composer-text'));

  act(showFile);
  const row = find(root, '.session-item');
  row.focus();
  send(row, 'b');
  expect(store.get().workbench.sidebar).toBe(false);
  expect(activeTab(store)).toBe('agent');
  expect(document.activeElement).toBe(find(root, '.composer-text'));
});

test('Command+P opens Quick Open from the terminal for the shown session', async () => {
  answer(url => url.startsWith('mention-files?') ? Response.json(['src/main.go']) : new Response(''));
  const store = appStore({selected: 'a', workbench: workbench({rightPanel: false})}, testSession({id: 'a', dir: '/repo', title: 'a', running: true}));
  const root = mount(store, <App store={store} />);
  const event = keydown(find(root, '.terminal-panel'), 'p', {metaKey: true});
  await act(async () => {});
  expect(event.defaultPrevented).toBe(true);
  const search = find(document, '[aria-label="Search files"]');
  expect(find(document, '[cmdk-item]').textContent).toBe('src/main.go');
  keydown(search, 'Enter');
  expect(store.get().session?.tabs.preview).toBe('file:src/main.go');
});

test('keyboard shortcuts dialog opens from the keyboard and the native menu', async () => {
  const store = createStore(testState({selected: null}));
  const root = mount(store, <App store={store} />);
  const button = find(root, '.layout-toggles button');
  button.focus();
  keydown(button, '/', {metaKey: true});
  const dialog = find(document, '[role=dialog]');
  expect(dialog.textContent).toContain('Keyboard shortcuts');
  expect(dialog.textContent).toContain('New Session');
  expect(dialog.textContent).toContain('Toggle Terminal');
  expect(covered(store.get(), {x: 10000, y: 10000, width: 100, height: 100})).toBe(true);
  const focused = new Promise(resolve => button.addEventListener('focus', resolve, {once: true}));
  act(() => { find(dialog, 'button').click(); });
  await focused;
  expect(document.querySelector('[role=dialog]')).toBeNull();
  expect(covered(store.get(), {x: 10000, y: 10000, width: 100, height: 100})).toBe(false);
  expect(document.activeElement === button).toBe(true);
  nativeShortcut('help');
  expect(find(document, '[role=dialog]').textContent).toContain('Keyboard shortcuts');
  act(() => { find(document, '[role=dialog] button').click(); });
  nativeShortcut('panel');
  expect(store.get().workbench.panel).toBe(true);
});

test('Settings opens from the sidebar, shortcut and menu and removes saved choices', async () => {
  const choices: Grant[] = [{session: '', title: '', harness: 'codex', tool: 'shell'}, {session: 'a', title: 'My session', harness: 'claude', tool: 'Bash'}];
  const version = 'Hypercube v0.1.0-beta.1+dirty (abc123)';
  const requests = answer((url, init) => url === 'version' ? new Response(version) : init?.method === 'POST' ? new Response('') : Response.json(choices));
  const store = createStore(testState({selected: null}));
  const root = mount(store, <App store={store} />);
  await act(async () => { find(root, '[aria-label="Settings"]').click(); });
  let dialog = find(document, '[role=dialog]');
  expect(dialog.textContent).toContain('Always Allowed Tools');
  expect(dialog.textContent).toContain('codex · All sessions');
  expect(dialog.textContent).toContain('claude · My session');
  expect(find(dialog, '[aria-label="Build"]').textContent).toContain(version);
  await act(async () => { find(dialog, '[aria-label="Remove shell, codex, all sessions"]').click(); });
  expect(dialog.textContent).not.toContain('shell');
  expect(requests.at(-1)?.params).toEqual({session: '', harness: 'codex', tool: 'shell'});
  act(() => { find(dialog, '[data-slot="dialog-close"]').click(); });
  const event = new KeyboardEvent('keydown', {key: ',', metaKey: true, bubbles: true, cancelable: true});
  await act(async () => { find(root, '.terminal-panel').dispatchEvent(event); });
  expect(event.defaultPrevented).toBe(true);
  dialog = find(document, '[role=dialog]');
  expect(dialog.textContent).toContain('Settings');
  act(() => { find(dialog, '[data-slot="dialog-close"]').click(); });
  await act(async () => { dispatchEvent(new CustomEvent('hypercube-shortcut', {detail: {id: 'settings'}})); });
  expect(find(document, '[role=dialog]').textContent).toContain('Settings');
});

test('Settings lists archived families and opens and deletes them', async () => {
  const requests = answer((_, init) => init?.method === 'POST' ? new Response('') : Response.json([]));
  const child = testSession({id: 'child', parent: 'archived', title: 'Child chat', ended: true, activity: 'exited'});
  const archived = testSession({id: 'archived', title: 'Finished work', ended: true, activity: 'exited', children: [child]});
  const active = testSession({id: 'active', title: 'Active work', running: true});
  const store = createStore(receivedWorkspaces(testState({selected: 'active', workbench: workbench({rightPanel: false})}),
    [testWorkspace({id: 'w', name: 'My workspace', path: '/repo', sessions: [active, archived]})]));
  const root = mount(store, <App store={store} />);
  expect(root.querySelector('[data-session="archived"]')).toBeNull();
  await act(async () => { find(root, '[aria-label="Settings"]').click(); });
  let settings = find(document, '[role=dialog]');
  expect(settings.textContent).toContain('Archived Sessions');
  expect(settings.textContent).toContain('Finished work');
  expect(settings.textContent).toContain('My workspace');
  expect(settings.textContent).not.toContain('Active work');
  await act(async () => { find(settings, '[aria-label="Open Finished work, My workspace"]').click(); });
  expect(document.querySelector('[role=dialog]')).toBeNull();
  expect(store.get().selected).toBe('archived');
  expect(root.querySelector('[data-session="child"]')).not.toBeNull();
  expect(sendDisabled(root)).toBe(true);
  act(() => { find(root, '[data-session="child"]').click(); });
  expect(store.get().selected).toBe('child');
  expect(sendDisabled(root)).toBe(true);
  act(() => { find(root, '[data-session="active"]').click(); });
  expect(root.querySelector('[data-session="archived"]')).toBeNull();
  await act(async () => { find(root, '[aria-label="Settings"]').click(); });
  settings = find(document, '[role=dialog]');
  act(() => { find(settings, '[aria-label="Delete Finished work, My workspace"]').click(); });
  const confirmation = find(document, '[role=alertdialog]');
  expect(confirmation.textContent).toContain("Its chat and its children's chats are deleted.");
  expect(requests.filter(request => request.method === 'POST')).toHaveLength(0);
  await act(async () => { buttonNamed(confirmation, 'Delete').click(); });
  expect(requests.at(-1)?.params).toEqual({action: 'delete', id: 'archived'});
});
