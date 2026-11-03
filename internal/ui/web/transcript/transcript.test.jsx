import {afterEach, expect, spyOn, test} from 'bun:test';
import {act} from 'preact/test-utils';
import {mount, unmountAll} from '../../test-render.jsx';
import {emptySession} from '../sessions/selection.js';
import {createStore} from '../store.js';
import {Transcript} from './transcript.jsx';
import {receivedTranscript} from './transcripts.js';

let spies = [];
afterEach(() => {
  for (const s of spies) s.mockRestore();
  spies = [];
  unmountAll();
});

const busy = {id: 's', running: true, activity: 'busy'};

function setup(items, session = busy) {
  const store = createStore({workspaces: [], drafts: {}, errors: [], session: emptySession('s')});
  const send = (items, reset = false) => store.update(s => receivedTranscript(s, {session: 's', reset, items}));
  send(withSeq(items), true);
  const root = mount(store, <Transcript session={session} />);
  return {store, root, send: items => send(withSeq(items))};
}
let seq = 0;
const withSeq = items => items.map(item => ({seq: item.seq ?? ++seq, ...item}));

test('items replace themselves in place, and unchanged items are left alone', () => {
  const {root, send} = setup([{id: '1', kind: 'user', text: 'hi', seq: 1}, {id: '2', kind: 'text', text: 'Hel', seq: 2}]);
  const list = root.querySelector('.transcript-list');
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
  expect(root.querySelectorAll('b, script, img, u, i, a').length).toBe(0);
  for (const text of ['<b>tool</b>', '<img src=x onerror=1>', '<u>bad</u>', '<i>thinking</i>', '<a href=x>me</a>']) expect(root.textContent).toContain(text);
});

test('a tool call shows its title, status, input, output, and nested items', () => {
  const {root, send} = setup([{id: 'c', kind: 'tool', tool: 'bash', input: {command: 'ls'}}]);
  const call = () => root.querySelector('.tool-items-call');
  expect(call().querySelector('.tool-items-header').textContent).toContain('bash ls');
  expect(call().querySelector('[title="Running"]')).not.toBeNull();
  expect(call().querySelector('pre').textContent).toBe('{\n  "command": "ls"\n}');
  send([{id: 'n', parent: 'c', kind: 'text', text: 'inside'}, {id: 'c', kind: 'tool', tool: 'bash', input: {command: 'ls'}, output: 'a.txt', done: true, failed: true}]);
  expect(call().querySelector('[title="Running"]')).toBeNull();
  expect(call().querySelector('[title="Failed"]')).not.toBeNull();
  expect(call().querySelectorAll('pre')[1].textContent).toBe('a.txt');
  expect(call().querySelector('.tool-items-children').textContent).toBe('inside');
});

const marker = (id, state, at, final) => ({id: 'L1:' + id, kind: 'state', state, at, done: true, final});
const summaries = root => [...root.querySelectorAll('.activity-item > summary')];

test('a running turn shows every row openly with no working summary', () => {
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
  expect(root.querySelector('.reasoning-item').textContent).toBe('Considering the plan.\nSecond line.');
  expect(root.querySelector('.tool-items-call').textContent).toContain('a.txt');
  expect(root.textContent).toContain('Progress update');
  expect(root.textContent).toContain('Peer says hello.\nMore detail.');
  expect(root.textContent).toContain('Reply body.');
  expect(root.querySelector('details')).toBeNull();
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
  const list = root.querySelector('.transcript-list');
  send([marker('i', 'idle', 133000, ['a', 'b'])]);
  expect([...list.children].map(row => row.className.split(' ')[0])).toEqual(['message-item', 'disclosure', 'transcript-text', 'transcript-text']);
  const [summary] = summaries(root);
  expect(summary.textContent).toBe('Worked for 2m 12s');
  expect(summary.parentElement.open).toBe(false);
  expect(summary.parentElement.querySelector('.activity-item-rows')).toBeNull();
  expect(list.children[0].textContent).toBe('Go');
  expect(list.children[2].textContent).toBe('First part.');
  expect(list.children[3].textContent).toBe('Second part.');
  expect(root.textContent).not.toContain('Thinking it over.');

  act(() => summary.click());
  const rows = [...summary.parentElement.querySelectorAll('.activity-item-rows > *')];
  expect(rows.map(row => row.textContent)).toEqual([
    'Thinking it over.', expect.stringContaining('bash ls'), 'Also check b', expect.stringContaining('Peer detail'), 'Almost done.',
  ]);
  expect(store.get().session.expanded['activity:L1:w']).toBe(true);
  act(() => summary.click());
  expect(summary.parentElement.querySelector('.activity-item-rows')).toBeNull();
});

