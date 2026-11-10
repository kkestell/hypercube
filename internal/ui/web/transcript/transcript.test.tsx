import {afterEach, expect, mock, spyOn, test} from 'bun:test';
import {act} from 'react';
import {clickControl, element, find, findAll, mount, setValue, stubFetch, unmountAll} from '../../test-render.tsx';
import {testSession, testState, testWorkspace} from '../../test-state.ts';
import {emptySession} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {Transcript} from './transcript.tsx';
import {receivedTranscript} from './transcripts.ts';
import type {Item, WorkState} from './types.ts';

afterEach(() => {
  mock.restore();
  unmountAll();
});

// requestParams are the query parameters of the nth fetch.
const requestParams = (calls: Parameters<typeof fetch>[], n: number) => new URL(String(calls[n][0]), 'http://x/').searchParams;

const busy = testSession({id: 's', activity: 'busy'});

type NewItem = Item & {seq?: number};

function setup(items: NewItem[], session = busy) {
  let seq = 0;
  const store = createStore(testState({session: emptySession('s')}));
  const send = (items: NewItem[], reset = false) => act(() => store.update(s => receivedTranscript(s, {type: 'transcript', session: 's', reset, items: items.map(item => ({...item, seq: item.seq ?? ++seq}))})));
  send(items, true);
  const root = mount(store, <Transcript session={session} />);
  return {store, root, send};
}

const marker = (id: string, state: WorkState, at: number, final?: string[]): Item => ({id: 'L1:' + id, kind: 'state', state, at, done: true, final});

const choose = (root: HTMLElement, choice: string) => {
  act(() => clickControl(find(root, `[role="radio"][data-choice="${choice}"]`)));
};
const submitButton = (root: HTMLElement) => find(root, '.attention-form-submit');
const submit = (root: HTMLElement) => act(() => clickControl(submitButton(root)));

test('an open proposal is pinned below the history, sends only its saved ID, and returns to history when started', async () => {
  const reply = Promise.withResolvers<Response>();
  const fetches = spyOn(globalThis, 'fetch').mockReturnValue(reply.promise);
  const {root, send} = setup([
    {id: 'L1:input', kind: 'user', text: 'Review this'},
    marker('working', 'working', 1),
    {id: 'L1:proposal', kind: 'proposal', text: 'Review separately', input: {name: 'review', title: 'Review separately', settings: {harness: 'codex', model: 'm', effort: 'low'}, dir: '.', message: 'Check the parser'}},
    {id: 'L1:answer', kind: 'text', text: 'I found a separate task.', done: true},
    marker('idle', 'idle', 2, ['L1:answer']),
  ]);
  const card = find(root, '.transcript-pending [aria-label="Proposed child session"]');
  expect(root.querySelector('.transcript-list [aria-label="Proposed child session"]')).toBeNull();
  expect(card.textContent).toContain('Check the parser');
  expect(card.textContent).toContain('Fresh context');
  act(() => find(card, 'button').click());
  expect(find(card, 'button').hasAttribute('disabled')).toBe(true);
  const params = requestParams(fetches.mock.calls, 0);
  expect(Object.fromEntries(params)).toEqual({action: 'start-proposal', id: 's', proposal: 'L1:proposal'});
  expect(fetches.mock.calls[0][1]?.body).toBeUndefined();
  await act(async () => reply.resolve(new Response('')));
  send([{id: 'L1:proposal', kind: 'proposal', text: 'Review separately', input: {name: 'review', title: 'Review separately', settings: {harness: 'codex', model: 'm', effort: 'low'}}, done: true, answer: 'child-id'}]);
  expect(root.querySelector('.transcript-pending')).toBeNull();
  const done = find(root, '.transcript-list [aria-label="Proposed child session"]');
  expect(done.textContent).toContain('Child session: review');
  expect([...done.querySelectorAll('button')].some(button => button.textContent === 'Start' || button.textContent === 'Dismiss')).toBe(false);
});

test('open and answered proposals show native model IDs without discovery', async () => {
  const fetches = stubFetch(() => Response.json([]));
  const {root, store, send} = setup([{id: 'p', kind: 'proposal', text: 'Task', input: {name: 'child', title: 'Task', settings: {harness: 'opencode', model: 'm', effort: 'low'}}}]);
  await act(async () => store.update(state => ({...state, workspaces: [testWorkspace({id: 'w', sessions: [busy]})]})));
  expect(find(root, '[title="OpenCode m, effort low"]').textContent).toContain('m');
  send([{id: 'p', kind: 'proposal', text: 'Task', done: true, answer: 'child', input: {name: 'child', title: 'Task', settings: {harness: 'opencode', model: 'm', effort: 'low'}}}]);
  expect(find(root, '[title="OpenCode m, effort low"]').textContent).toContain('m');
  expect(fetches).not.toHaveBeenCalled();
});

