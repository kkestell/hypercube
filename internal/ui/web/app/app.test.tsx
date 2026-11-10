import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {clickControl, element, find, findAll, mount, setValue, stubFetch, testBridge, unmountAll} from '../../test-render.tsx';
import {testConfig, testSession, testState, testWorkspace} from '../../test-state.ts';
import {openUnsent, receivedWorkspaces, selectSession, selectWorkspace} from '../sessions/selection.ts';
import {typed, unsentKey} from '../agent/drafts.ts';
import {closeTab, moveTab, openFileTab, showMain} from '../editor/editor.ts';
import {mainTabId, panes} from '../editor/tab-layout.ts';
import {receivedTranscript} from '../transcript/transcripts.ts';
import type {Session} from '../sessions/types.ts';
import type {State} from '../state.ts';
import {createStore} from '../store.ts';
import {App} from './app.tsx';
import type {Workbench} from './types.ts';

afterEach(() => {
  mock.restore();
  unmountAll();
  delete globalThis.hypercube;
});

const workbench = (overrides: Partial<Workbench>): Workbench => ({...testState().workbench, ...overrides});
const workspaces = (...sessions: Session[]) => [testWorkspace({id: 'w', name: 'repo', path: '/repo', sessions})];
const appStore = (overrides: Partial<State> = {}, ...sessions: Session[]) =>
  createStore(receivedWorkspaces(testState(overrides), workspaces(...sessions)));

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

// transcriptView is the transcript's scroller, around its list.
const transcriptView = (root: ParentNode) => find(root, '[aria-label=Transcript]').parentElement!;

const sendDisabled = (root: ParentNode) => {
  const button = find(root, '[aria-label=Send]');
  return button instanceof HTMLButtonElement && button.disabled;
};

const keydown = (target: EventTarget, key: string, options: KeyboardEventInit = {}) => {
  const event = new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true, ...options});
  act(() => { target.dispatchEvent(event); });
  return event;
};

test('Escape interrupts only the shown busy session and leaves other Escape actions alone', async () => {
  const requests = answer(() => new Response(''));
  const sessions = ['a', 'b'].map(id => testSession({id, title: id, activity: 'busy'}));
  const store = appStore({selected: 'a', workbench: workbench({rightPanel: false, rightTab: 'search'})}, ...sessions);
  const root = mount(store, <App store={store} />);
  requests.length = 0;
  const field = find(root, 'textarea[aria-label=Message]');
  await act(async () => { expect(keydown(field, 'Escape').defaultPrevented).toBe(true); });
  expect(requests).toHaveLength(1);
  expect(requests[0].params).toEqual({action: 'interrupt', id: 'a'});

  keydown(field, 'Escape', {repeat: true});
  keydown(field, 'Escape', {isComposing: true});
  keydown(field, 'Escape', {metaKey: true});
  keydown(find(root, '[data-region=panel]'), 'Escape');
  const handled = new KeyboardEvent('keydown', {key: 'Escape', bubbles: true, cancelable: true});
  handled.preventDefault();
  act(() => { field.dispatchEvent(handled); });
  expect(requests).toHaveLength(1);

  act(() => { find(root, '[data-row="a"]').dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true})); });
  keydown(find(document, '[role=menu]'), 'Escape');
  expect(document.querySelector('[role=menu]')).toBeNull();
  expect(requests).toHaveLength(1);

  act(() => store.update(state => ({...state, confirmation: {message: 'Confirm?', detail: '', primary: 'OK', accept() {}}})));
  keydown(find(document, '[role=alertdialog]'), 'Escape');
  expect(store.get().confirmation).toBeNull();
  expect(requests).toHaveLength(1);

  act(() => store.update(state => receivedWorkspaces(state, workspaces({...sessions[0], activity: 'idle'}, sessions[1]))));
  keydown(field, 'Escape');
  expect(requests).toHaveLength(1);
});

