import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {clickControl, element, find, findAll, mount, stubFetch, unmountAll} from '../../test-render.tsx';
import {testConfig, testSession, testState, testWorkspace} from '../../test-state.ts';
import {openUnsent, receivedWorkspaces} from '../sessions/selection.ts';
import {typed, unsentKey} from '../agent/drafts.ts';
import type {Session} from '../sessions/types.ts';
import type {State} from '../state.ts';
import {createStore} from '../store.ts';
import {App} from './app.tsx';
import type {Workbench} from './types.ts';

afterEach(() => {
  mock.restore();
  unmountAll();
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

const sendDisabled = (root: ParentNode) => {
  const button = find(root, '.composer [title=Send]');
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
  const field = find(root, '.composer textarea');
  await act(async () => { expect(keydown(field, 'Escape').defaultPrevented).toBe(true); });
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
  act(() => find(root, '.composer textarea').blur());
  expect(document.activeElement).toBe(document.body);
  expect(keydown(document.body, 'b', {metaKey: true}).defaultPrevented).toBe(true);
  expect(store.get().workbench.sidebar).toBe(false);
  expect(keydown(document.body, 'p', {metaKey: true}).defaultPrevented).toBe(true);
  expect(document.querySelector('[role=dialog]')).not.toBeNull();
  keydown(document.activeElement ?? document.body, 'Escape');
  await act(async () => { keydown(document.body, 'Escape'); });
  expect(requests.some(request => request.params.action === 'interrupt')).toBe(true);
});

test('a new session draft is a sidebar row that reopens its prompt and can be discarded', async () => {
  answer(() => Response.json(testConfig()));
  const store = appStore({selected: 'a'}, testSession({id: 'a'}));
  const root = mount(store, <App store={store} />);
  const draftRow = () => root.querySelector<HTMLElement>(`.sidebar [data-session="${unsentKey('w')}"]`);
  act(() => store.update(state => openUnsent(state, 'w')));
  act(() => find(root, '.sidebar [data-session="a"]').click());
  expect(draftRow()).toBeNull();

  act(() => store.update(state => typed(openUnsent(state, 'w'), unsentKey('w'), 'finish this later')));
  expect(draftRow()?.getAttribute('aria-selected')).toBe('true');
  act(() => find(root, '.sidebar [data-session="a"]').click());
  expect(store.get().selected).toBe('a');
  act(() => draftRow()!.click());
  expect(element(root, '.composer textarea', HTMLTextAreaElement).value).toBe('finish this later');

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
  expect(root.querySelector('[data-session="archived"]')).toBeNull();
  act(() => { clickControl(find(root, '[aria-label="Sidebar"] [role=tab][aria-label="Archived"]')); });
  expect(root.querySelector('.sidebar [data-session="active"]')).toBeNull();
  act(() => { find(root, '[data-session="archived"]').click(); });
  expect(store.get().selected).toBe('archived');
  expect(sendDisabled(root)).toBe(true);
  act(() => { find(root, '[data-session="child"]').click(); });
  expect(store.get().selected).toBe('child');
  expect(sendDisabled(root)).toBe(true);
  act(() => { find(root, '[data-session="archived"]').dispatchEvent(new MouseEvent('contextmenu', {bubbles: true})); });
  const deleteItem = findAll(document, '[role=menuitem]').find(item => item.textContent === 'Delete Session');
  if (!deleteItem) throw new Error('No Delete Session item');
  act(() => { clickControl(deleteItem); });
  const confirmation = find(document, '[role=alertdialog]');
  expect(confirmation.textContent).toContain("Its chat and its children's chats are deleted.");
  expect(requests.filter(request => request.method === 'POST')).toHaveLength(0);
  await act(async () => { buttonNamed(confirmation, 'Delete').click(); });
  expect(requests.at(-1)?.params).toEqual({action: 'delete', id: 'archived'});
});