test('a refused proposal action can be retried or dismissed', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockResolvedValue(new Response('Could not start', {status: 400}));
  const {root} = setup([{id: 'p', kind: 'proposal', text: 'Task', input: {name: 'child', title: 'Task', settings: {harness: 'codex', model: 'm', effort: 'low'}}}]);
  const card = find(root, '[aria-label="Proposed child session"]');
  await act(async () => find(card, 'button').click());
  expect(find(card, 'button').hasAttribute('disabled')).toBe(false);
  fetches.mockResolvedValue(new Response(''));
  await act(async () => findAll(card, 'button')[1].click());
  expect(requestParams(fetches.mock.calls, 1).get('action')).toBe('dismiss-proposal');
});

test('an archived proposal cannot be started or dismissed', () => {
  const fetches = spyOn(globalThis, 'fetch');
  const {root} = setup([{id: 'p', kind: 'proposal', text: 'Task', input: {name: 'child', title: 'Task', settings: {harness: 'codex', model: 'm', effort: 'low'}}}], testSession({id: 's', ended: true, activity: 'exited'}));
  const buttons = findAll(root, '[aria-label="Proposed child session"] button');
  expect(buttons.map(button => button.textContent)).toEqual(['Start', 'Dismiss']);
  expect(buttons.every(button => button.hasAttribute('disabled'))).toBe(true);
  act(() => buttons.forEach(button => button.click()));
  expect(fetches).not.toHaveBeenCalled();
});

test('items replace themselves in place, and unchanged items are left alone', () => {
  const {root, send} = setup([{id: '1', kind: 'user', text: 'hi', seq: 1}, {id: '2', kind: 'text', text: 'Hel', seq: 2}]);
  const list = find(root, '.transcript-list');
  const [user, reply] = list.children;
  expect(user.textContent).toBe('hi');
  expect(reply.textContent).toBe('Hel');
  send([{id: '2', kind: 'text', text: 'Hello **there**', seq: 2}, {id: '3', kind: 'text', text: 'next', seq: 3}]);
  expect(list.children.length).toBe(3);
  expect(list.children[0]).toBe(user);
  expect(list.children[1]).toBe(reply);
  expect(reply.textContent).toBe('Hello there');
  expect(reply.querySelector('strong')).not.toBeNull();
});

test('tool, error, Thinking, and prompt text is shown as text', () => {
  const {root} = setup([
    {id: 'c', kind: 'tool', tool: '<b>tool</b>', input: {command: '<script>x</script>'}, output: '<img src=x onerror=1>', done: true},
    {id: 'e', kind: 'error', text: '<u>bad</u>'},
    {id: 'r', kind: 'reasoning', text: '<i>thinking</i>'},
    {id: 'u', kind: 'user', text: '<a href=x>me</a>'},
  ]);
  act(() => find(root, '.tool-items-call > [data-slot=collapsible-trigger]').click());
  expect(root.querySelectorAll('b, script, img, u, i, a').length).toBe(0);
  for (const text of ['<b>tool</b>', '<img src=x onerror=1>', '<u>bad</u>', '<i>thinking</i>', '<a href=x>me</a>']) expect(root.textContent).toContain(text);
});

test('an expanded running tool explains missing details and updates as they arrive', () => {
  const {root, send} = setup([{id: 'c', kind: 'tool', tool: 'bash', seq: 1}]);
  act(() => find(root, '.tool-items-call > [data-slot=collapsible-trigger]').click());
  expect(find(root, '.tool-items-body').textContent).toBe('Waiting for tool details…');

  send([{id: 'c', kind: 'tool', tool: 'bash', input: {command: 'pwd'}, seq: 1}]);
  expect(find(root, '.tool-items-input').textContent).toContain('pwd');
  expect(root.textContent).not.toContain('Waiting for tool details…');

  send([{id: 'c', kind: 'tool', tool: 'bash', input: {command: 'pwd'}, output: '/workspace', done: true, seq: 1}]);
  expect(find(root, '.tool-items-body').textContent).toContain('/workspace');
});