test('app shortcuts work with focus on the transcript background', async () => {
  const requests = answer(() => Response.json([]));
  const store = appStore({selected: 'a'}, testSession({id: 'a', activity: 'busy'}));
  const root = mount(store, <App store={store} />);
  act(() => find(root, 'textarea[aria-label=Message]').blur());
  expect(document.activeElement).toBe(document.body);
  expect(keydown(document.body, 'b', {metaKey: true}).defaultPrevented).toBe(true);
  expect(store.get().workbench.sidebar).toBe(false);
  expect(keydown(document.body, 'p', {metaKey: true}).defaultPrevented).toBe(true);
  expect(document.querySelector('[role=dialog]')).not.toBeNull();
  keydown(document.activeElement ?? document.body, 'Escape');
  await act(async () => { keydown(document.body, 'Escape'); });
  expect(requests.some(request => request.params.action === 'interrupt')).toBe(true);
});

test('chat is a tab that files open on top of, that splits beside them, and that never closes', async () => {
  answer(url => new URL(url, 'http://x/').pathname === '/file' ? new Response('import math') : Response.json([]));
  const store = appStore({selected: 'a', workbench: workbench({rightPanel: false, fixedWidth: true})}, testSession({id: 'a', title: 'Visible chat'}));
  store.update(state => typed(receivedTranscript(state, {type: 'transcript', session: 'a', reset: true, items: [
    {id: 'reply', seq: 1, kind: 'text', text: 'Shown reply'},
  ]}), 'a', 'unfinished prompt'));
  const root = mount(store, <App store={store} />);
  const tabs = () => findAll(root, '[data-editor-area] [role=tab]').map(tab => tab.textContent);
  const chat = () => root.querySelector('[aria-label=Chat]');
  expect(chat()?.closest('[data-editor-area]')).not.toBeNull();
  expect(chat()?.textContent).toContain('Shown reply');
  expect(tabs()).toEqual(['Visible chat']);
  expect(root.querySelector('[aria-label="Close Visible chat"]')).toBeNull();
  expect(root.querySelector('[aria-label="Fixed Width"]')).toBeNull();
  expect(find(root, '[aria-label=Transcript]').className).not.toContain('max-w-');

  // The test page has no width, so the first file opens on top of chat.
  await act(async () => { openFileTab(store, 'a.py'); });
  expect(tabs()).toEqual(['Visible chat', 'a.py']);
  expect(chat()).toBeNull();
  expect(find(root, '[data-editor-area]').textContent).toContain('import math');
  act(() => store.update(showMain));
  expect(element(root, 'textarea[aria-label=Message]', HTMLTextAreaElement).value).toBe('unfinished prompt');

  act(() => store.update(state => moveTab(state, mainTabId, panes(state.editor!.tabs.root)[0].id, 'left')));
  expect(root.querySelectorAll('[data-pane]')).toHaveLength(2);
  expect(chat()?.textContent).toContain('Shown reply');
  expect(find(root, '[data-editor-area]').textContent).toContain('import math');
  act(() => store.update(state => closeTab(closeTab(state, 'file:a.py'), mainTabId)));
  expect(root.querySelectorAll('[data-pane]')).toHaveLength(1);
  expect(tabs()).toEqual(['Visible chat']);
});

test('Find follows focus between chat and files, and focusing the composer brings chat forward', async () => {
  answer(url => new URL(url, 'http://x/').pathname === '/file' ? new Response('import math') : Response.json([]));
  const store = appStore({selected: 'a', workbench: workbench({rightPanel: false})}, testSession({id: 'a'}));
  const root = mount(store, <App store={store} />);
  await act(async () => { openFileTab(store, 'a.py'); });
  act(() => find(root, '[aria-label="File contents"]').focus());
  keydown(document.activeElement!, 'f', {metaKey: true});
  expect(root.querySelector('[role=search][aria-label="Find in file"]')).not.toBeNull();
  expect(root.querySelector('[role=search][aria-label="Find in transcript"]')).toBeNull();
  keydown(document.activeElement!, 'Escape');
  act(() => { keydown(document.activeElement!, 'l', {metaKey: true}); });
  expect(document.activeElement).toBe(find(root, 'textarea[aria-label=Message]'));
  expect(panes(store.get().editor!.tabs.root).flatMap(pane => pane.tabs)).toContain('file:a.py');
  keydown(document.activeElement!, 'f', {metaKey: true});
  expect(root.querySelector('[role=search][aria-label="Find in transcript"]')).not.toBeNull();
  expect(root.querySelector('[role=search][aria-label="Find in file"]')).toBeNull();
});