test('a finished turn keeps its summary while the next turn runs', () => {
  const {root, send} = setup([
    {id: 'u', kind: 'user', text: 'One', at: 1000},
    marker('w1', 'working', 1100),
    {id: 'c', kind: 'tool', tool: 'bash', done: true, at: 1200},
    {id: 'a', kind: 'text', text: 'Answer one', at: 1300},
    marker('i1', 'idle', 5000, ['a']),
  ]);
  send([{id: 'v', kind: 'user', text: 'Two', at: 6000}, marker('w2', 'working', 6100), {id: 'd', kind: 'tool', tool: 'read', at: 6200}]);
  expect(summaries(root).map(s => s.textContent)).toEqual(['Worked for 4s']);
  expect(root.querySelectorAll('.tool-items-call').length).toBe(1);
  expect(root.querySelector('.tool-items-call').textContent).toContain('read');
});

test.each([
  ['an idle state without a final', [marker('i', 'idle', 9000)]],
  ['a final naming nothing shown', [marker('i', 'idle', 9000, ['gone'])]],
])('%s leaves the turn expanded', (_name, ending) => {
  const {root} = setup([
    {id: 'u', kind: 'user', text: 'Go', at: 1000},
    marker('w', 'working', 1100),
    {id: 'c', kind: 'tool', tool: 'bash', input: {command: 'make'}, output: 'failed', failed: true, done: true, at: 1200},
    {id: 'e', kind: 'error', text: 'The turn failed.', at: 1300},
    ...ending,
  ]);
  expect(root.querySelector('.activity-item')).toBeNull();
  expect(root.querySelector('.tool-items-call [title="Failed"]')).not.toBeNull();
  expect(root.querySelector('.transcript-error').textContent).toBe('The turn failed.');
});

test('a transcript saved without state items stays expanded', () => {
  const {root} = setup([
    {id: 'u', kind: 'user', text: 'Go', at: 1000},
    {id: 'r', kind: 'reasoning', text: 'Old thought', at: 1100, done: true},
    {id: 'c', kind: 'tool', tool: 'read', done: true, at: 1200},
    {id: 'a', kind: 'text', text: 'Old answer', at: 1300},
  ]);
  expect(root.querySelector('.activity-item')).toBeNull();
  expect(root.textContent).toContain('Old thought');
  expect(root.textContent).toContain('Old answer');
});

test('a later launch cannot hide what an earlier launch left unfinished', () => {
  const {root} = setup([
    {id: 'u', kind: 'user', text: 'Go', at: 1000},
    marker('w', 'working', 1100),
    {id: 'c', kind: 'tool', tool: 'shell', input: {command: 'sleep 60'}, at: 1200},
    {id: 'u2', kind: 'user', text: 'Again', at: 9000},
    {id: 'M2:w', kind: 'state', state: 'working', done: true, at: 9100},
    {id: 'd', kind: 'tool', tool: 'read', done: true, at: 9200},
    {id: 'a', kind: 'text', text: 'Fresh answer', at: 9300},
    {id: 'M2:i', kind: 'state', state: 'idle', done: true, at: 9400, final: ['a']},
  ]);
  expect(summaries(root).map(s => s.textContent)).toEqual(['Worked for 0s']);
  expect(root.textContent).toContain('sleep 60');
  expect(root.querySelectorAll('.tool-items-call').length).toBe(1);
});