test('an expanded completed tool with no details says so', () => {
  const {root} = setup([{id: 'c', kind: 'tool', tool: 'bash', done: true}]);
  act(() => find(root, '.tool-items-call > [data-slot=collapsible-trigger]').click());
  expect(find(root, '.tool-items-body').textContent).toBe('No details recorded.');
});

test('an attention request offers its choices, waits while sending, and opens again when refused', async () => {
  const refusal = Promise.withResolvers<Response>();
  const fetches = spyOn(globalThis, 'fetch').mockReturnValue(refusal.promise);
  const {root, send} = setup([{id: 'q', kind: 'attention', text: 'Proceed?', choices: ['yes', 'no']}]);
  const rows = () => findAll(root, '[role="radio"][data-choice]');
  const list = () => find(root, '[role="radiogroup"]');
  expect(list().getAttribute('aria-label')).toBe('Choices');
  expect(rows().map(r => r.getAttribute('data-choice'))).toEqual(['yes', 'no']);
  expect(root.querySelector('input')).toBeNull();
  expect(submitButton(root).hasAttribute('disabled')).toBe(true);
  choose(root, 'yes');
  expect(fetches).not.toHaveBeenCalled();
  submit(root);
  expect(Object.fromEntries(requestParams(fetches.mock.calls, 0))).toEqual({action: 'answer', id: 's', request: 'q', choice: 'yes'});
  expect(rows().every(row => row.hasAttribute('disabled'))).toBe(true);
  choose(root, 'no');
  submit(root);
  expect(fetches).toHaveBeenCalledTimes(1);
  await act(async () => {
    refusal.resolve(new Response('no longer open', {status: 409}));
    await new Promise(resolve => setTimeout(resolve));
  });
  expect(rows().every(row => !row.hasAttribute('disabled'))).toBe(true);

  send([{id: 'q', kind: 'attention', text: 'Proceed?', done: true, answer: 'no', seq: 1}]);
  expect(rows()).toEqual([]);
  expect(find(root, '.attention-item').textContent).toContain('Answered');
  expect(root.querySelector('.attention-item-answer')).toBeNull();
  act(() => find(root, '.attention-item > [data-slot=collapsible-trigger]').click());
  expect(find(root, '.attention-item-question').textContent).toBe('Proceed?');
  expect(find(root, '.attention-item-answer').textContent).toBe('no');
});

test.each(['New session', 'Do it here'])('a task offer sends %s immediately, prevents duplicate answers, and records it', choice => {
  const fetches = spyOn(globalThis, 'fetch').mockReturnValue(new Promise<Response>(() => {}));
  const offer = {id: 'o', kind: 'attention' as const, offer: true, text: 'Fix the parser', choices: ['New session', 'Do it here']};
  const {root, send} = setup([offer]);
  const pending = find(root, '.transcript-pending');
  expect(pending.querySelector('[role="radio"], .attention-form-submit')).toBeNull();
  const buttons = findAll(pending, 'button');
  expect(buttons.map(button => button.getAttribute('aria-label') ?? button.textContent)).toEqual(['Dismiss task', 'New session', 'Do it here']);
  act(() => buttons.find(button => button.textContent === choice)!.click());
  expect(buttons.every(button => button.hasAttribute('disabled'))).toBe(true);
  act(() => buttons.forEach(button => button.click()));
  expect(fetches).toHaveBeenCalledTimes(1);
  expect(Object.fromEntries(requestParams(fetches.mock.calls, 0))).toEqual({action: 'answer', id: 's', request: 'o', choice});
  send([{...offer, done: true, answer: choice}]);
  expect(root.querySelector('.transcript-pending .attention-item')).toBeNull();
  expect(find(root, '.attention-item').textContent).toContain(choice);
});

test('current and saved two-choice offers can be dismissed and leave the pending area', () => {
  const fetches = spyOn(globalThis, 'fetch').mockReturnValue(new Promise<Response>(() => {}));
  for (const choices of [['New session', 'Do it here', 'Dismiss'], ['New session', 'Do it here']]) {
    const offer = {id: 'o', kind: 'attention' as const, offer: true, text: 'Fix the parser', choices};
    const {root, send} = setup([offer]);
    const pending = find(root, '.transcript-pending');
    act(() => find(pending, '[aria-label="Dismiss task"]').click());
    const call = fetches.mock.calls.at(-1)!;
    expect(Object.fromEntries(new URL(String(call[0]), 'http://x/').searchParams)).toEqual({action: 'answer', id: 's', request: 'o', choice: 'Dismiss'});
    send([{...offer, done: true, answer: 'Dismiss'}]);
    expect(root.querySelector('.transcript-pending')).toBeNull();
    expect(find(root, '.attention-item').textContent).toContain('Dismiss');
  }
  expect(fetches).toHaveBeenCalledTimes(2);
});

