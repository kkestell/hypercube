import {expect, test} from 'bun:test';
import {toolSummary, toolTitle, transcriptRows} from './transcript-tree.js';

const transcript = items => ({order: items.map(item => item.id), items: Object.fromEntries(items.map(item => [item.id, item]))});
const rows = items => transcriptRows(transcript(items));
const call = (id, tool, at, extra = {}) => ({id, kind: 'tool', tool, at, ...extra});
const text = (id, at, extra = {}) => ({id, kind: 'text', text: id, at, ...extra});
const user = (id, at, extra = {}) => ({id, kind: 'user', text: id, at, ...extra});
// Hypercube's state items, which name their launch before the colon.
const working = (id, at, launch = 'L1') => ({id: `${launch}:${id}`, kind: 'state', state: 'working', done: true, at});
const idle = (id, at, final = [], launch = 'L1') => ({id: `${launch}:${id}`, kind: 'state', state: 'idle', done: true, at, final});
const shape = list => list.map(row => row.type === 'activity' ? ['activity', row.rows.map(r => r.item.id)] : row.item.id);

test('toolSummary picks the telling text of a call', () => {
  expect(toolSummary({command: 'ls -la\n  /tmp', timeout: 5})).toBe('ls -la /tmp');
  expect(toolSummary({file_path: '/a/b.go', content: 'x'})).toBe('/a/b.go');
  expect(toolSummary({n: 1, label: 'first string'})).toBe('first string');
  expect(toolSummary('plain')).toBe('plain');
  expect(toolSummary({n: 1})).toBe('');
  expect(toolSummary(undefined)).toBe('');
  expect(toolSummary({command: 'x'.repeat(200)}).length).toBe(120);
});

test('toolTitle says what Hypercube tools do and names other servers', () => {
  expect(toolTitle({server: 'hypercube', tool: 'end_session', input: {name: 'auditor'}})).toEqual(['End session', 'auditor']);
  expect(toolTitle({server: 'hypercube', tool: 'create_session', input: {name: 'w1', profile: 'claude', message: 'the brief'}})).toEqual(['Create session', 'w1']);
  expect(toolTitle({server: 'hypercube', tool: 'list_sessions', input: {}})).toEqual(['List sessions', '']);
  expect(toolTitle({server: 'hypercube', tool: 'end_session'})).toEqual(['End session', '']);
  expect(toolTitle({server: 'context7', tool: 'query-docs', input: {query: 'go'}})).toEqual(['context7 - query-docs', 'go']);
  expect(toolTitle({tool: 'bash', input: {command: 'ls'}})).toEqual(['bash', 'ls']);
});

test('a successful turn is its opener, one activity, and its response', () => {
  const result = rows([
    user('ask', 1000),
    working('w', 1500),
    {id: 'think', kind: 'reasoning', text: 'hm', at: 2000},
    call('read', 'read', 3000, {done: true}),
    text('progress', 4000),
    text('answer', 5000),
    idle('i', 9000, ['answer']),
  ]);
  expect(shape(result)).toEqual(['ask', ['activity', ['think', 'read', 'progress']], 'answer']);
  expect(result[1]).toMatchObject({key: 'activity:L1:w', start: 1000, until: 9000});
  expect(result[0].inline).toBeUndefined();
});

test('every final block follows the activity, and a response alone has no activity', () => {
  expect(shape(rows([user('ask', 1), working('w', 2), text('a', 3), call('c', 'read', 4), text('b', 5), idle('i', 6, ['a', 'b'])])))
    .toEqual(['ask', ['activity', ['c']], 'a', 'b']);
  expect(shape(rows([user('ask', 1), working('w', 2), text('a', 3), idle('i', 4, ['a'])]))).toEqual(['ask', 'a']);
});

test('the duration starts at the working state when no input began the turn', () => {
  const [activity] = rows([working('w', 1500), call('c', 'read', 2000), text('a', 3000), idle('i', 7500, ['a'])]);
  expect(activity).toMatchObject({type: 'activity', start: 1500, until: 7500});
});

test('a running turn draws every row as it is, with no activity', () => {
  const result = rows([user('ask', 1), working('w', 2), {id: 'think', kind: 'reasoning', text: 'hm', at: 3}, call('c', 'read', 4), text('progress', 5)]);
  expect(result.map(row => row.type)).toEqual(['user', 'reasoning', 'tool', 'text']);
});

test.each([['the user', undefined], ['another session', 'child']])('a message from %s during the turn is activity, in order', (_source, from) => {
  const items = [user('ask', 1), working('w', 2), call('a', 'read', 3, {done: true}), user('more', 4, {from}), call('b', 'bash', 5, {done: true}), text('answer', 6), idle('i', 7, ['answer'])];
  const result = rows(items);
  expect(shape(result)).toEqual(['ask', ['activity', ['a', 'more', 'b']], 'answer']);
  expect(result[1].rows[1]).toMatchObject({type: 'user', inline: true});
  // While the turn runs, the message draws in place among the other rows.
  const running = rows(items.slice(0, 5));
  expect(running.map(row => row.item.id)).toEqual(['ask', 'a', 'more', 'b']);
  expect(running[2].inline).toBe(true);
  expect(running[0].inline).toBeUndefined();
});

test('an idle state outside a turn leaves the message that will begin the next one', () => {
  const [opener, activity, answer] = rows([user('ask', 1000), idle('start', 2000), working('w', 5000), call('c', 'read', 6000), text('answer', 9000), idle('i', 10000, ['answer'])]);
  expect(opener.item.id).toBe('ask');
  expect(activity).toMatchObject({type: 'activity', start: 1000, until: 10000});
  expect(answer.item.id).toBe('answer');
});