test('a reply is seen only once chat shows, not while a file covers it', async () => {
  const requests = answer(url => new URL(url, 'http://x/').pathname === '/file' ? new Response('import math') : new Response(''));
  const store = appStore({selected: 'a', visible: true, workbench: workbench({rightPanel: false})}, testSession({id: 'a'}));
  const root = mount(store, <App store={store} />);
  await act(async () => { openFileTab(store, 'a.py'); });
  const seen = () => requests.filter(request => request.params.action === 'seen');
  act(() => store.update(state => receivedWorkspaces(state, workspaces(testSession({id: 'a', unseen: true})))));
  expect(seen()).toHaveLength(0);
  await act(async () => store.update(showMain));
  expect(seen()).toEqual([{params: {action: 'seen', id: 'a'}, method: 'POST'}]);
  expect(root.querySelector('[aria-label=Chat]')).not.toBeNull();
});

test('the composer takes the focus each time its session is chosen again, but not when chat moves', () => {
  answer(() => Response.json([]));
  const store = appStore({selected: 'a', workbench: workbench({rightPanel: false})}, testSession({id: 'a'}));
  const root = mount(store, <App store={store} />);
  const composer = () => find(root, 'textarea[aria-label=Message]');
  expect(document.activeElement).toBe(composer());
  act(() => store.update(state => selectWorkspace(state, 'w')));
  act(() => store.update(state => selectSession(state, 'a')));
  expect(document.activeElement).toBe(composer());
  act(() => (document.activeElement as HTMLElement).blur());
  act(() => store.update(state => moveTab(state, mainTabId, panes(state.editor!.tabs.root)[0].id, 'left')));
  expect(document.activeElement).toBe(document.body);
});

test('a new session draft is a sidebar row that reopens its prompt and can be discarded', async () => {
  answer(() => Response.json(testConfig()));
  const store = appStore({selected: 'a'}, testSession({id: 'a'}));
  const root = mount(store, <App store={store} />);
  const draftRow = () => root.querySelector<HTMLElement>(`[data-region=sidebar] [data-row="${unsentKey('w')}"]`);
  act(() => store.update(state => openUnsent(state, 'w')));
  act(() => find(root, '[data-region=sidebar] [data-row="a"]').click());
  expect(draftRow()).toBeNull();

  act(() => store.update(state => typed(openUnsent(state, 'w'), unsentKey('w'), 'finish this later')));
  expect(draftRow()?.getAttribute('aria-selected')).toBe('true');
  act(() => find(root, '[data-region=sidebar] [data-row="a"]').click());
  expect(store.get().selected).toBe('a');
  act(() => draftRow()!.click());
  expect(element(root, 'textarea[aria-label=Message]', HTMLTextAreaElement).value).toBe('finish this later');

  act(() => { draftRow()!.dispatchEvent(new MouseEvent('contextmenu', {bubbles: true})); });
  expect(findAll(document, '[role=menuitem]').map(item => item.textContent)).toEqual(['Discard Draft']);
  act(() => { clickControl(findAll(document, '[role=menuitem]')[0]); });
  expect(draftRow()).toBeNull();
  expect(store.get().unsent).toBeNull();
  expect(store.get().drafts[unsentKey('w')]).toBeUndefined();
  await act(async () => {});
});

