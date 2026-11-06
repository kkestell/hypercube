import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {clickControl, element, find, findAll, mount, setValue, stubFetch, unmountAll} from '../../test-render.tsx';
import {testConfig, testSession, testState, testWorkspace} from '../../test-state.ts';
import {openUnsent, receivedWorkspaces} from '../sessions/selection.ts';
import {createSession} from '../sessions/session-actions.ts';
import type {Session} from '../sessions/types.ts';
import {createStore} from '../store.ts';
import {typed, unsentKey as workspaceDraftKey} from './drafts.ts';
import {SessionSelection} from './session-selection.tsx';
import type {Catalog, Harness} from './types.ts';

const unsentKey = workspaceDraftKey('w');
afterEach(() => { mock.restore(); unmountAll(); });
const settle = () => act(() => new Promise<void>(resolve => setTimeout(resolve)));

const harnesses: Harness[] = [
  {id: 'claude', name: 'Claude Code', installed: true},
  {id: 'codex', name: 'Codex', installed: true},
  {id: 'pi', name: 'Pi', installed: false},
];
// The stand-in catalogs: Claude's opus has efforts, and its plain model has
// none; Codex offers sol.
const catalogs: Record<string, Catalog> = {
  claude: {models: [{id: 'opus', label: 'Opus'}, {id: 'plain', label: 'Plain', provider: 'Anthropic'}], default: 'opus'},
  'claude/opus': {models: [], model: 'opus', efforts: ['low', 'high', 'max'], default_effort: 'high'},
  'claude/plain': {models: [], model: 'plain'},
  codex: {models: [{id: 'sol', label: 'Sol', provider: 'OpenAI'}], default: 'sol'},
  'codex/sol': {models: [], model: 'sol', efforts: ['low', 'medium'], default_effort: 'medium'},
};

// serve answers the picker's requests from the stand-in catalogs and records
// every request.
function serve(answer: (url: string, init?: RequestInit) => Response | Promise<Response> | undefined = () => undefined) {
  return stubFetch((url, init) => {
    const custom = answer(url, init);
    if (custom) return custom;
    if (url === 'harnesses?') return Response.json(harnesses);
    if (url.startsWith('models?')) {
      const q = new URL(url, 'http://x/').searchParams;
      const key = q.get('model') ? `${q.get('harness')}/${q.get('model')}` : q.get('harness')!;
      return catalogs[key] ? Response.json(catalogs[key]) : new Response(`model ${q.get('model')} is not offered`, {status: 409});
    }
    if (url.startsWith('session?')) return Response.json({session: 'created'});
    return Response.json([]);
  });
}

const trigger = (root: ParentNode) => element(root, 'button[aria-label^="Model"]', HTMLButtonElement);
const open = async (root: ParentNode) => { clickControl(trigger(root)); await settle(); await settle(); };
const item = (label: string) => {
  const found = findAll(document, '[cmdk-item]').find(i => i.textContent?.startsWith(label));
  if (!found) throw new Error('Missing model: ' + label);
  return found;
};
const choose = async (label: string) => { clickControl(item(label)); await settle(); await settle(); };
const chooseOption = async (label: string, option: string) => {
  const key = (target: Element) => act(() => { target.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true, cancelable: true})); });
  key(element(document, `[aria-label="${label}"]`, HTMLButtonElement));
  const found = findAll(document, '[role=option]').find(o => o.textContent === option);
  if (!found) throw new Error('Missing option: ' + option);
  key(found);
  await settle();
};
const queryOf = (url: string) => Object.fromEntries(new URL(url, 'http://x/').searchParams);

const unsentStore = (sessions: Session[] = []) => createStore(openUnsent(testState({
  config: testConfig({default_selection: {harness: 'claude', model: 'opus', effort: 'high'}}),
  workspaces: [testWorkspace({id: 'w', path: '/repo', sessions})],
}), 'w'));

test('a new session starts from the newest session in its workspace, then the default', () => {
  serve();
  const fresh = unsentStore();
  const root = mount(fresh, <SessionSelection session={testSession({id: unsentKey})} />);
  expect(trigger(root).title).toBe('Claude opus, effort high');
  unmountAll();
  const used = unsentStore([testSession({id: 'n', harness: 'codex', model: 'sol', effort: ''})]);
  const second = mount(used, <SessionSelection session={testSession({id: unsentKey})} />);
  expect(trigger(second).title).toBe('Codex sol, default effort');
});

