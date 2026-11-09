import {afterEach, expect, mock, test} from 'bun:test';
import {act} from 'react';
import {clickControl, element, findAll, mount, setValue, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {createStore} from '../store.ts';
import type {ModelEntry} from '../agent/types.ts';
import {SettingsModels} from './settings-models.tsx';

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

test('adding immediately saves ordered effort levels and the chosen default', async () => {
  const posts: ModelEntry[] = [];
  stubFetch((url, init) => {
    if (url === 'harnesses?') return Response.json([{id: 'claude', name: 'Claude'}]);
    const entry = JSON.parse(String(init?.body)); posts.push(entry);
    return Response.json([{...entry, id: 'new'}]);
  });
  const store = createStore(testState({modelEntries: []}));
  mount(store, <SettingsModels />);
  await click('Add Model');
  await select('model-harness', 'Claude');
  write('model-id', 'native/id');
  write('model-title', 'Friendly title');
  expect(document.querySelector('#model-default-effort')).toBeNull();
  write('model-efforts', 'low');
  expect(document.querySelector('#model-default-effort')).toBeNull();
  write('model-efforts', 'low, medium, high');
  // Until a default is chosen, it follows the last level typed.
  expect(element(document, '#model-default-effort', HTMLButtonElement).textContent).toBe('high');
  await select('model-default-effort', 'medium');
  write('model-efforts', 'medium, high');
  expect(element(document, '#model-default-effort', HTMLButtonElement).textContent).toBe('medium');
  await click('Save');
  expect(posts).toEqual([{id: '', harness: 'claude', model: 'native/id', title: 'Friendly title', efforts: ['medium', 'high'], default_effort: 'medium'}]);
  expect(store.get().modelEntries?.[0]?.id).toBe('new');
  expect(document.body.textContent).toContain('Friendly title');
  expect(document.body.textContent).toContain('medium (default) · high');
  expect(document.querySelector('[role=dialog]')).toBeNull();
});

test('new levels default to the last typed level and a refused save keeps the form', async () => {
  stubFetch((url) => url === 'harnesses?' ? Response.json([{id: 'claude'}]) : new Response('Claude already lists that model', {status: 409}));
  mount(createStore(testState({modelEntries: []})), <SettingsModels />);
  await click('Add Model');
  await select('model-harness', 'Claude');
  write('model-id', 'opus'); write('model-title', 'Opus'); write('model-efforts', 'low, medium, high');
  expect(element(document, '#model-default-effort', HTMLButtonElement).textContent).toBe('high');
  await click('Save');
  expect(document.querySelector('[role=alert]')?.textContent).toContain('already lists');
  expect(element(document, '#model-id', HTMLInputElement).value).toBe('opus');
});

test('editing preserves an unavailable harness and deleting leaves sessions unchanged', async () => {
  const entry: ModelEntry = {id: 'e', harness: 'uninstalled', model: 'native', title: 'Native Model', efforts: [], default_effort: ''};
  const fetches = stubFetch((url, init) => {
    if (url === 'harnesses?') return Response.json([{id: 'claude'}]);
    return Response.json(init?.method === 'DELETE' ? [] : [JSON.parse(String(init?.body))]);
  });
  const workspaces = [testWorkspace({id: 'w', sessions: [testSession({id: 's', harness: entry.harness, model: entry.model, effort: 'old'})]})];
  const store = createStore(testState({modelEntries: [entry], workspaces}));
  mount(store, <SettingsModels />);
  await click('Edit');
  expect(element(document, '#model-harness', HTMLButtonElement).textContent).toBe('uninstalled');
  expect(element(document, '#model-title', HTMLInputElement).value).toBe('Native Model');
  write('model-title', 'Renamed Model');
  await click('Save');
  expect(store.get().modelEntries).toEqual([{...entry, title: 'Renamed Model'}]);
  await click('Delete');
  expect(store.get().modelEntries).toEqual([]);
  expect(store.get().workspaces).toBe(workspaces);
  expect(fetches.mock.calls.some(([url, init]) => url === 'models/e' && init?.method === 'DELETE')).toBe(true);
});