test('an offer in an archived transcript cannot be answered', () => {
  const fetches = spyOn(globalThis, 'fetch');
  const {root} = setup([{id: 'o', kind: 'attention', offer: true, text: 'Fix the parser', choices: ['New session', 'Do it here']}], testSession({id: 's', ended: true, activity: 'exited'}));
  const buttons = findAll(root, '.attention-item button');
  expect(buttons).toHaveLength(3);
  expect(buttons.every(button => button.hasAttribute('disabled'))).toBe(true);
  act(() => buttons.forEach(button => button.click()));
  expect(fetches).not.toHaveBeenCalled();
});

test('a refused task placement can be retried', async () => {
  const fetches = stubFetch(() => new Response('Could not answer', {status: 409}));
  const {root} = setup([{id: 'o', kind: 'attention', offer: true, text: 'Fix the parser', choices: ['New session', 'Do it here']}]);
  const buttons = findAll(root, '.attention-item button');
  await act(async () => buttons[2].click());
  expect(buttons.every(button => !button.hasAttribute('disabled'))).toBe(true);
  await act(async () => buttons[2].click());
  expect(fetches).toHaveBeenCalledTimes(2);
});

test('an approval request offers to always allow its tool in either scope', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockResolvedValue(new Response(''));
  for (const [i, scope] of ['session', 'all'].entries()) {
    const {root} = setup([{id: 'q', kind: 'attention', text: 'Allow Bash?', choices: ['Allow', 'Deny'], grant: 'Bash'}]);
    const buttons = findAll(root, 'button[data-always]');
    expect(buttons.map(b => b.textContent)).toEqual(['Always allow in this session', 'Always allow in all sessions']);
    await act(async () => { clickControl(buttons[i]); });
    expect(Object.fromEntries(requestParams(fetches.mock.calls, i))).toEqual({action: 'allow', id: 's', request: 'q', scope});
  }
});

test('a typed answer is sent with Enter and not when empty', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockResolvedValue(new Response(''));
  const {root} = setup([{id: 'q', kind: 'attention', text: 'Name?', free: true, choices: ['Ada']}]);
  const field = element(root, 'input', HTMLInputElement);
  expect(field.getAttribute('aria-label')).toBe('Other response');
  expect(submitButton(root).hasAttribute('disabled')).toBe(true);
  act(() => clickControl(submitButton(root)));
  act(() => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true})));
  expect(fetches).not.toHaveBeenCalled();
  const otherRadio = find(root, '[role="radio"]:not([data-choice])');
  act(() => clickControl(otherRadio));
  expect(otherRadio.getAttribute('aria-checked')).toBe('true');
  expect(submitButton(root).hasAttribute('disabled')).toBe(true);
  choose(root, 'Ada');
  expect(otherRadio.getAttribute('aria-checked')).toBe('false');
  setValue(field, 'Grace');
  act(() => field.dispatchEvent(new Event('input', {bubbles: true})));
  expect(find(root, '[role="radio"][data-choice="Ada"]').getAttribute('aria-checked')).toBe('false');
  expect(submitButton(root).hasAttribute('disabled')).toBe(false);
  await act(async () => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true})));
  expect(requestParams(fetches.mock.calls, 0).get('choice')).toBe('');
  expect(fetches.mock.calls[0][1]?.body).toBe('Grace');
});

