import {afterEach, expect, mock, test} from 'bun:test';
import {act, useState} from 'react';
import {element, find, mount, setValue, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {ProfileSelect} from '../agent/profile-select.tsx';
import {unsentKey as workspaceDraftKey} from '../agent/drafts.ts';
import {openUnsent} from '../sessions/selection.ts';
import {createSession} from '../sessions/session-actions.ts';
import {createStore} from '../store.ts';
import {SettingsDialog} from './settings-dialog.tsx';
import type {Config} from './types.ts';

const unsentKey = workspaceDraftKey('w');

const config: Config = {default_profile: 'lead', profiles: [
  {name: 'lead', description: 'Large tasks', harness: 'claude', model: 'opus', effort: 'high'},
  {name: 'worker', description: 'Small tasks', harness: 'codex', model: 'sol', effort: 'low'},
]};
afterEach(() => { mock.restore(); unmountAll(); });
const settle = () => act(() => new Promise<void>(resolve => setImmediate(resolve)));
const button = (name: string) => {
  const found = [...document.querySelectorAll('button')].find(b => b.textContent === name);
  if (!found) throw new Error('Missing button: ' + name);
  return found;
};
const click = async (name: string) => { await act(async () => button(name).click()); };
const input = (id: string, value: string) => {
  const field = id === 'profile-description' ? element(document, '#' + id, HTMLTextAreaElement) : element(document, '#' + id, HTMLInputElement);
  act(() => { setValue(field, value); field.dispatchEvent(new Event('input', {bubbles: true})); });
};
const select = (id: string, value: string) => {
  const field = element(document, '#' + id, HTMLSelectElement);
  act(() => { field.value = value; field.dispatchEvent(new Event('change', {bubbles: true})); });
};
function View() {
  const [open, setOpen] = useState(true);
  return <><button onClick={() => setOpen(true)}>Open settings</button><SettingsDialog open={open} onOpenChange={setOpen} /></>;
}
const open = async () => { mount(createStore(testState()), <View />); await settle(); };

test('profile create, update, delete and default selection save as one config', async () => {
  const posts: Config[] = [];
  stubFetch((url, init) => {
    if (init?.method === 'POST') { posts.push(JSON.parse(String(init.body))); return new Response(null, {status: 204}); }
    return url === 'version' ? new Response('test build') : Response.json(config);
  });
  await open();
  await click('Agent profiles');
  input('profile-name', 'renamed');
  input('profile-model', 'sonnet');
  await click('Add profile');
  input('profile-name', 'new-profile');
  input('profile-description', 'New tasks');
  input('profile-model', 'new-model');
  input('profile-effort', 'medium');
  select('profile-harness', 'pi');
  await click('worker');
  await click('Remove profile');
  await click('General');
  select('default-profile', '1');
  expect(posts).toHaveLength(0);
  await click('Save changes');
  expect(posts).toEqual([{default_profile: 'new-profile', profiles: [
    {...config.profiles[0], name: 'renamed', model: 'sonnet'},
    {name: 'new-profile', description: 'New tasks', harness: 'pi', model: 'new-model', effort: 'medium'},
  ]}]);
  expect(Boolean(document.querySelector('[role=dialog][data-state=open]'))).toBe(false);
});

test('cancel discards edits and reopening reloads the saved config', async () => {
  const fetches = stubFetch(url => url === 'version' ? new Response('build') : Response.json(config));
  await open();
  await click('Agent profiles');
  input('profile-name', 'discard-me');
  await click('Cancel');
  await click('Open settings');
  await click('Agent profiles');
  expect(element(document, '#profile-name', HTMLInputElement).value).toBe('lead');
  expect(fetches.mock.calls.filter(([, init]) => init?.method === 'POST')).toHaveLength(0);
});

test('default follows its profile through rename and deletion, and the last profile remains', async () => {
  const posts: Config[] = [];
  stubFetch((url, init) => {
    if (init?.method === 'POST') { posts.push(JSON.parse(String(init.body))); return new Response(null, {status: 204}); }
    return url === 'version' ? new Response('build') : Response.json(config);
  });
  await open();
  select('default-profile', '1');
  await click('Agent profiles');
  input('profile-name', 'worker');
  input('profile-name', 'renamed');
  await click('General');
  expect(element(document, '#default-profile', HTMLSelectElement).value).toBe('1');
  await click('Agent profiles');
  await click('worker (default)');
  input('profile-name', 'default-renamed');
  await click('General');
  expect(element(document, '#default-profile', HTMLSelectElement).selectedOptions[0].textContent).toBe('default-renamed');
  await click('Agent profiles');
  await click('default-renamed (default)');
  await click('Remove profile');
  expect(button('Remove profile').disabled).toBe(true);
  await click('Save changes');
  expect(posts[0].default_profile).toBe('renamed');
});

test('a refused save keeps the edits and shows the server reason', async () => {
  stubFetch((url, init) => init?.method === 'POST' ? new Response('duplicate profile "worker"', {status: 400}) : url === 'version' ? new Response('build') : Response.json(config));
  await open();
  await click('Agent profiles');
  input('profile-name', 'worker');
  await click('Save changes');
  expect(find(document, '[role=alert]').textContent).toContain('duplicate profile "worker"');
  expect(element(document, '#profile-name', HTMLInputElement).value).toBe('worker');
  expect(button('Save changes').disabled).toBe(false);
});

test('saving holds the modal open until the write completes', async () => {
  let complete!: (response: Response) => void;
  stubFetch((url, init) => init?.method === 'POST' ? new Promise<Response>(resolve => { complete = resolve; }) : url === 'version' ? new Response('build') : Response.json(config));
  await open();
  await click('Save changes');
  expect(button('Cancel').disabled).toBe(true);
  expect(button('Saving…').disabled).toBe(true);
  expect(document.querySelector('[data-slot=dialog-close]')).toBeNull();
  await act(async () => complete(new Response(null, {status: 204})));
  await settle();
  expect(Boolean(document.querySelector('[role=dialog][data-state=open]'))).toBe(false);
});

test.each(['remove', 'rename', 'keep'] as const)('saving settings reconciles a draft choice (%s) before sending without opening its menu', async change => {
  let saved = config;
  const created: string[] = [];
  stubFetch((url, init) => {
    if (url === 'config' && init?.method === 'POST') {
      saved = JSON.parse(String(init.body));
      return new Response(null, {status: 204});
    }
    if (url.startsWith('session?')) {
      created.push(new URL(url, 'http://test/').searchParams.get('profile')!);
      return Response.json({session: 'created'});
    }
    return url === 'version' ? new Response('build') : Response.json(saved);
  });
  const store = createStore(openUnsent(testState({workspaces: [testWorkspace({id: 'w'})]}), 'w'));
  const root = mount(store, <><ProfileSelect session={testSession({id: unsentKey})} /><View /></>);
  await settle();
  expect(store.get().unsent?.profile).toBe('lead');
  select('default-profile', '1');
  await click('Agent profiles');
  if (change === 'remove') await click('Remove profile');
  if (change === 'rename') input('profile-name', 'renamed');
  if (change === 'keep') {
    input('profile-model', 'sonnet');
    input('profile-effort', 'low');
  }
  await click('Save changes');
  const expected = change === 'keep' ? 'lead' : 'worker';
  expect(store.get().unsent?.profile).toBe(expected);
  expect(element(root, '[aria-label=Profile]', HTMLButtonElement).textContent).toBe(change === 'keep' ? 'Sonnet · Low' : 'Sol · Low');
  await act(async () => { await createSession(store, {text: 'Start the draft', images: []}, unsentKey); });
  expect(created).toEqual([expected]);
});
