import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {clickControl, element, findAll, mount, setValue, stubFetch, unmountAll} from '../../test-render.tsx';
import {testConfig, testSession, testState, testWorkspace} from '../../test-state.ts';
import {openUnsent} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {typed, unsentKey} from './drafts.ts';
import {SessionSelection} from './session-selection.tsx';
import type {ModelEntry} from './types.ts';

afterEach(() => { mock.restore(); unmountAll(); });
const settle = () => act(() => new Promise<void>(resolve => setImmediate(resolve)));
export const entries: ModelEntry[] = [
  {id: 'a', harness: 'claude', model: 'opus', title: 'Opus', efforts: ['low', 'medium', 'high'], default_effort: 'medium'},
  {id: 'b', harness: 'claude', model: 'plain', title: 'Plain Model', efforts: [], default_effort: ''},
  {id: 'c', harness: 'opencode', model: 'sol', title: 'Sol', efforts: ['high'], default_effort: 'high'},
];
const draft = () => createStore(openUnsent(testState({modelEntries: entries,
  config: testConfig({default_selection: {harness: 'claude', model: 'opus', effort: 'medium'}}),
  workspaces: [testWorkspace({id: 'w'})],
}), 'w'));
const trigger = (root: ParentNode) => element(root, 'button[aria-label^="Model"]', HTMLButtonElement);
const open = async (root: ParentNode) => { clickControl(trigger(root)); await settle(); };
const choose = async (model: string) => {
  const item = findAll(document, '[cmdk-item]').find(i => i.textContent === model);
  if (!item) throw new Error('Missing model ' + model);
  clickControl(item); await settle();
};
const key = (target: Element, key: string) => act(() => { target.dispatchEvent(new KeyboardEvent('keydown', {key, bubbles: true, cancelable: true})); });

test('model labels follow title edits for the matching harness without changing session settings', () => {
  const s = draft();
  const settings = s.get().unsent?.settings;
  const root = mount(s, <SessionSelection session={testSession({id: unsentKey('w')})} />);
  expect(trigger(root).textContent).toContain('Opus');
  expect(trigger(root).title).toBe('Claude Opus, effort medium');
  act(() => s.update(state => ({...state, modelEntries: [
    {id: 'other', harness: 'opencode', model: 'opus', title: 'Other Opus', efforts: [], default_effort: ''},
    ...entries.map(e => e.id === 'a' ? {...e, title: 'Claude Opus'} : e),
  ]})));
  expect(trigger(root).textContent).toContain('Claude Opus');
  expect(trigger(root).getAttribute('aria-label')).toBe('Model: Claude Claude Opus, effort medium');
  expect(s.get().unsent?.settings).toBe(settings);
  act(() => s.update(state => ({...state, modelEntries: []})));
  expect(trigger(root).textContent).toContain('opus');
  expect(trigger(root).title).toBe('Claude opus, effort medium');
});

test('opening focuses search, shows harness tabs, and searches only the selected harness without requests', async () => {
  const fetches = stubFetch(() => Response.json([]));
  const s = draft();
  const root = mount(s, <SessionSelection session={testSession({id: unsentKey('w')})} />);
  await open(root);
  expect(findAll(document, '[role=tab]').map(t => t.getAttribute('aria-label'))).toEqual(['Claude', 'OpenCode']);
  expect(document.querySelector('[role=tab][data-state=active]')?.getAttribute('aria-label')).toBe('Claude');
  const search = element(document, 'input[aria-label="Search models"]', HTMLInputElement);
  expect(document.activeElement).toBe(search);
  act(() => { setValue(search, 'plain'); search.dispatchEvent(new Event('input', {bubbles: true})); });
  await settle();
  expect(findAll(document, '[cmdk-item]').map(i => i.textContent)).toEqual(['Plain Model']);
  act(() => { setValue(search, 'Model'); search.dispatchEvent(new Event('input', {bubbles: true})); });
  await settle();
  expect(findAll(document, '[cmdk-item]').map(i => i.textContent)).toEqual(['Plain Model']);
  expect(fetches).not.toHaveBeenCalled();
});