const choose = (root, choice) => {
  act(() => root.querySelector(`vscode-button[data-choice="${choice}"]`).click());
};

test('an attention request offers its choices, waits while sending, and opens again when refused', async () => {
  let answer;
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(() => new Promise(resolve => { answer = resolve; }));
  spies.push(fetches);
  const {root, send} = setup([{id: 'q', kind: 'attention', text: 'Proceed?', choices: ['yes', 'no']}]);
  const rows = () => [...root.querySelectorAll('vscode-button[data-choice]')];
  const list = () => root.querySelector('[role="group"]');
  expect(list().getAttribute('aria-label')).toBe('Choices');
  expect(rows().map(r => r.textContent)).toEqual(['yes', 'no']);
  expect(root.querySelector('vscode-textfield')).toBeNull();
  choose(root, 'yes');
  const url = new URL(fetches.mock.calls[0][0], 'http://x/');
  expect(Object.fromEntries(url.searchParams)).toEqual({action: 'answer', id: 's', request: 'q', choice: 'yes'});
  expect(rows().every(row => row.hasAttribute('disabled'))).toBe(true);
  choose(root, 'no');
  expect(fetches).toHaveBeenCalledTimes(1);
  await act(async () => {
    answer(new Response('no longer open', {status: 409}));
    await new Promise(resolve => setTimeout(resolve));
  });
  expect(rows().every(row => !row.hasAttribute('disabled'))).toBe(true);

  send([{id: 'q', kind: 'attention', text: 'Proceed?', done: true, answer: 'no', seq: 1}]);
  expect(rows()).toEqual([]);
  expect(root.querySelector('.attention-item-answered').textContent).toBe('Proceed?');
  expect(root.querySelector('.attention-item-answer').textContent).toBe('no');
});

test('an approval request offers to always allow its tool in either scope', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  spies.push(fetches);
  for (const [i, scope] of ['session', 'all'].entries()) {
    const {root} = setup([{id: 'q', kind: 'attention', text: 'Allow Bash?', choices: ['Allow', 'Deny'], grant: 'Bash'}]);
    const buttons = [...root.querySelectorAll('vscode-button[data-always]')];
    expect(buttons.map(b => b.textContent)).toEqual(['Always allow in this session', 'Always allow in all sessions']);
    await act(async () => { buttons[i].click(); });
    const url = new URL(fetches.mock.calls[i][0], 'http://x/');
    expect(Object.fromEntries(url.searchParams)).toEqual({action: 'allow', id: 's', request: 'q', scope});
  }
});

test('a request that is not an approval offers no always allow', () => {
  const {root} = setup([{id: 'q', kind: 'attention', text: 'Proceed?', choices: ['yes', 'no']}]);
  expect(root.querySelector('vscode-button[data-always]')).toBeNull();
});

test('a choice shows its associated note', () => {
  const {root} = setup([{id: 'q', kind: 'attention', text: 'Language?', choices: ['Python', 'Go'], notes: ['Readable', '']}]);
  const [python, go] = root.querySelectorAll('vscode-button[data-choice]');
  expect(python.textContent).toBe('Python');
  expect(document.getElementById(python.getAttribute('aria-describedby')).textContent).toBe('Readable');
  expect(go.hasAttribute('aria-describedby')).toBe(false);
});