test('choosing a harness, a searched model, and an effort sets the draft without losing its text', async () => {
  const fetches = serve();
  const s = unsentStore();
  s.update(state => typed(state, unsentKey, 'keep this'));
  const root = mount(s, <SessionSelection session={testSession({id: unsentKey})} />);
  expect(fetches).not.toHaveBeenCalled();
  await open(root);
  // Only installed harnesses are offered.
  await chooseOption('Harness', 'Codex');
  expect(item('Sol').textContent).toContain('OpenAI · sol');
  await choose('Sol');
  expect(s.get().unsent?.settings).toEqual({harness: 'codex', model: 'sol', effort: 'medium'});
  await chooseOption('Effort', 'low');
  expect(s.get().unsent?.settings).toEqual({harness: 'codex', model: 'sol', effort: 'low'});
  await chooseOption('Effort', 'Harness default');
  expect(s.get().unsent?.settings).toEqual({harness: 'codex', model: 'sol', effort: ''});
  expect(s.get().drafts[unsentKey].text).toBe('keep this');
  const asked = fetches.mock.calls.map(([url]) => url).filter(url => url.startsWith('models?')).map(queryOf);
  expect(asked).toContainEqual({workspace: 'w', harness: 'codex'});
  expect(asked).toContainEqual({workspace: 'w', harness: 'codex', model: 'sol'});
  await act(async () => { await createSession(s, {text: 'keep this', images: []}, unsentKey); });
  const created = fetches.mock.calls.map(([url]) => url).find(url => url.startsWith('session?'))!;
  expect(queryOf(created)).toMatchObject({action: 'new', workspace: 'w', harness: 'codex', model: 'sol', effort: ''});
});

test('search narrows the models, and a model without efforts has no effort control', async () => {
  serve();
  const s = unsentStore();
  const root = mount(s, <SessionSelection session={testSession({id: unsentKey})} />);
  await open(root);
  const search = element(document, 'input[aria-label="Search models"]', HTMLInputElement);
  act(() => { setValue(search, 'anthropic'); search.dispatchEvent(new Event('input', {bubbles: true})); });
  await settle();
  expect(findAll(document, '[cmdk-item]').map(i => i.textContent)).toEqual(['PlainAnthropic · plain']);
  await choose('Plain');
  expect(s.get().unsent?.settings).toEqual({harness: 'claude', model: 'plain', effort: ''});
  expect(document.querySelector('[aria-label=Effort]')).toBeNull();
});

test('a running session changes its model within its harness', async () => {
  const fetches = serve();
  const s = createStore(testState());
  const running = testSession({id: 'a', harness: 'claude', model: 'plain', effort: '', ready: true});
  const root = mount(s, <SessionSelection session={running} />);
  await open(root);
  expect(document.querySelector('[aria-label=Harness]')).toBeNull();
  await choose('Opus');
  const posted = fetches.mock.calls.map(([url]) => url).find(url => url.startsWith('session?'))!;
  expect(queryOf(posted)).toEqual({action: 'selection', id: 'a', model: 'opus', effort: 'high'});
  expect(fetches.mock.calls.some(([url]) => url === 'harnesses?')).toBe(false);
  unmountAll();
  // A session that is not ready shows its settings and cannot change them.
  const starting = mount(s, <SessionSelection session={{...running, ready: false}} />);
  expect(trigger(starting).disabled).toBe(true);
  expect(trigger(starting).title).toBe('Claude plain, default effort');
});