test('a nested request stays pinned outside a collapsed activity and returns to history when answered', () => {
  const {root, send} = setup([
    {id: 'u', kind: 'user', text: 'Go', at: 1000},
    marker('w', 'working', 1100),
    {id: 'a', kind: 'tool', tool: 'agent', at: 1200},
    {id: 'q', parent: 'a', kind: 'attention', text: 'Allow?', choices: ['ok'], at: 1300},
    {id: 't', kind: 'text', text: 'Waiting on you', at: 1400},
    marker('i', 'idle', 2000, ['t']),
  ]);
  const group = find(root, '.activity-item');
  expect(group.getAttribute('data-state') === 'open').toBe(false);
  const pending = find(root, '[aria-label="Waiting on you"]');
  expect(pending.querySelector('button[data-choice="ok"]')).not.toBeNull();
  expect(pending.closest('.transcript-list')).toBeNull();
  expect(root.querySelector('.transcript-list .attention-item')).toBeNull();
  send([{id: 'q', parent: 'a', kind: 'attention', text: 'Allow?', done: true, answer: 'ok', at: 1300}]);
  expect(root.querySelector('.transcript-pending')).toBeNull();
  act(() => find(group, '[data-slot=collapsible-trigger]').click());
  act(() => find(group, '.tool-items-call > [data-slot=collapsible-trigger]').click());
  expect(find(group, '.attention-item').textContent).toContain('ok');
});

test('pending questions and their draft stay outside the history list while output arrives', () => {
  const {root, send} = setup([
    {id: 'q1', kind: 'attention', text: 'First?', free: true},
    {id: 'q2', kind: 'attention', text: 'Second?', choices: ['yes']},
  ]);
  const pending = find(root, '.transcript-pending');
  const field = element(pending, 'input', HTMLInputElement);
  setValue(field, 'My answer');
  act(() => field.dispatchEvent(new Event('input', {bubbles: true})));
  const view = find(root, '.transcript');
  view.scrollTop = 0;
  act(() => view.dispatchEvent(new Event('scroll')));
  send([{id: 'output', kind: 'text', text: 'More work'}]);
  expect(find(root, '.transcript-pending') === pending).toBe(true);
  expect(find(pending, 'input') === field).toBe(true);
  expect(field.value).toBe('My answer');
  expect(findAll(pending, '.attention-item-question').map(item => item.textContent)).toEqual(['First?']);
  expect(find(pending, '.questions-form-progress').textContent).toBe('Question 1 of 2');
  expect(find(view, '.transcript-list').textContent).toBe('More work');
  expect(find(root, '.transcript-footer').contains(pending)).toBe(true);
  send([{id: 'q1', kind: 'attention', text: 'First?', done: true, answer: 'My answer', seq: 0}]);
  expect(find(view, '.attention-item').textContent).toContain('My answer');
  expect(findAll(pending, '.attention-item-question').map(item => item.textContent)).toEqual(['Second?']);
  expect(pending.querySelector('.questions-form-progress')).toBeNull();
});

test('a turn draws as it runs and folds its activity under Worked for once it finishes', () => {
  const {root, send} = setup([
    {id: 'u', kind: 'user', text: 'Go', at: 1000},
    marker('w', 'working', 1100),
    {id: 'c', kind: 'tool', tool: 'bash', at: 1200},
    {id: 't', kind: 'text', text: 'Done', at: 1400},
  ]);
  expect(root.querySelector('.activity-item')).toBeNull();
  expect(find(root, '.transcript-list > .tool-items-call')).toBeDefined();
  send([marker('i', 'idle', 2000, ['t'])]);
  expect(find(root, '.activity-item [data-slot=collapsible-trigger]').textContent).toBe('Worked for 1s');
  expect(root.querySelector('.transcript-list > .tool-items-call')).toBeNull();
  expect(find(root, '.transcript-list > .transcript-text').textContent).toBe('Done');
});

test('empty reasoning and text do not create an empty activity disclosure', () => {
  const {root, send} = setup([
    {id: 'u', kind: 'user', text: 'Go', at: 1000},
    marker('w', 'working', 1100),
    {id: 'r', kind: 'reasoning', at: 1200},
    {id: 'blank', kind: 'text', text: '  \n', at: 1300},
    {id: 't', kind: 'text', text: 'Done', at: 1400},
    marker('i', 'idle', 5000, ['t']),
  ]);
  expect(root.querySelector('.activity-item')).toBeNull();
  expect(find(root, '.transcript-list > .transcript-text').textContent).toBe('Done');

  send([{id: 'r', kind: 'reasoning', text: 'Checking the answer', at: 1200, seq: 3}]);
  const activity = find(root, '.activity-item');
  act(() => find(activity, '[data-slot=collapsible-trigger]').click());
  expect(find(activity, '.reasoning-item').textContent).toBe('Checking the answer');
});

