import {afterEach, expect, mock, spyOn, test} from 'bun:test';
import type {DetachedWindowAPI} from 'happy-dom';
import {act} from 'react';
import {clickControl, element, find, findAll, mount, setValue, unmountAll} from '../../test-render.tsx';
import {testSession, testState} from '../../test-state.ts';
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
const happyDOM: DetachedWindowAPI = Reflect.get(window, 'happyDOM');

const busy = testSession({id: 's', running: true, activity: 'busy'});

type NewItem = Item & {seq?: number};

function setup(items: NewItem[], session = busy) {
  let seq = 0;
  const store = createStore(testState({session: emptySession('s')}));
  const send = (items: NewItem[], reset = false) => act(() => store.update(s => receivedTranscript(s, {type: 'transcript', session: 's', reset, items: items.map(item => ({...item, seq: item.seq ?? ++seq}))})));
  send(items, true);
  const root = mount(store, <Transcript session={session} />);
  return {store, root, send};
}

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

test('a tool call starts collapsed and preserves expansion as its output arrives', () => {
  const {root, store, send} = setup([{id: 'c', kind: 'tool', tool: 'bash', input: {command: 'ls'}}]);
  const call = () => find(root, '.tool-items-call');
  expect(find(call(), '.tool-items-title').textContent).toContain('Run command ls');
  expect(call().querySelector('[title="Running"]')).not.toBeNull();
  expect(call().getAttribute('data-state') === 'open').toBe(false);
  expect(call().querySelector('.tool-items-body')).toBeNull();
  act(() => find(call(), '[data-slot=collapsible-trigger]').click());
  expect(store.get().session?.expanded.c).toBe(true);
  expect(find(call(), 'pre').textContent).toBe('{\n  "command": "ls"\n}');
  send([{id: 'n', parent: 'c', kind: 'text', text: 'inside'}, {id: 'c', kind: 'tool', tool: 'bash', input: {command: 'ls'}, output: 'a.txt', done: true, failed: true}]);
  expect(call().querySelector('[title="Running"]')).toBeNull();
  expect(call().querySelector('[title="Failed"]')).not.toBeNull();
  expect(call().querySelectorAll('pre')[1].textContent).toBe('a.txt');
  expect(findAll(call(), 'h4').map(header => header.textContent)).toEqual(['Arguments', 'Output']);
  expect(find(call(), '.tool-items-children').textContent).toBe('inside');
  act(() => find(call(), '[data-slot=collapsible-trigger]').click());
  expect(call().querySelector('.tool-items-body')).toBeNull();
  expect(store.get().session?.expanded.c).toBeUndefined();
});

test('a question renders once with choices while pending, then with its answer in a card', () => {
  const {root, send} = setup([
    {id: 'copy', kind: 'text', text: 'Which format?\n- PDF\n- HTML', done: true},
    {id: 'q', kind: 'attention', text: 'Which format?', choices: ['PDF', 'HTML'], free: true},
  ]);
  expect(findAll(root, '.attention-item-question').map(question => question.textContent)).toEqual(['Which format?']);
  expect(root.querySelector('.transcript-text')).toBeNull();
  expect(findAll(root, '[data-choice]').map(button => button.textContent)).toEqual(['PDF', 'HTML']);
  send([{id: 'q', kind: 'attention', text: 'Which format?', choices: ['PDF', 'HTML'], done: true, answer: 'PDF'}]);
  const card = find(root, '.attention-item');
  expect(card.textContent).toContain('Answered');
  expect(find(card, '.attention-item-answer').textContent).toBe('PDF');
  expect(root.querySelector('.transcript-text')).toBeNull();
  expect(root.querySelector('[data-choice]')).toBeNull();
  expect(card.textContent).not.toContain('HTML');
});

test('Hypercube tools show readable titles and retain their full input and result', () => {
  const input = {path: 'main.go', lines: [{start: 10, end: 20}]};
  const {root} = setup([{id: 'c', kind: 'tool', server: 'hypercube', tool: 'open_file', input, output: 'Opened main.go.', done: true}]);
  const call = find(root, '.tool-items-call');
  expect(find(call, '.tool-items-name').textContent).toBe('Open file');
  expect(find(call, '.tool-items-detail').textContent).toBe('main.go');
  expect(call.querySelector('.tool-items-body')).toBeNull();
  act(() => clickControl(find(call, '[data-slot=collapsible-trigger]')));
  expect(findAll(call, 'pre').map(pre => pre.textContent)).toEqual([JSON.stringify(input, null, 2), 'Opened main.go.']);
});

