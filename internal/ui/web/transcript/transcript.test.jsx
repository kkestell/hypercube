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

function setup(items) {
  const store = createStore({workspaces: [], drafts: {}, errors: [], session: emptySession('s')});
  const send = (items, reset = false) => store.update(s => receivedTranscript(s, {session: 's', reset, items}));
  send(withSeq(items), true);
  const root = mount(store, <Transcript session={{id: "s"}} />);
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
  expect(call().querySelector('summary').textContent).toContain('bash ls');
  expect(call().querySelector('[title="Running"]')).not.toBeNull();
  expect(call().querySelector('pre').textContent).toBe('{\n  "command": "ls"\n}');
  send([{id: 'n', parent: 'c', kind: 'text', text: 'inside'}, {id: 'c', kind: 'tool', tool: 'bash', input: {command: 'ls'}, output: 'a.txt', done: true, failed: true}]);
  expect(call().querySelector('[title="Running"]')).toBeNull();
  expect(call().querySelector('[title="Failed"]')).not.toBeNull();
  expect(call().querySelectorAll('pre')[1].textContent).toBe('a.txt');
  expect(call().querySelector('.tool-items-children').textContent).toBe('inside');
});

test.each([['the user', undefined], ['another session', 'child']])('a tool run passes over a message from %s and shows working details until other output', (_source, from) => {
  const first = {id: 'a', kind: 'tool', tool: 'read', input: {path: '/first'}, at: 1000, seq: 1};
  const {root, send} = setup([first]);
  const list = root.querySelector('.transcript-list');
  const group = list.firstElementChild;
  const summary = group.querySelector('summary');
  const chevron = summary.querySelector('.disclosure-chevron');
  expect(chevron).not.toBeNull();
  expect(summary.querySelector('[title="Running"]')).toBeNull();
  const heading = summary.querySelector('.disclosure-heading');
  expect(heading.lastElementChild).toBe(chevron);
  expect([...summary.children].map(line => line.textContent)).toEqual(['Working…', 'read /first']);
  act(() => summary.click());
  expect(group.open).toBe(true);
  expect(summary.textContent).toBe('Working…');
  expect(summary.querySelector('.disclosure-preview')).toBeNull();
  expect(group.querySelector('.tool-items-call [title="Running"]')).not.toBeNull();

  send([{...first, done: true}, {id: 'u', kind: 'user', from, text: 'Keep going', at: 3000, seq: 2}]);
  const user = list.children[1];
  expect(user.textContent).toContain('Keep going');
  expect(summary.querySelector('.disclosure-preview')).toBeNull();

  send([{id: 'b', kind: 'tool', tool: 'bash', input: {command: 'x'.repeat(200)}, at: 5000, seq: 3}]);
  expect([...list.children]).toEqual([group, user]);
  expect(group.open).toBe(true);
  expect(group.querySelectorAll('.tool-items-call').length).toBe(2);
  expect(summary.textContent).toBe('Working…');
  expect(summary.querySelector('[title="Running"]')).toBeNull();
  expect(group.querySelectorAll('.tool-items-call [title="Running"]').length).toBe(1);

  act(() => summary.click());
  expect(group.open).toBe(false);
  expect(summary.querySelector('.disclosure-preview').textContent).toBe('bash ' + 'x'.repeat(119) + '…');
  act(() => summary.click());
  expect(group.open).toBe(true);
  expect(summary.querySelector('.disclosure-preview')).toBeNull();

  send([{id: 't', kind: 'text', text: 'Finished', at: 10000, seq: 4}]);
  expect(list.firstElementChild).toBe(group);
  expect(group.open).toBe(true);
  expect(summary.textContent).toBe('Worked for 9s');
  expect(heading.lastElementChild).toBe(chevron);
  expect(summary.querySelector('.disclosure-preview')).toBeNull();
  expect(summary.querySelector('[title="Running"]')).toBeNull();
});

const choose = (root, choice) => {
  const row = root.querySelector(`vscode-tree-item[data-choice="${choice}"]`);
  act(() => row.closest('vscode-tree').dispatchEvent(new CustomEvent('vsc-tree-select', {detail: [row]})));
};

test('an attention request offers its choices, waits while sending, and opens again when refused', async () => {
  let answer;
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(() => new Promise(resolve => { answer = resolve; }));
  spies.push(fetches);
  const {root, send} = setup([{id: 'q', kind: 'attention', text: 'Proceed?', choices: ['yes', 'no']}]);
  const rows = () => [...root.querySelectorAll('vscode-tree-item')];
  const list = () => root.querySelector('vscode-tree');
  expect(list().getAttribute('aria-label')).toBe('Choices');
  expect(rows().map(r => r.textContent)).toEqual(['yes', 'no']);
  expect(root.querySelector('vscode-textfield')).toBeNull();
  choose(root, 'yes');
  const url = new URL(fetches.mock.calls[0][0], 'http://x/');
  expect(Object.fromEntries(url.searchParams)).toEqual({action: 'answer', id: 's', request: 'q', choice: 'yes'});
  expect(list().getAttribute('aria-disabled')).toBe('true');
  choose(root, 'no');
  expect(fetches).toHaveBeenCalledTimes(1);
  await act(async () => {
    answer(new Response('no longer open', {status: 409}));
    await new Promise(resolve => setTimeout(resolve));
  });
  expect(list().getAttribute('aria-disabled')).toBe('false');

  send([{id: 'q', kind: 'attention', text: 'Proceed?', done: true, answer: 'no', seq: 1}]);
  expect(rows()).toEqual([]);
  expect(root.querySelector('.attention-item-answered').textContent).toBe('Proceed?');
  expect(root.querySelector('.attention-item-answer').textContent).toBe('no');
});