test('image-only activity remains available to expand', () => {
  const {root} = setup([
    marker('w', 'working', 1000),
    {id: 'image', kind: 'text', images: ['/transcript-image?index=0'], at: 1200},
    {id: 't', kind: 'text', text: 'Done', at: 1400},
    marker('i', 'idle', 5000, ['t']),
  ]);
  const activity = find(root, '.activity-item');
  act(() => find(activity, '[data-slot=collapsible-trigger]').click());
  expect(find(activity, 'img').getAttribute('src')).toBe('/transcript-image?index=0');
});

// Happy DOM does not perform layout. These assertions protect the scrolling
// boundary and tab order that keep long pending forms above the composer.
test('pending forms have a shrinking scroll region before the composer', () => {
  const items: Item[] = Array.from({length: 6}, (_, i) => ({id: `q${i}`, kind: 'attention', text: `Question ${i}`, choices: ['Yes', 'No'], free: true}));
  items.push({id: 'p', kind: 'proposal', text: 'Long task', input: {name: 'child', title: 'Long task', settings: {harness: 'codex', model: 'm', effort: 'low'}, message: 'A long brief.\n'.repeat(40)}});
  const store = createStore(testState({session: emptySession('s')}));
  store.update(s => receivedTranscript(s, {type: 'transcript', session: 's', reset: true, items: items.map((item, seq) => ({...item, seq}))}));
  const root = mount(store, <Transcript session={busy} footer={<textarea aria-label="Message" />} />);
  const footer = find(root, '.transcript-footer');
  const pending = find(footer, '.transcript-pending');
  expect(footer.classList.contains('min-h-0')).toBe(true);
  expect(footer.classList.contains('max-h-3/4')).toBe(true);
  expect(find(footer, '.transcript-footer-content').classList.contains('overflow-y-auto')).toBe(true);
  expect(pending.classList.contains('min-h-24')).toBe(true);
  expect(pending.classList.contains('overflow-y-auto')).toBe(true);
  expect(findAll(pending, '.attention-item-question').map(item => item.textContent)).toEqual(['Question 0']);
  expect(findAll(pending, '.questions-form-next')).toHaveLength(1);
  expect(findAll(pending, '[aria-label="Proposed child session"] button').map(button => button.textContent)).toEqual(['Start', 'Dismiss']);
  const controls = findAll(footer, 'button, input, textarea').filter(control => !control.hasAttribute('tabindex') || control.tabIndex >= 0);
  expect(controls.at(-1)?.getAttribute('aria-label')).toBe('Message');
});

test('incoming, outgoing, and automatic messages collapse to one line and can be reopened', () => {
  const image = '/transcript-image?index=0';
  const {root} = setup([
    {id: 'user', kind: 'user', text: 'A regular user prompt'},
    {id: 'incoming', kind: 'user', from: 'child', text: 'First line\nSecond line', images: [image]},
    {id: 'outgoing', kind: 'tool', server: 'hypercube', tool: 'send_message', input: {to: 'child', text: 'A single line message'}, done: true},
    {id: 'failed', kind: 'tool', server: 'hypercube', tool: 'send_message', input: {to: 'child', text: 'Try again'}, failed: true, done: true, output: 'Delivery failed', images: [image]},
    {id: 'automatic', kind: 'user', automatic: true, text: 'A report\nFull details', images: [image]},
  ]);
  expect(find(root, '.message-item-user').textContent).toBe('A regular user prompt');
  expect(root.querySelector('.message-item-user button[data-slot=collapsible-trigger]')).toBeNull();
  const messages = findAll(root, '.message-item-agent');
  expect(messages).toHaveLength(4);
  expect(find(messages[3], '.disclosure-heading').textContent).toContain('Automatic message');
  expect(root.textContent).not.toContain('Child reports');
  for (const [i, message] of messages.entries()) {
    const trigger = find(message, '[data-slot=collapsible-trigger]');
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(find(message, '.disclosure-heading').contains(find(message, '.message-item-preview'))).toBe(true);
    expect(message.querySelector('.message-item-body')).toBeNull();
    expect(message.querySelector('img')).toBeNull();
    act(() => trigger.click());
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    expect(message.querySelector('.message-item-body')).not.toBeNull();
    if (i === 0) expect(find(message, '.message-item-body').textContent).toContain('Second line');
    if (i === 2) expect(find(message, '.message-item-body').textContent).toContain('Delivery failed');
    if (i !== 1) expect(message.querySelector('img')).not.toBeNull();
    act(() => trigger.click());
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(message.querySelector('.message-item-body')).toBeNull();
  }
});