test.each([
  ['shell', {command: "/bin/zsh -lc 'git status --short'", cwd: '/repo'}, 'Run command', 'git status --short'],
  ['apply_patch', {path: '/repo/main.go', changes: [{path: '/repo/main.go', diff: '+hello'}]}, 'Edit files', '/repo/main.go'],
  ['web_search', {query: 'Go documentation'}, 'Search web', 'Go documentation'],
  ['view_image', {path: '/repo/preview.png'}, 'View image', '/repo/preview.png'],
] as const)('Codex %s calls keep readable labels while running, completed, and expanded', (tool, input, name, detail) => {
  const {root, send} = setup([{id: 'c', kind: 'tool', tool, input}]);
  const call = () => find(root, '.tool-items-call');
  expect(find(call(), '.tool-items-name').textContent).toBe(name);
  expect(find(call(), '.tool-items-detail').textContent).toBe(detail);
  send([{id: 'c', kind: 'tool', tool, input, output: 'Result', done: true}]);
  expect(find(call(), '.tool-items-name').textContent).toBe(name);
  expect(find(call(), '.tool-items-detail').textContent).toBe(detail);
  act(() => clickControl(find(call(), '[data-slot=collapsible-trigger]')));
  expect(findAll(call(), 'pre').map(pre => pre.textContent)).toEqual([JSON.stringify(input, null, 2), 'Result']);
});

const marker = (id: string, state: WorkState, at: number, final?: string[]): Item => ({id: 'L1:' + id, kind: 'state', state, at, done: true, final});
const summaries = (root: ParentNode) => findAll(root, '.activity-item > [data-slot=collapsible-trigger]');

test('a running turn shows its rows with collapsed tool details and no working summary', () => {
  const {root} = setup([
    {id: 'u', kind: 'user', text: 'Go', at: 1000},
    marker('w', 'working', 1100),
    {id: 'r', kind: 'reasoning', text: 'Considering the plan.\nSecond line.', at: 1200},
    {id: 'c', kind: 'tool', tool: 'bash', input: {command: 'ls'}, output: 'a.txt', done: true, at: 1300},
    {id: 'p', kind: 'text', text: 'Progress update', at: 1400},
    {id: 'm', kind: 'user', from: 'peer', text: 'Peer says hello.\nMore detail.', at: 1500},
    {id: 'o', kind: 'tool', server: 'hypercube', tool: 'send_message', input: {to: 'peer', text: 'Reply body.'}, done: true, at: 1600},
  ]);
  expect(root.querySelector('.activity-item')).toBeNull();
  expect(root.textContent).not.toContain('Working');
  expect(find(root, '.reasoning-item').textContent).toBe('Considering the plan.\nSecond line.');
  expect(find(root, '.tool-items-call').textContent).toContain('Run command ls');
  expect(root.querySelector('.tool-items-body')).toBeNull();
  expect(root.textContent).toContain('Progress update');
  expect(root.textContent).toContain('Peer says hello.\nMore detail.');
  expect(root.textContent).toContain('Reply body.');
  expect(find(root, '[data-slot=collapsible]').getAttribute('data-state') === 'open').toBe(false);
});

test('a successful turn collapses to its opener, one Worked row, and the whole response', () => {
  const {root, store, send} = setup([
    {id: 'u', kind: 'user', text: 'Go', at: 1000},
    marker('w', 'working', 1100),
    {id: 'r', kind: 'reasoning', text: 'Thinking it over.', at: 1200},
    {id: 'c', kind: 'tool', tool: 'bash', input: {command: 'ls'}, output: 'a.txt', done: true, at: 1300},
    {id: 'h', kind: 'user', text: 'Also check b', at: 1350},
    {id: 'm', kind: 'user', from: 'peer', text: 'Peer detail', at: 1400},
    {id: 'p', kind: 'text', text: 'Almost done.', at: 1450},
    {id: 'a', kind: 'text', text: 'First part.', at: 1500},
    {id: 'b', kind: 'text', text: 'Second part.', at: 1600},
  ]);
  send([marker('i', 'idle', 133000, ['a', 'b'])]);
  const [summary] = summaries(root);
  const group = find(root, '.activity-item');
  expect(summary.textContent).toBe('Worked for 2m 12s');
  expect(group.getAttribute('data-state') === 'open').toBe(false);
  expect(group.querySelector('.activity-item-rows')).toBeNull();
  expect(find(root, '.transcript-list').textContent).toBe('GoWorked for 2m 12sFirst part.Second part.');
  expect(root.textContent).not.toContain('Thinking it over.');

  act(() => clickControl(summary));
  for (const text of ['Thinking it over.', 'Run command ls', 'Also check b', 'Peer detail', 'Almost done.']) expect(group.textContent).toContain(text);
  expect(store.get().session?.expanded['activity:L1:w']).toBe(true);
  act(() => clickControl(summary));
  expect(group.querySelector('.activity-item-rows')).toBeNull();
});