test('a typed answer is sent with Enter and not when empty', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  spies.push(fetches);
  const {root} = setup([{id: 'q', kind: 'attention', text: 'Name?', free: true, choices: ['Ada']}]);
  const field = root.querySelector('vscode-textfield');
  const sendButton = root.querySelector('.attention-form-entry vscode-button');
  const label = root.querySelector('vscode-label');
  expect(label.textContent).toBe('Other response');
  expect(label.getAttribute('for')).toBe(field.id);
  expect(field.querySelector('vscode-button')).toBeNull();
  expect(sendButton.textContent).toBe('Send');
  expect(sendButton.hasAttribute('disabled')).toBe(true);
  act(() => sendButton.click());
  expect(fetches).not.toHaveBeenCalled();
  act(() => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter'})));
  expect(fetches).not.toHaveBeenCalled();
  field.value = 'Ada';
  act(() => field.dispatchEvent(new Event('input')));
  expect(sendButton.hasAttribute('disabled')).toBe(false);
  await act(async () => field.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter'})));
  const [url, options] = fetches.mock.calls[0];
  expect(new URL(url, 'http://x/').searchParams.get('choice')).toBe('');
  expect(options.body).toBe('Ada');
});

test('the Send button beside the field sends the typed answer', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  spies.push(fetches);
  const {root} = setup([{id: 'q', kind: 'attention', text: 'Name?', free: true}]);
  const field = root.querySelector('vscode-textfield');
  expect(root.querySelector('vscode-label').textContent).toBe('Your response');
  field.value = 'Grace';
  act(() => field.dispatchEvent(new Event('input')));
  await act(async () => root.querySelector('.attention-form-entry vscode-button').click());
  expect(fetches.mock.calls[0][1].body).toBe('Grace');
});

test('a request inside a finished turn opens its activity', () => {
  const {root} = setup([
    {id: 'u', kind: 'user', text: 'Go', at: 1000},
    marker('w', 'working', 1100),
    {id: 'a', kind: 'tool', tool: 'agent', at: 1200},
    {id: 'q', parent: 'a', kind: 'attention', text: 'Allow?', choices: ['ok'], at: 1300},
    {id: 't', kind: 'text', text: 'Waiting on you', at: 1400},
    marker('i', 'idle', 2000, ['t']),
  ]);
  const group = root.querySelector('.activity-item');
  expect(group.open).toBe(true);
  expect(group.querySelector('vscode-button[data-choice="ok"]')).not.toBeNull();
});

test('the list follows its end until the user scrolls up', () => {
  const {root, send} = setup([{id: '1', kind: 'text', text: 'a'}]);
  const view = root.querySelector('.transcript');
  let height = 1000;
  Object.defineProperty(view, 'scrollHeight', {get: () => height});
  Object.defineProperty(view, 'clientHeight', {get: () => 300});
  const scroll = top => {
    view.scrollTop = top;
    view.dispatchEvent(new Event('scroll'));
  };
  send([{id: '2', kind: 'text', text: 'b'}]);
  expect(view.scrollTop).toBe(1000);
  scroll(500);
  height = 1200;
  send([{id: '3', kind: 'text', text: 'c'}]);
  expect(view.scrollTop).toBe(500);
  scroll(899);
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
  expect(incoming.querySelector('.disclosure-heading').textContent).toBe('From c1');
  expect(incoming.querySelector('.message-item-bubble')).toBeNull();
  expect(incoming.querySelector('.disclosure-preview').textContent).toBe('line 0');
  expect(incoming.querySelector('.message-item-text')).toBeNull();
  act(() => incoming.querySelector('summary').click());
  expect(incoming.open).toBe(true);
  expect(incoming.querySelector('.disclosure-preview')).toBeNull();
  expect(incoming.querySelector('.message-item-text').textContent).toBe(longText);
  act(() => incoming.querySelector('summary').click());
  expect(incoming.open).toBe(false);
  expect(incoming.querySelector('.disclosure-preview').textContent).toBe('line 0');
  expect(own.querySelector('.message-item-bubble')).not.toBeNull();
  expect(own.querySelector('.message-item-text').textContent).toBe(longText);
  expect(own.querySelector('summary')).toBeNull();
});