test('model choices reset effort, preserve draft text, and never fetch', async () => {
  const fetches = stubFetch(() => Response.json([]));
  const s = draft(); s.update(state => typed(state, unsentKey('w'), 'keep this'));
  const root = mount(s, <SessionSelection session={testSession({id: unsentKey('w')})} />);
  await open(root);
  await choose('Plain Model');
  expect(s.get().unsent?.settings).toEqual({harness: 'claude', model: 'plain', effort: ''});
  expect(document.querySelector('[role=slider]')).toBeNull();
  await choose('Opus');
  expect(s.get().unsent?.settings?.effort).toBe('medium');
  key(element(document, '[role=slider]', HTMLElement), 'ArrowRight'); await settle();
  expect(s.get().unsent?.settings?.effort).toBe('high');
  key(element(document, '[role=tab][aria-label=OpenCode]', HTMLButtonElement), 'Enter'); await settle();
  await choose('Sol');
  expect(s.get().unsent?.settings).toEqual({harness: 'opencode', model: 'sol', effort: 'high'});
  expect(document.querySelector('[role=slider]')).toBeNull();
  expect(document.body.textContent).toContain('Efforthigh');
  expect(s.get().drafts[unsentKey('w')].text).toBe('keep this');
  expect(fetches).not.toHaveBeenCalled();
});

test('a running session has no tabs and sends one selection per slider commit, without discovery', async () => {
  const fetches = stubFetch(() => new Response(''));
  const session = testSession({id: 's', harness: 'claude', model: 'opus', effort: 'medium', ready: true});
  const s = createStore(testState({modelEntries: entries, workspaces: [testWorkspace({id: 'w', sessions: [session]})]}));
  const root = mount(s, <SessionSelection session={session} />);
  await open(root);
  expect(document.querySelector('[role=tab]')).toBeNull();
  expect(findAll(document, '[cmdk-item]').map(i => i.textContent)).toEqual(['Opus', 'Plain Model']);
  expect(fetches).not.toHaveBeenCalled();
  key(element(document, '[role=slider]', HTMLElement), 'ArrowRight'); await settle();
  expect(fetches.mock.calls).toHaveLength(1);
  expect(Object.fromEntries(new URL(fetches.mock.calls[0]![0], 'http://x').searchParams)).toEqual({action: 'selection', id: 's', model: 'opus', effort: 'high'});
});

test('unlisted selections survive and empty pickers link to Agents settings without requests', async () => {
  const fetches = stubFetch(() => Response.json([]));
  const s = draft(); s.update(state => ({...state, modelEntries: []}));
  const root = mount(s, <SessionSelection session={testSession({id: unsentKey('w')})} />);
  expect(trigger(root).title).toBe('Claude opus, effort medium');
  await open(root);
  expect(document.body.textContent).toContain('No models configured.');
  clickControl(findAll(document, 'button').find(b => b.textContent === 'Open Settings')!);
  expect(s.get().settings).toBe('Agents');
  expect(s.get().unsent?.settings?.model).toBe('opus');
  unmountAll();
  const session = testSession({id: 's', harness: 'missing', model: 'old', effort: 'max', ready: true});
  const other = createStore(testState({modelEntries: entries}));
  const running = mount(other, <SessionSelection session={session} />);
  await open(running);
  expect(document.body.textContent).toContain('No missing models configured.');
  clickControl(findAll(document, 'button').find(b => b.textContent === 'Open Settings')!);
  expect(other.get().settings).toBe('Agents');
  expect(fetches).not.toHaveBeenCalled();
});

test('an edited effort remains visible until the user moves the slider', async () => {
  const fetches = stubFetch(() => Response.json([]));
  const s = draft(); s.update(state => ({...state, unsent: {...state.unsent!, settings: {harness: 'claude', model: 'opus', effort: 'removed'}}}));
  const root = mount(s, <SessionSelection session={testSession({id: unsentKey('w')})} />);
  await open(root);
  const slider = element(document, '[role=slider]', HTMLElement);
  expect(slider.getAttribute('aria-valuenow')).toBe('1');
  expect(slider.getAttribute('aria-valuetext')).toBe('removed');
  await choose('Opus');
  expect(s.get().unsent?.settings?.effort).toBe('removed');
  key(slider, 'ArrowRight'); await settle();
  expect(s.get().unsent?.settings?.effort).toBe('high');
  expect(fetches).not.toHaveBeenCalled();
});