const choose = (root: HTMLElement, choice: string) => {
  act(() => clickControl(find(root, `button[data-choice="${choice}"]`)));
};

test('an attention request offers its choices, waits while sending, and opens again when refused', async () => {
  const refusal = Promise.withResolvers<Response>();
  const fetches = spyOn(globalThis, 'fetch').mockReturnValue(refusal.promise);
  const {root, send} = setup([{id: 'q', kind: 'attention', text: 'Proceed?', choices: ['yes', 'no']}]);
  const rows = () => findAll(root, 'button[data-choice]');
  const list = () => find(root, '[role="group"]');
  expect(list().getAttribute('aria-label')).toBe('Choices');
  expect(rows().map(r => r.textContent)).toEqual(['yes', 'no']);
  expect(root.querySelector('input')).toBeNull();
  choose(root, 'yes');
  expect(Object.fromEntries(requestParams(fetches.mock.calls, 0))).toEqual({action: 'answer', id: 's', request: 'q', choice: 'yes'});
  expect(rows().every(row => row.hasAttribute('disabled'))).toBe(true);
  choose(root, 'no');
  expect(fetches).toHaveBeenCalledTimes(1);
  await act(async () => {
    refusal.resolve(new Response('no longer open', {status: 409}));
    await new Promise(resolve => setTimeout(resolve));
  });
  expect(rows().every(row => !row.hasAttribute('disabled'))).toBe(true);

  send([{id: 'q', kind: 'attention', text: 'Proceed?', done: true, answer: 'no', seq: 1}]);
  expect(rows()).toEqual([]);
  expect(find(root, '.attention-item-answered').textContent).toBe('Proceed?');
  expect(find(root, '.attention-item-answer').textContent).toBe('no');
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

test('a request that is not an approval offers no always allow', () => {
  const {root} = setup([{id: 'q', kind: 'attention', text: 'Proceed?', choices: ['yes', 'no']}]);
  expect(root.querySelector('button[data-always]')).toBeNull();
});

test('a choice shows its associated note', () => {
  const {root} = setup([{id: 'q', kind: 'attention', text: 'Language?', choices: ['Python', 'Go'], notes: ['Readable', '']}]);
  const [python, go] = root.querySelectorAll('button[data-choice]');
  expect(python.textContent).toBe('Python');
  expect(document.getElementById(python.getAttribute('aria-describedby') ?? '')?.textContent).toBe('Readable');
  expect(go.hasAttribute('aria-describedby')).toBe(false);
});

test('a typed answer is sent with Enter and not when empty', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockResolvedValue(new Response(''));
  const {root} = setup([{id: 'q', kind: 'attention', text: 'Name?', free: true, choices: ['Ada']}]);
  const field = element(root, 'input', HTMLInputElement);
  const sendButton = find(root, '.attention-form-entry button');
  const label = find(root, 'label');
  expect(label.textContent).toBe('Other response');
  expect(label.getAttribute('for')).toBe(field.id);
  expect(field.querySelector('button')).toBeNull();
  expect(sendButton.textContent).toBe('Send');
  expect(sendButton.hasAttribute('disabled')).toBe(true);
  act(() => clickControl(sendButton));
  expect(fetches).not.toHaveBeenCalled();
  act(() => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true})));
  expect(fetches).not.toHaveBeenCalled();
  setValue(field, 'Ada');
  act(() => field.dispatchEvent(new Event('input', {bubbles: true})));
  expect(sendButton.hasAttribute('disabled')).toBe(false);
  await act(async () => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true})));
  expect(requestParams(fetches.mock.calls, 0).get('choice')).toBe('');
  expect(fetches.mock.calls[0][1]?.body).toBe('Ada');
});