test('discovery failures are shown and leave the selection as it was', async () => {
  serve(url => url.startsWith('models?') ? new Response('Claude Code is not signed in', {status: 409}) : undefined);
  const s = unsentStore();
  const root = mount(s, <SessionSelection session={testSession({id: unsentKey})} />);
  await open(root);
  expect(find(document, '[role=alert]').textContent).toBe('Claude Code is not signed in');
  expect(s.get().unsent?.settings).toEqual({harness: 'claude', model: 'opus', effort: 'high'});
  unmountAll();
  serve(url => url === 'harnesses?' ? Response.json([{id: 'pi', installed: false}]) : undefined);
  const none = unsentStore();
  none.update(state => ({...state, config: testConfig({default_selection: undefined}), unsent: state.unsent && {...state.unsent, settings: null}}));
  const empty = mount(none, <SessionSelection session={testSession({id: unsentKey})} />);
  expect(trigger(empty).textContent).toBe('Choose a model');
  await open(empty);
  expect(find(document, '[role=alert]').textContent).toBe('No harness is installed.');
});

test('a late catalog for another workspace is ignored', async () => {
  const waiting: ((response: Response) => void)[] = [];
  const fetches = serve(url => url.startsWith('models?') && queryOf(url).workspace === 'w'
    ? new Promise<Response>(resolve => { waiting.push(resolve); }) : undefined);
  const s = unsentStore();
  s.update(state => receivedWorkspaces(state, [...state.workspaces, testWorkspace({id: 'other', path: '/other'})]));
  const root = mount(s, <SessionSelection session={testSession({id: unsentKey})} />);
  await open(root);
  const first = fetches.mock.calls.findIndex(([url]) => url.startsWith('models?'));
  act(() => s.update(state => openUnsent(state, 'other')));
  await settle();
  expect(fetches.mock.calls[first][1]?.signal?.aborted).toBe(true);
  waiting[0](Response.json({models: [{id: 'stale', label: 'Stale'}]}));
  await settle();
  expect(findAll(document, '[cmdk-item]').some(i => i.textContent?.startsWith('Stale'))).toBe(false);
  expect(s.get().unsent).toMatchObject({workspace: 'other', settings: {harness: 'claude', model: 'opus', effort: 'high'}});
});

test('a child whose settings differ from its parent is marked', () => {
  serve();
  const child = testSession({id: 'c', parent: 'p', harness: 'claude', model: 'plain', effort: ''});
  const parent = testSession({id: 'p', harness: 'claude', model: 'opus', effort: 'high', children: [child]});
  const s = createStore(testState({workspaces: [testWorkspace({id: 'w', sessions: [parent]})]}));
  const root = mount(s, <SessionSelection session={child} />);
  expect(root.querySelector('[role=img][aria-label^="Settings differ"]')?.getAttribute('aria-label'))
    .toBe("Settings differ from the parent's: Claude plain, default effort; parent Claude opus, effort high");
  unmountAll();
  const same = mount(s, <SessionSelection session={{...child, model: 'opus', effort: 'high'}} />);
  expect(same.querySelector('[aria-label^="Settings differ"]')).toBeNull();
});

test('a model whose efforts cannot be listed keeps the selection and shows why until another choice', async () => {
  serve(url => queryOf(url).model === 'plain' ? new Response('effort discovery failed', {status: 409}) : undefined);
  const s = unsentStore();
  s.update(state => ({...state, unsent: state.unsent && {...state.unsent, settings: {harness: 'claude', model: 'opus', effort: 'high'}}}));
  const root = mount(s, <SessionSelection session={testSession({id: unsentKey})} />);
  await open(root);
  await choose('Plain');
  await settle();
  expect(s.get().unsent?.settings).toEqual({harness: 'claude', model: 'opus', effort: 'high'});
  expect(find(document, '[role=alert]').textContent).toBe('Could not choose plain: effort discovery failed');
  await choose('Opus');
  expect(document.querySelector('[role=alert]')).toBeNull();
  expect(s.get().unsent?.settings).toEqual({harness: 'claude', model: 'opus', effort: 'high'});
});

test("a failure to list the current model's efforts is shown", async () => {
  serve(url => queryOf(url).model === 'opus' ? new Response('effort discovery failed', {status: 409}) : undefined);
  const root = mount(unsentStore(), <SessionSelection session={testSession({id: unsentKey})} />);
  await open(root);
  expect(find(document, '[role=alert]').textContent).toBe('Could not list the efforts of opus: effort discovery failed');
  expect(document.querySelector('[aria-label=Effort]')).toBeNull();
});