test('archived families open from the sidebar and can be deleted', async () => {
  const requests = answer((_, init) => init?.method === 'POST' ? new Response('') : Response.json([]));
  const child = testSession({id: 'child', parent: 'archived', title: 'Child chat', ended: true, activity: 'exited'});
  const archived = testSession({id: 'archived', title: 'Finished work', ended: true, activity: 'exited', children: [child]});
  const active = testSession({id: 'active', title: 'Active work'});
  const store = createStore(receivedWorkspaces(testState({selected: 'active', workbench: workbench({rightPanel: false})}),
    [testWorkspace({id: 'w', name: 'My workspace', path: '/repo', sessions: [active, archived]})]));
  const root = mount(store, <App store={store} />);
  expect(root.querySelector('[data-row="archived"]')).toBeNull();
  act(() => { clickControl(find(root, '[aria-label="Sidebar"] [role=tab][aria-label="Archived"]')); });
  expect(root.querySelector('[data-region=sidebar] [data-row="active"]')).toBeNull();
  act(() => { find(root, '[data-row="archived"]').click(); });
  expect(store.get().selected).toBe('archived');
  expect(sendDisabled(root)).toBe(true);
  act(() => { find(root, '[data-row="child"]').click(); });
  expect(store.get().selected).toBe('child');
  expect(sendDisabled(root)).toBe(true);
  act(() => { find(root, '[data-row="archived"]').dispatchEvent(new MouseEvent('contextmenu', {bubbles: true})); });
  const deleteItem = findAll(document, '[role=menuitem]').find(item => item.textContent === 'Delete Session');
  if (!deleteItem) throw new Error('No Delete Session item');
  act(() => { clickControl(deleteItem); });
  const confirmation = find(document, '[role=alertdialog]');
  expect(confirmation.textContent).toContain("Its chat and its children's chats are deleted.");
  expect(requests.filter(request => request.method === 'POST')).toHaveLength(0);
  await act(async () => { buttonNamed(confirmation, 'Delete').click(); });
  expect(requests.at(-1)?.params).toEqual({action: 'delete', id: 'archived'});
});

test('the desktop app\'s menus get the platform\'s bindings', () => {
  answer(() => Response.json([]));
  const bindingsOf = (platform: string) => {
    const posted: {type: string; items: {id: string; bindings: Record<string, unknown>[]}[]}[] = [];
    globalThis.hypercube = testBridge({platform, postMessage: message => posted.push(message as typeof posted[number])});
    const store = appStore();
    mount(store, <App store={store} />);
    unmountAll();
    return Object.fromEntries(posted.find(message => message.type === 'shortcuts')!.items.map(item => [item.id, item.bindings]));
  };
  const mac = bindingsOf('darwin');
  expect(mac.next).toEqual([
    {key: 'ArrowDown', metaKey: true, altKey: true, ctrlKey: false, shiftKey: false},
    {key: 'tab', metaKey: false, altKey: false, ctrlKey: true, shiftKey: false},
  ]);
  const linux = bindingsOf('linux');
  expect(linux.next).toEqual([
    {key: 'pagedown', metaKey: false, altKey: false, ctrlKey: true, shiftKey: false},
    {key: 'tab', metaKey: false, altKey: false, ctrlKey: true, shiftKey: false},
  ]);
  expect(linux.previous?.[0]).toEqual({key: 'pageup', metaKey: false, altKey: false, ctrlKey: true, shiftKey: false});
  expect(Object.values(linux).flat().some(b => b.metaKey)).toBe(false);
  expect(linux.sidebar).toEqual([{key: 'b', metaKey: false, altKey: false, ctrlKey: true, shiftKey: false}]);
});