test('of several messages before the working state, the first begins the turn', () => {
  const result = rows([user('first', 1), user('second', 2), working('w', 3), text('answer', 4), idle('i', 5, ['answer'])]);
  expect(shape(result)).toEqual(['first', ['activity', ['second']], 'answer']);
  expect(result[1].start).toBe(1);
});

test('a peer message that begins a turn stays above its activity', () => {
  const result = rows([user('note', 1, {from: 'child'}), working('w', 2), call('c', 'read', 3), text('answer', 4), idle('i', 5, ['answer'])]);
  expect(shape(result)).toEqual(['note', ['activity', ['c']], 'answer']);
});

test('earlier turns stay grouped while a later turn runs', () => {
  const result = rows([
    user('one', 1), working('w1', 2), call('a', 'read', 3), text('first', 4), idle('i1', 5, ['first']),
    user('two', 6), working('w2', 7), call('b', 'bash', 8),
  ]);
  expect(shape(result)).toEqual(['one', ['activity', ['a']], 'first', 'two', 'b']);
});

test('repeated working states of one launch do not split a turn', () => {
  expect(shape(rows([user('ask', 1), working('w1', 2), call('a', 'read', 3), working('w2', 4), call('b', 'bash', 5), text('answer', 6), idle('i', 7, ['answer'])])))
    .toEqual(['ask', ['activity', ['a', 'b']], 'answer']);
});

test.each([
  ['an idle state without a final', [user('ask', 1), working('w', 2), call('a', 'read', 3), text('partial', 4), idle('i', 5)]],
  ['a final that names nothing in the turn', [user('ask', 1), working('w', 2), call('a', 'read', 3), idle('i', 4, ['missing'])]],
  ['a transcript without state items', [user('ask', 1), call('a', 'read', 2), text('answer', 3)]],
])('%s leaves every row as it is', (_name, items) => {
  expect(rows(items).every(row => row.type !== 'activity')).toBe(true);
  expect(rows(items).map(row => row.item.id)).toEqual(items.filter(item => item.kind !== 'state').map(item => item.id));
});

test('a later launch does not group what an earlier launch left unfinished', () => {
  const result = rows([
    user('old', 1), working('w1', 2), call('lost', 'shell', 3), text('stale', 4),
    idle('start', 10, [], 'L2'),
    user('new', 11), working('w2', 12, 'L2'), call('b', 'bash', 13, {done: true}), text('answer', 14), idle('i2', 15, ['answer'], 'L2'),
  ]);
  expect(shape(result)).toEqual(['old', 'lost', 'stale', 'new', ['activity', ['b']], 'answer']);
  expect(result[4]).toMatchObject({key: 'activity:L2:w2', start: 11, until: 15});
});

test('a working state of a new launch ends the old launch\'s turn without grouping it', () => {
  const result = rows([user('old', 1), working('w1', 2), call('lost', 'shell', 3), working('w2', 4, 'L2'), text('answer', 5), idle('i', 6, ['answer'], 'L2')]);
  expect(shape(result)).toEqual(['old', 'lost', 'answer']);
});

test('unknown kinds draw as text and state items stay hidden', () => {
  const result = rows([user('ask', 1), working('w', 2), {id: 'unknown', kind: 'unknown', from: 'child', text: 'Working', at: 3}, call('a', 'read', 4), idle('i', 5)]);
  expect(result.map(row => row.item.id)).toEqual(['ask', 'unknown', 'a']);
  expect(result[1].type).toBe('text');
});

test('nested items draw under their call, in order', () => {
  const [outer, next] = rows([
    call('parent', 'agent', 1000),
    call('one', 'read', 2000, {parent: 'parent', done: true}),
    {id: 'child-text', parent: 'parent', kind: 'text', text: 'Child finished', at: 6000},
    call('next', 'edit', 7000),
  ]);
  expect(outer.children.map(row => row.item.id)).toEqual(['one', 'child-text']);
  expect(next.item.id).toBe('next');
});

test('nested items inside a finished turn stay under their call', () => {
  const [, activity] = rows([user('ask', 1), working('w', 2), call('p', 'agent', 3), call('n', 'read', 4, {parent: 'p'}), text('answer', 5), idle('i', 6, ['answer'])]);
  expect(activity.rows[0].children.map(row => row.item.id)).toEqual(['n']);
});

test('an item that arrives before its parent moves under it', () => {
  const child = {id: 'c', parent: 'p', kind: 'text', seq: 1};
  expect(rows([child]).map(r => r.item.id)).toEqual(['c']);
  const [parent] = rows([child, call('p', 'agent', 0, {seq: 2})]);
  expect(parent.children.map(r => r.item.id)).toEqual(['c']);
});

test('message calls draw as messages', () => {
  const rowsOfCalls = rows([call('a', 'read', 1000), {id: 'm', kind: 'tool', server: 'hypercube', tool: 'send_message', input: {to: 'lead', text: 'Result'}}]);
  expect(rowsOfCalls.map(row => row.type)).toEqual(['tool', 'message']);
});

test('history omits open requests and keeps answered requests under their calls', () => {
  const [parent] = rows([
    call('a', 'agent', 1),
    {id: 'q', parent: 'a', kind: 'attention', at: 2},
    {id: 'answered', parent: 'a', kind: 'attention', at: 3, done: true, answer: 'yes'},
    {id: 'raw', kind: 'attention', at: 4},
  ]);
  expect(parent.children.map(row => row.item.id)).toEqual(['answered']);
});