test('the Send button beside the field sends the typed answer', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockResolvedValue(new Response(''));
  const {root} = setup([{id: 'q', kind: 'attention', text: 'Name?', free: true}]);
  const field = element(root, 'input', HTMLInputElement);
  expect(find(root, 'label').textContent).toBe('Your response');
  setValue(field, 'Grace');
  act(() => field.dispatchEvent(new Event('input', {bubbles: true})));
  await act(async () => clickControl(find(root, '.attention-form-entry button')));
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
  const pending = find(root, '[aria-label="Pending questions"]');
  expect(pending.querySelector('button[data-choice="ok"]')).not.toBeNull();
  expect(pending.closest('.transcript-list')).toBeNull();
  expect(root.querySelector('.transcript-list .attention-item')).toBeNull();
  send([{id: 'q', parent: 'a', kind: 'attention', text: 'Allow?', done: true, answer: 'ok', at: 1300}]);
  expect(root.querySelector('.transcript-pending')).toBeNull();
  act(() => find(group, '[data-slot=collapsible-trigger]').click());
  act(() => find(group, '.tool-items-call > [data-slot=collapsible-trigger]').click());
  expect(find(group, '.attention-item-answer').textContent).toBe('ok');
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
  expect(findAll(pending, '.attention-item-question').map(item => item.textContent)).toEqual(['First?', 'Second?']);
  expect(find(view, '.transcript-list').textContent).toBe('More work');
  expect(pending.parentElement === find(view, '.transcript-bottom')).toBe(true);
  send([{id: 'q1', kind: 'attention', text: 'First?', done: true, answer: 'My answer', seq: 0}]);
  expect(find(view, '.attention-item-answer').textContent).toBe('My answer');
  expect(pending.querySelectorAll('.attention-item').length).toBe(1);
});

test('the list follows its end until the user scrolls up', () => {
  const {root, send} = setup([{id: '1', kind: 'text', text: 'a'}]);
  const view = find(root, '.transcript');
  let height = 1000;
  Object.defineProperty(view, 'scrollHeight', {get: () => height});
  Object.defineProperty(view, 'clientHeight', {get: () => 300});
  const scroll = (top: number) => {
    view.scrollTop = top;
    view.dispatchEvent(new Event('scroll'));
  };
  send([{id: '2', kind: 'text', text: 'b'}]);
  expect(view.scrollTop).toBe(1000);
  act(() => scroll(500));
  expect(root.querySelector('[title="Scroll to newest output"]')).not.toBeNull();
  height = 1200;
  send([{id: '3', kind: 'text', text: 'c'}]);
  expect(view.scrollTop).toBe(500);
  const composer = document.createElement('textarea');
  composer.className = 'composer-text';
  root.append(composer);
  const button = find(root, '[title="Scroll to newest output"]');
  button.focus();
  act(() => button.click());
  expect(root.querySelector('[title="Scroll to newest output"]')).toBeNull();
  expect(document.activeElement).toBe(composer);
  send([{id: '4', kind: 'text', text: 'd'}]);
  expect(view.scrollTop).toBe(1200);
});

const longText = Array.from({length: 10}, (_, i) => 'line ' + i).join('\n');

test('a message that begins a turn shows its sender and first line, and expands to the full message', () => {
  const {root} = setup([
    {id: 'in', kind: 'user', from: 'c1', text: longText, done: true},
    {id: 'own', kind: 'user', text: longText},
  ]);
  const [incoming, own] = root.querySelectorAll('.message-item');
  expect(find(incoming, '.disclosure-heading').textContent).toBe('From c1');
  expect(incoming.querySelector('.message-item-bubble')).toBeNull();
  expect(find(incoming, '.disclosure-preview').textContent).toBe('line 0');
  expect(incoming.querySelector('.message-item-text')).toBeNull();
  act(() => find(incoming, '[data-slot=collapsible-trigger]').click());
  expect(incoming.getAttribute('data-state') === 'open').toBe(true);
  expect(incoming.querySelector('.disclosure-preview')).toBeNull();
  expect(find(incoming, '.message-item-text').textContent).toBe(longText);
  act(() => find(incoming, '[data-slot=collapsible-trigger]').click());
  expect(incoming.getAttribute('data-state') === 'open').toBe(false);
  expect(find(incoming, '.disclosure-preview').textContent).toBe('line 0');
  expect(own.querySelector('.message-item-bubble')).not.toBeNull();
  expect(find(own, '.message-item-text').textContent).toBe(longText);
  expect(own.querySelector('[data-slot=collapsible-trigger]')).toBeNull();
});