test('a choice shows its note after the label', () => {
  const {root} = setup([{id: 'q', kind: 'attention', text: 'Language?', choices: ['Python', 'Go'], notes: ['Readable', '']}]);
  const [python, go] = root.querySelectorAll('vscode-tree-item');
  expect(python.firstChild.textContent).toBe('Python');
  expect(python.querySelector('[slot="description"]').textContent).toBe('Readable');
  expect(go.querySelector('[slot="description"]')).toBeNull();
});

test('a typed answer is sent with Enter and not when empty', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  spies.push(fetches);
  const {root} = setup([{id: 'q', kind: 'attention', text: 'Name?', free: true, choices: ['Ada']}]);
  const field = root.querySelector('vscode-textfield');
  const sendButton = field.querySelector('vscode-button[slot="content-after"]');
  expect(sendButton.getAttribute('aria-label')).toBe('Send');
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

test('the Send button in the field sends the typed answer', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  spies.push(fetches);
  const {root} = setup([{id: 'q', kind: 'attention', text: 'Name?', free: true}]);
  const field = root.querySelector('vscode-textfield');
  field.value = 'Grace';
  act(() => field.dispatchEvent(new Event('input')));
  await act(async () => field.querySelector('vscode-button').click());
  expect(fetches.mock.calls[0][1].body).toBe('Grace');
});

test('a request inside collapsed tool calls opens them', () => {
  const {root} = setup([
    {id: 'a', kind: 'tool', tool: 'agent'},
    {id: 'b', parent: 'a', kind: 'tool', tool: 'inner'},
    {id: 'q', parent: 'b', kind: 'attention', text: 'Allow?', choices: ['ok']},
  ]);
  const details = [...root.querySelectorAll('details')];
  expect(details.length).toBe(4);
  expect(details.every(d => d.open)).toBe(true);
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

test('agent messages show the sender and first line, and expand to the full message', () => {
  const {root, send} = setup([
    {id: 'in', kind: 'user', from: 'c1', text: longText, done: true},
    {id: 'out', kind: 'tool', server: 'hypercube', tool: 'send_message', input: {to: 'c1', text: longText}},
    {id: 'own', kind: 'user', text: longText},
  ]);
  const [incoming, outgoing, own] = root.querySelectorAll('.message-item');
  expect(incoming.querySelector('.disclosure-heading').textContent).toBe('From c1');
  expect(outgoing.querySelector('.disclosure-heading').textContent).toBe('To c1');
  expect(outgoing.querySelector('[title="Running"]')).not.toBeNull();
  for (const message of [incoming, outgoing]) {
    expect(message.querySelector('.message-item-bubble')).toBeNull();
    expect(message.querySelector('.disclosure-preview').textContent).toBe('line 0');
    expect(message.querySelector('.message-item-text')).toBeNull();
    act(() => message.querySelector('summary').click());
    expect(message.open).toBe(true);
    expect(message.querySelector('.disclosure-preview')).toBeNull();
    expect(message.querySelector('.message-item-text').textContent).toBe(longText);
  }
  send([{id: 'out', kind: 'tool', server: 'hypercube', tool: 'send_message', input: {to: 'c1', text: longText}, done: true, failed: true, output: 'Delivery failed'}]);
  expect(outgoing.open).toBe(true);
  expect(outgoing.querySelector('[title="Running"]')).toBeNull();
  expect(outgoing.querySelector('[title="Failed"]')).not.toBeNull();
  expect(outgoing.querySelector('.message-item-body .message-item-failure').textContent).toBe('Delivery failed');
  act(() => incoming.querySelector('summary').click());
  expect(incoming.open).toBe(false);
  expect(incoming.querySelector('.disclosure-preview').textContent).toBe('line 0');
  expect(own.querySelector('.message-item-bubble')).not.toBeNull();
  expect(own.querySelector('.message-item-text').textContent).toBe(longText);
  expect(own.querySelector('summary')).toBeNull();
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

test('Thinking shows a preview until expanded and keeps the choice while it streams', () => {
  const {root, send} = setup([{id: 't', kind: 'reasoning', text: longText}]);
  const preview = () => root.querySelector('.reasoning-item-text').classList.contains('reasoning-item-preview');
  expect(preview()).toBe(true);
  act(() => root.querySelector('.reasoning-item summary').click());
  expect(preview()).toBe(false);
  send([{id: 't', kind: 'reasoning', text: longText + '\nmore', seq: 1}]);
  expect(preview()).toBe(false);
  expect(root.querySelector('.reasoning-item-text').textContent).toEndWith('more');
});

const png = 'iVBORw0KGgo=';

test('prompts, text, and tool calls show their images, outside the call\'s details', () => {
  const {root} = setup([
    {id: 'u', kind: 'user', text: 'look', images: [{mime_type: 'image/png', data: png}]},
    {id: 'x', kind: 'text', images: [{mime_type: 'image/gif', data: 'R0lGODlh'}]},
    {id: 'c', kind: 'tool', tool: 'screenshot', done: true, images: [{mime_type: 'image/png', data: png}, {mime_type: 'image/svg+xml', data: png}]},
  ]);
  const sources = selector => [...root.querySelectorAll(selector + ' img')].map(img => img.getAttribute('src'));
  expect(sources('.message-item')).toEqual(['data:image/png;base64,' + png]);
  expect(sources('.transcript-text')).toEqual(['data:image/gif;base64,R0lGODlh']);
  expect(sources('.tool-items-call')).toEqual(['data:image/png;base64,' + png]);
  expect(sources('.tool-items-call details')).toEqual([]);
});

test('mirrored requests name their sender and answer at its session ID; child status stays hidden', async () => {
  const fetches = spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(''));
  spies.push(fetches);
  const {store, root} = setup([
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
});