test('messages sent by the agent show their recipient and whole body', () => {
  const sent = {id: 'out', kind: 'tool', server: 'hypercube', tool: 'send_message', input: {to: 'c1', text: longText}};
  const {root, send} = setup([sent]);
  const outgoing = root.querySelector('.message-item');
  expect(outgoing.querySelector('.message-item-heading').textContent).toBe('To c1');
  expect(outgoing.querySelector('[title="Running"]')).not.toBeNull();
  expect(outgoing.querySelector('.message-item-text').textContent).toBe(longText);
  expect(outgoing.querySelector('summary')).toBeNull();
  send([{...sent, done: true, failed: true, output: 'Delivery failed'}]);
  expect(outgoing.querySelector('[title="Running"]')).toBeNull();
  expect(outgoing.querySelector('[title="Failed"]')).not.toBeNull();
  expect(outgoing.querySelector('.message-item-body .message-item-failure').textContent).toBe('Delivery failed');
});

test('short agent messages also expand, including their attached images', () => {
  const {root} = setup([{id: 'in', kind: 'user', from: 'c1', text: 'One line', images: [{mime_type: 'image/png', data: 'iVBORw0KGgo='}]}]);
  const message = root.querySelector('.message-item-agent');
  expect(message.querySelector('.disclosure-preview').textContent).toBe('One line');
  expect(message.querySelector('img')).toBeNull();
  act(() => message.querySelector('summary').click());
  expect(message.querySelector('.message-item-text').textContent).toBe('One line');
  expect(message.querySelector('img').getAttribute('src')).toBe('data:image/png;base64,iVBORw0KGgo=');
});

test('Thinking shows its full text as it streams', () => {
  const {root, send} = setup([{id: 't', kind: 'reasoning', text: longText, at: 1000, seq: 1}]);
  const item = root.querySelector('.reasoning-item');
  expect(item.textContent).toBe(longText);
  expect(root.querySelector('summary')).toBeNull();
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
  const sources = selector => [...root.querySelectorAll(selector + ' img')].map(img => img.getAttribute('src'));
  expect(sources('.message-item')).toEqual(['data:image/png;base64,' + png]);
  expect(sources('.transcript-text')).toEqual(['data:image/gif;base64,R0lGODlh']);
  expect(sources('.tool-items-call')).toEqual(['data:image/png;base64,' + png]);
});

test('mirrored requests name their sender and answer at its session ID; child status stays hidden', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  spies.push(fetches);
  const {store, root, send} = setup([
    {id: 'status', kind: 'status', from: 'child', text: 'Reading.', at: 1},
    {id: 'request', kind: 'attention', from: 'child', text: 'Proceed?', choices: ['yes']},
    {id: 'message', kind: 'user', from: 'child', text: 'Result.'},
  ]);
  store.update(state => ({...state, workspaces: [{sessions: [{id: 's', children: [{id: 'child', parent: 's', title: 'Builder', children: []}]}]}]}));
  expect(root.textContent).not.toContain('Reading.');
  expect(store.get().session.transcript.items.status.text).toBe('Reading.');
  expect(root.querySelector('.attention-item-source').textContent).toBe('Builder');
  expect(root.querySelector('.disclosure-heading').textContent).toBe('From Builder');
  await act(async () => choose(root, 'yes'));
  expect(new URL(fetches.mock.calls[0][0], 'http://x/').searchParams.get('id')).toBe('child');
  send([{id: 'request', kind: 'attention', from: 'child', text: 'Proceed?', done: true, answer: 'yes'}]);
  expect(root.querySelector('.attention-item')).toBeNull();
  expect(root.textContent).toContain('Result.');
});

test('saved completed child requests are hidden while the session\'s own answers remain', () => {
  const {root} = setup([
    {id: 'child-request', kind: 'attention', from: 'child', text: 'Child approval?', done: true, answer: 'Allow'},
    {id: 'own-request', kind: 'attention', text: 'Own approval?', done: true, answer: 'Deny'},
  ]);
  expect(root.textContent).not.toContain('Child approval?');
  expect(root.textContent).not.toContain('Allow');
  expect(root.querySelector('.attention-item-answered').textContent).toBe('Own approval?');
  expect(root.querySelector('.attention-item-answer').textContent).toBe('Deny');
});