test('messages sent by the agent show their recipient and whole body', () => {
  const sent: NewItem = {id: 'out', kind: 'tool', server: 'hypercube', tool: 'send_message', input: {to: 'c1', text: longText}};
  const {root, send} = setup([sent]);
  const outgoing = find(root, '.message-item');
  expect(find(outgoing, '.message-item-heading').textContent).toBe('To c1');
  expect(outgoing.querySelector('[title="Running"]')).not.toBeNull();
  expect(find(outgoing, '.message-item-text').textContent).toBe(longText);
  expect(outgoing.querySelector('[data-slot=collapsible-trigger]')).toBeNull();
  send([{...sent, done: true, failed: true, output: 'Delivery failed'}]);
  expect(outgoing.querySelector('[title="Running"]')).toBeNull();
  expect(outgoing.querySelector('[title="Failed"]')).not.toBeNull();
  expect(find(outgoing, '.message-item-body .message-item-failure').textContent).toBe('Delivery failed');
});

test('short agent messages also expand, including their attached images', () => {
  const {root} = setup([{id: 'in', kind: 'user', from: 'c1', text: 'One line', images: [{mime_type: 'image/png', data: 'iVBORw0KGgo='}]}]);
  const message = find(root, '.message-item-agent');
  expect(find(message, '.disclosure-preview').textContent).toBe('One line');
  expect(message.querySelector('img')).toBeNull();
  act(() => find(message, '[data-slot=collapsible-trigger]').click());
  expect(find(message, '.message-item-text').textContent).toBe('One line');
  expect(find(message, 'img').getAttribute('src')).toBe('data:image/png;base64,iVBORw0KGgo=');
});

test('Thinking shows its full text as it streams', () => {
  const {root, send} = setup([{id: 't', kind: 'reasoning', text: longText, at: 1000, seq: 1}]);
  const item = find(root, '.reasoning-item');
  expect(item.textContent).toBe(longText);
  expect(root.querySelector('[data-slot=collapsible-trigger]')).toBeNull();
  send([{id: 't', kind: 'reasoning', text: longText + '\nmore', at: 1000, seq: 1}]);
  expect(item.textContent).toEndWith('more');
  send([{id: 'e', kind: 'reasoning', at: 1100, seq: 2}]);
  expect(root.querySelectorAll('.reasoning-item').length).toBe(1);
});

const png = 'iVBORw0KGgo=';

test('prompts, text, and tool calls show their images', () => {
  const {root} = setup([
    {id: 'u', kind: 'user', text: 'look', images: [{mime_type: 'image/png', data: png}]},
    {id: 'x', kind: 'text', images: [{mime_type: 'image/gif', data: 'R0lGODlh'}]},
    {id: 'c', kind: 'tool', tool: 'screenshot', done: true, images: [{mime_type: 'image/png', data: png}, {mime_type: 'image/svg+xml', data: png}]},
  ]);
  act(() => find(root, '.tool-items-call > [data-slot=collapsible-trigger]').click());
  const sources = (selector: string) => findAll(root, selector + ' img').map(img => img.getAttribute('src'));
  expect(sources('.message-item')).toEqual(['data:image/png;base64,' + png]);
  expect(sources('.transcript-text')).toEqual(['data:image/gif;base64,R0lGODlh']);
  expect(sources('.tool-items-call')).toEqual(['data:image/png;base64,' + png]);
});

test('copy buttons copy the original message and code', async () => {
  const copied: string[] = [];
  spyOn(navigator.clipboard, 'writeText').mockImplementation(async text => { copied.push(text); });
  const {root} = setup([{id: 'u', kind: 'user', text: '**question**'}, {id: 'a', kind: 'text', text: 'An **answer**\n\n```js\nlet x = 1;\n```'}]);
  const buttons = findAll(root, 'button[aria-label^="Copy"]');
  await act(async () => { for (const button of buttons) clickControl(button); });
  expect(copied).toEqual(['**question**', 'let x = 1;', 'An **answer**\n\n```js\nlet x = 1;\n```']);
  expect(buttons.map(button => element(button, 'svg[data-icon]', SVGElement).getAttribute('data-icon'))).toEqual(['check', 'check', 'check']);
});

