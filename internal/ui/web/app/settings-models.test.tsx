import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {clickControl, element, findAll, mount, setValue, stubFetch, unmountAll} from '../../test-render.tsx';
import {testConfig, testSession, testState, testWorkspace} from '../../test-state.ts';
import type {ModelEntry} from '../agent/types.ts';
import {createStore, useStore} from '../store.ts';
import {SettingsDialog} from './settings-dialog.tsx';
import type {Config} from './types.ts';

afterEach(() => { mock.restore(); unmountAll(); });
const settle = () => act(() => new Promise<void>(resolve => setImmediate(resolve)));
const button = (name: string) => {
  const found = findAll(document, 'button').find(b => b.textContent === name);
  if (!found) throw new Error('Missing button ' + name);
  return found;
};
const click = async (name: string) => { clickControl(button(name)); await settle(); };
const write = (id: string, text: string) => act(() => {
  const input = element(document, '#' + id, HTMLInputElement);
  setValue(input, text); input.dispatchEvent(new Event('input', {bubbles: true}));
});
const select = async (id: string, text: string) => {
  const key = (target: Element) => act(() => target.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true, cancelable: true})));
  key(element(document, '#' + id, HTMLButtonElement));
  key(findAll(document, '[role=option]').find(o => o.textContent === text)!);
  await settle();
};
const alert = () => document.querySelector('[role=alert]')?.textContent;

const saved: ModelEntry = {harness: 'claude', model: 'opus', title: 'Opus', efforts: ['low', 'high'], default_effort: 'high'};
const config = testConfig({models: [saved]});

function View() {
  const [, store] = useStore();
  return <><button onClick={() => store.update(s => ({...s, settings: 'Agents'}))}>Open settings</button><SettingsDialog /></>;
}

// openAgents opens Settings › Agents against a server holding served, and
// returns the store, the fetch stub, and the configs it was sent.
async function openAgents(state = testState(), served = config) {
  const posts: Config[] = [];
  const fetches = stubFetch((url, init) => {
    if (url === 'harnesses?') return Response.json([{id: 'claude', name: 'Claude'}]);
    if (url === 'config' && init?.method === 'POST') { posts.push(JSON.parse(String(init.body))); return new Response(null, {status: 204}); }
    return url === 'version' ? new Response('build') : Response.json(served);
  });
  const store = createStore({...state, settings: 'Agents', config: served});
  mount(store, <View />);
  await settle();
  return {store, fetches, posts};
}

test('added entries apply with Save changes', async () => {
  const {store, fetches, posts} = await openAgents();
  await click('Add Model');
  await select('model-harness', 'Claude');
  write('model-id', ' native/id ');
  write('model-title', 'Friendly title');
  write('model-efforts', 'low, medium, high,');
  await select('model-default-effort', 'medium');
  await click('Save');
  expect(document.body.textContent).toContain('Friendly title');
  expect(document.body.textContent).toContain('low · medium (default) · high');
  expect(fetches.mock.calls.map(([url, init]) => `${init?.method ?? 'GET'} ${url}`).filter(call => call !== 'GET config')).toEqual(['GET harnesses?']);
  expect(store.get().config?.models).toEqual([saved]);
  await click('Save changes');
  const models = [saved, {harness: 'claude', model: 'native/id', title: 'Friendly title', efforts: ['low', 'medium', 'high'], default_effort: 'medium'}];
  expect(posts.map(posted => posted.models)).toEqual([models]);
  expect(store.get().config?.models).toEqual(models);
});

test('Cancel discards added, edited, and deleted entries', async () => {
  const {store, posts} = await openAgents();
  await click('Add Model');
  await select('model-harness', 'Claude');
  write('model-id', 'sonnet'); write('model-title', 'Sonnet');
  await click('Save');
  clickControl(element(document, '[aria-label="Edit Claude Opus"]', HTMLButtonElement)); await settle();
  write('model-title', 'Renamed');
  await click('Save');
  clickControl(element(document, '[aria-label="Delete Claude Sonnet"]', HTMLButtonElement)); await settle();
  expect(document.body.textContent).toContain('Renamed');
  expect(document.body.textContent).not.toContain('Sonnet');
  await click('Cancel');
  expect(posts).toEqual([]);
  expect(store.get().config?.models).toEqual([saved]);
  await click('Open settings');
  expect(document.querySelector('[aria-label="Edit Claude Opus"]')).not.toBeNull();
  expect(document.body.textContent).not.toContain('Renamed');
});

test('the entry dialog refuses listed models and repeated levels, and the default follows the last level', async () => {
  await openAgents();
  await click('Add Model');
  await select('model-harness', 'Claude');
  write('model-id', 'opus'); write('model-title', 'Another Opus');
  expect(document.querySelector('#model-default-effort')).toBeNull();
  write('model-efforts', 'low');
  expect(document.querySelector('#model-default-effort')).toBeNull();
  write('model-efforts', 'low, medium, high');
  expect(element(document, '#model-default-effort', HTMLButtonElement).textContent).toBe('high');
  await select('model-default-effort', 'medium');
  write('model-efforts', 'medium, high');
  expect(element(document, '#model-default-effort', HTMLButtonElement).textContent).toBe('medium');
  await click('Save');
  expect(alert()).toBe('Claude already lists opus.');
  expect(element(document, '#model-title', HTMLInputElement).value).toBe('Another Opus');
  write('model-id', 'sonnet'); write('model-efforts', 'low, high, low');
  await click('Save');
  expect(alert()).toBe('Effort level "low" repeats.');
  expect(document.querySelector('[role=dialog] #model-id')).not.toBeNull();
});

test('editing keeps an uninstalled harness and deleting leaves sessions unchanged', async () => {
  const workspaces = [testWorkspace({id: 'w', sessions: [testSession({id: 's', harness: 'claude', model: 'opus', effort: 'old'})]})];
  const {store, posts} = await openAgents(testState({workspaces}), testConfig({models: [{...saved, harness: 'uninstalled'}]}));
  clickControl(element(document, '[aria-label="Edit uninstalled Opus"]', HTMLButtonElement)); await settle();
  expect(element(document, '#model-harness', HTMLButtonElement).textContent).toBe('uninstalled');
  write('model-title', 'Renamed');
  await click('Save');
  expect(document.body.textContent).toContain('Renamed');
  clickControl(element(document, '[aria-label="Delete uninstalled Renamed"]', HTMLButtonElement)); await settle();
  expect(document.body.textContent).toContain('No models.');
  await click('Save changes');
  expect(posts.map(posted => posted.models)).toEqual([[]]);
  expect(store.get().workspaces).toBe(workspaces);
});