test('workspace heading opens archived sessions and shared tools; its chevron collapses independently', async () => {
  const calls: URL[] = [];
  stubFetch(url => {
    const request = new URL(url, 'http://localhost/');
    calls.push(request);
    if (request.pathname === '/files/tree') return Response.json({'': [{name: 'main.go', dir: false}]});
    if (request.pathname === '/changes') return Response.json({branch: 'feature', files: [{path: 'main.go', status: 'modified', added: 1, removed: 1}]});
    if (request.pathname === '/mention-files') return Response.json(['main.go']);
    if (request.pathname === '/search-files') return Response.json([{path: 'main.go', lines: [{line: 3, parts: ['', 'needle', '']}]}]);
    return new Response('source\nneedle');
  });
  const sessions = Array.from({length: 12}, (_, i) => testSession({id: `s${i}`, title: `Session ${i}`, ended: i === 1}));
  const store = appStore({selected: 's0', workbench: workbench({rightPanel: true})}, ...sessions);
  const root = mount(store, <App store={store} />);
  await act(async () => clickControl(buttonNamed(root, 'repo')));
  const dashboard = find(root, '[aria-label="Workspace dashboard"]');
  expect(dashboard.className).not.toContain('max-w-');
  expect([...dashboard.querySelectorAll('[class*="max-w-"]')].filter(e => !e.closest('[aria-label="Working directory and branch"]'))
    .map(e => e.className).filter(name => /max-w-(?!full\b)/.test(name))).toEqual([]);
  expect([...dashboard.querySelectorAll('[aria-label="Archived sessions"] li')].map(li => li.textContent)).toEqual(['Session 1']);
  expect(store.get().session).toBeNull();
  expect(document.title).toBe('repo — Hypercube');
  expect(decodeURIComponent(location.hash.slice(1))).toBe('workspace:w');
  await act(async () => clickControl(find(root, '[aria-label="Collapse repo"]')));
  expect(store.get().selected).toBe('workspace:w');
  expect(store.get().workbench.collapsedWorkspaces.w).toBe(true);
  expect(find(root, '[aria-label="Expand repo"]')).toBeDefined();
  await act(async () => clickControl(find(root, '[role=treeitem][data-row="main.go"]')));
  expect(store.get().editor?.tabs.preview).toBe('file:main.go');
  await act(async () => clickControl(find(root, '[data-region=rightPanel] [role=tab][aria-label="Git"]')));
  expect(store.get().workbench.rightTab).toBe('git');
  const git = find(root, '[aria-label="Git changes"]');
  expect(git.closest('[role=tabpanel]')?.hasAttribute('hidden')).toBe(false);
  await act(async () => clickControl(find(git, '[aria-label="main.go, Modified, 1 added, 1 removed"] [aria-hidden="true"] span')));
  expect(store.get().editor?.tabs.preview).toBe('diff:main.go');
  const toolRequests = calls.filter(url => ['/files/tree', '/file', '/changes', '/diff'].includes(url.pathname) && url.searchParams.has('workspace'));
  expect(toolRequests.map(url => url.pathname)).toEqual(expect.arrayContaining(['/files/tree', '/file', '/changes', '/diff']));
  expect(toolRequests.every(url => url.searchParams.get('workspace') === 'w' && !url.searchParams.has('session'))).toBe(true);
  await act(async () => clickControl(find(root, '[data-region=rightPanel] [aria-label="Search"]')));
  const search = element(root, '[aria-label="Search files contents"]', HTMLInputElement);
  await act(async () => { setValue(search, 'needle'); search.dispatchEvent(new Event('input', {bubbles: true})); });
  await act(async () => clickControl(find(root, 'button[title="needle"]')));
  expect(store.get().editor?.files.open['main.go'].lines).toEqual([{start: 3, end: 3}]);
  expect(calls.find(url => url.pathname === '/search-files')?.searchParams.get('workspace')).toBe('w');
  act(() => store.update(state => showMain(selectWorkspace(state, 'w'))));
  await act(async () => clickControl(buttonNamed(find(root, '[aria-label="Workspace dashboard"]'), 'Session 1')));
  expect(store.get().selected).toBe('s1');
  expect(store.get().session?.id).toBe('s1');
  await act(async () => clickControl(find(root, '[data-region=rightPanel] [role=tab][aria-label="Git"]')));
  await act(async () => clickControl(find(root, '[aria-label="Git changes"] [aria-label="main.go, Modified, 1 added, 1 removed"]')));
  expect(calls.filter(url => url.pathname === '/diff').at(-1)?.searchParams.get('session')).toBe('s1');
  expect(calls.filter(url => url.pathname === '/diff').at(-1)?.searchParams.has('workspace')).toBe(false);
});


test('workspace without sessions has a dashboard empty state', async () => {
  stubFetch(url => Response.json(new URL(url, 'http://x/').pathname === '/files/tree' ? {'': []} : null));
  const store = appStore();
  store.update(state => selectWorkspace(state, 'w'));
  const root = mount(store, <App store={store} />);
  await act(async () => {});
  expect(find(root, '[aria-label="Workspace dashboard"]').textContent).toContain('No archived sessions.');
});