test('find opens with Cmd+F, steps through visible matches and restores focus', () => {
  const {root} = setup([{id: 'u', kind: 'user', text: 'Hello'}, {id: 'a', kind: 'text', text: 'Hello **world** and hello again'}]);
  const focus = find(root, 'button[aria-label="Copy message"]');
  focus.focus();
  act(() => focus.dispatchEvent(new KeyboardEvent('keydown', {key: 'f', metaKey: true, bubbles: true, cancelable: true})));
  const field = element(root, '.transcript-find input', HTMLInputElement);
  expect(field).not.toBeNull();
  act(() => { setValue(field, 'hello'); field.dispatchEvent(new Event('input', {bubbles: true})); });
  expect(find(root, '.transcript-find-count').textContent).toBe('1 of 3');
  act(() => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true})));
  expect(find(root, '.transcript-find-count').textContent).toBe('2 of 3');
  act(() => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', shiftKey: true, bubbles: true})));
  expect(find(root, '.transcript-find-count').textContent).toBe('1 of 3');
  act(() => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true})));
  expect(root.querySelector('.transcript-find')).toBeNull();
  expect(document.activeElement).toBe(focus);
});

test('find preserves the selected match as the transcript streams and clamps it when matches disappear', async () => {
  const {root, send} = setup([{id: 'a', kind: 'text', text: 'one one one'}]);
  const focus = find(root, 'button[aria-label="Copy message"]');
  focus.focus();
  act(() => focus.dispatchEvent(new KeyboardEvent('keydown', {key: 'f', metaKey: true, bubbles: true, cancelable: true})));
  const field = element(root, '.transcript-find input', HTMLInputElement);
  act(() => { setValue(field, 'one'); field.dispatchEvent(new Event('input', {bubbles: true})); });
  act(() => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true})));
  expect(find(root, '.transcript-find-count').textContent).toBe('2 of 3');
  send([{id: 'a', kind: 'text', text: 'one one one one'}]);
  await act(async () => { await happyDOM.whenAsyncComplete(); });
  expect(find(root, '.transcript-find-count').textContent).toBe('2 of 4');
  send([{id: 'a', kind: 'text', text: 'one'}]);
  await act(async () => { await happyDOM.whenAsyncComplete(); });
  expect(find(root, '.transcript-find-count').textContent).toBe('1 of 1');
});

test('Enter on find buttons is not intercepted as find navigation', () => {
  const {root} = setup([{id: 'a', kind: 'text', text: 'one one'}]);
  const focus = find(root, 'button[aria-label="Copy message"]');
  focus.focus();
  act(() => focus.dispatchEvent(new KeyboardEvent('keydown', {key: 'f', metaKey: true, bubbles: true})));
  const field = element(root, '.transcript-find input', HTMLInputElement);
  act(() => { setValue(field, 'one'); field.dispatchEvent(new Event('input', {bubbles: true})); });
  const close = find(root, 'button[aria-label="Close find"]');
  const enter = new KeyboardEvent('keydown', {key: 'Enter', bubbles: true, cancelable: true});
  act(() => close.dispatchEvent(enter));
  expect(enter.defaultPrevented).toBe(false);
  expect(find(root, '.transcript-find-count').textContent).toBe('1 of 2');
  act(() => clickControl(close));
  expect(root.querySelector('.transcript-find')).toBeNull();
});

test('find shortcuts stay in the chat and navigate in both directions', () => {
  const {root} = setup([{id: 'a', kind: 'text', text: 'one one one'}]);
  const key = (target: HTMLElement, name: string, modifiers: KeyboardEventInit = {}) => {
    const event = new KeyboardEvent('keydown', {key: name, bubbles: true, cancelable: true, ...modifiers});
    act(() => target.dispatchEvent(event));
    return event;
  };
  const focus = find(root, 'button[aria-label="Copy message"]');
  focus.focus();
  expect(key(focus, 'f', {metaKey: true, shiftKey: true}).defaultPrevented).toBe(false);
  key(focus, 'f', {metaKey: true});
  const field = element(root, '.transcript-find input', HTMLInputElement);
  act(() => { setValue(field, 'one'); field.dispatchEvent(new Event('input', {bubbles: true})); });
  key(field, 'g', {metaKey: true});
  expect(find(root, '.transcript-find-count').textContent).toBe('2 of 3');
  key(field, 'F3', {shiftKey: true});
  expect(find(root, '.transcript-find-count').textContent).toBe('1 of 3');
});
