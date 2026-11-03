import {expect, test} from 'bun:test';
import {ancestorKeys, toolSummary, toolTitle, transcriptRows} from './transcript-tree.js';

const transcript = items => ({order: items.map(item => item.id), items: Object.fromEntries(items.map(item => [item.id, item]))});
const rows = items => transcriptRows(transcript(items));
const call = (id, tool, at, extra = {}) => ({id, kind: 'tool', tool, at, ...extra});
const ids = calls => calls.map(c => c.item.id);

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
  expect(toolTitle({server: 'hypercube', tool: 'set_status', input: {text: 'Reading\n the code'}})).toEqual(['Set status', 'Reading the code']);
  expect(toolTitle({server: 'hypercube', tool: 'end_session'})).toEqual(['End session', '']);
  expect(toolTitle({server: 'context7', tool: 'query-docs', input: {query: 'go'}})).toEqual(['context7 - query-docs', 'go']);
  expect(toolTitle({tool: 'bash', input: {command: 'ls'}})).toEqual(['bash', 'ls']);
});

test('consecutive calls share one open run', () => {
  const [run] = rows([call('a', 'read', 1000, {done: true}), call('b', 'bash', 2000)]);
  expect(run).toMatchObject({type: 'run', key: 'run:a', closed: false});
  expect(ids(run.calls)).toEqual(['a', 'b']);
});

test('child status updates stay hidden and do not split tool runs', () => {
  const [run, text] = rows([
    call('a', 'read', 1000, {done: true}),
    {id: 'status', kind: 'status', from: 'child', text: 'Working', at: 2000},
    call('b', 'bash', 3000, {done: true}),
    {id: 'text', kind: 'text', text: 'Done', at: 4000},
  ]);
  expect(ids(run.calls)).toEqual(['a', 'b']);
  expect(run).toMatchObject({start: 1000, until: 4000});
  expect(text.item.id).toBe('text');
  expect(rows([{id: 'status', kind: 'status', text: 'Working'}])).toEqual([]);
});

test.each(['text', 'reasoning', 'error', 'attention'])('the next %s item closes the run with its duration', kind => {
  const [run, next] = rows([call('a', 'read', 1000), call('b', 'bash', 2000, {done: true, failed: true}), {id: 't', kind, at: 33000}]);
  expect(run).toMatchObject({closed: true, start: 1000, until: 33000});
  expect(next.type).toBe(kind);
});

test('a run with missing timestamps has no times', () => {
  expect(rows([call('a', 'read'), {id: 't', kind: 'text'}])[0]).toMatchObject({closed: true, start: 0, until: 0});
});

test('a run still open when the session is not working closes at its last call\'s end', () => {
  const items = [call('a', 'read', 1000, {done: true, end: 2000}), call('b', 'bash', 3000, {done: true, end: 7000}), call('c', 'edit', 4000, {done: true, end: 5000})];
  expect(rows(items)[0]).toMatchObject({closed: false, until: 0});
  expect(transcriptRows(transcript(items), false)[0]).toMatchObject({closed: true, start: 1000, until: 7000});
  expect(transcriptRows(transcript([call('a', 'read', 1000, {done: true})]), false)[0]).toMatchObject({closed: true, start: 1000, until: 0});
  const nested = transcriptRows(transcript([call('p', 'agent', 1000, {end: 9000}), call('n', 'read', 2000, {parent: 'p', end: 3000})]), false);
  expect(nested[0].calls[0].children[0]).toMatchObject({closed: true, until: 3000});
});

test.each([['the user', undefined], ['another session', 'child']])('a message from %s leaves the run open and later calls join above it', (_source, from) => {
  const items = [call('a', 'read', 1000, {done: true}), {id: 'u', kind: 'user', from, text: 'Continue', at: 3000}];
  const [open, user] = rows(items);
  expect(open).toMatchObject({key: 'run:a', closed: false, until: 0});
  expect(user).toEqual({type: 'user', item: items[1]});

  items.push(call('b', 'bash', 5000, {done: true}));
  const joined = rows(items);
  expect(joined.map(row => row.type)).toEqual(['run', 'user']);
  expect(joined[0]).toMatchObject({key: 'run:a', closed: false, until: 0});
  expect(ids(joined[0].calls)).toEqual(['a', 'b']);

  items.push({id: 't', kind: 'text', text: 'Done', at: 10000});
  const closed = rows(items);
  expect(closed.map(row => row.type)).toEqual(['run', 'user', 'text']);
  expect(closed[0]).toMatchObject({key: 'run:a', closed: true, start: 1000, until: 10000});
  expect(ids(closed[0].calls)).toEqual(['a', 'b']);
});

test.each([['the user', undefined], ['another session', 'child']])('a message from %s leaves a closed run closed and later calls start a new run', (_source, from) => {
  const items = [call('a', 'read', 1000), {id: 't', kind: 'text', text: 'Done', at: 4000}];
  const closed = rows(items)[0];
  items.push({id: 'u', kind: 'user', from, text: 'Continue', at: 6000});
  expect(rows(items)[0]).toEqual(closed);

  items.push(call('b', 'bash', 8000));
  const result = rows(items);
  expect(result.map(row => row.type)).toEqual(['run', 'text', 'user', 'run']);
  expect(result[0]).toEqual(closed);
  expect(result[3]).toMatchObject({key: 'run:b', closed: false, until: 0});
  expect(ids(result[3].calls)).toEqual(['b']);
});

test('replayed history keeps separate runs and their durations', () => {
  const result = rows([
    call('a', 'read', 1000, {done: true}),
    call('b', 'bash', 5000, {done: true}),
    {id: 'r', kind: 'reasoning', text: 'Next', at: 33000},
    call('c', 'edit', 34000, {done: true}),
    {id: 'u', kind: 'user', text: 'Continue', at: 36000},
  ]);
  expect(result.map(r => r.type)).toEqual(['run', 'reasoning', 'run', 'user']);
  expect(result[0]).toMatchObject({start: 1000, until: 33000});
  expect(result[2]).toMatchObject({closed: false, until: 0});
});

test('nested items group among their siblings and leave the outer run open', () => {
  const [outer] = rows([
    call('parent', 'agent', 1000),
    call('one', 'read', 2000, {parent: 'parent'}),
    call('two', 'bash', 3000, {parent: 'parent'}),
    {id: 'child-text', parent: 'parent', kind: 'text', text: 'Child finished', at: 6000},
    call('next', 'edit', 7000),
  ]);
  expect(outer.closed).toBe(false);
  expect(ids(outer.calls)).toEqual(['parent', 'next']);
  const [inner, text] = outer.calls[0].children;
  expect(ids(inner.calls)).toEqual(['one', 'two']);
  expect(inner).toMatchObject({start: 2000, until: 6000});
  expect(text.item.id).toBe('child-text');
});

test('an item that arrives before its parent moves under it', () => {
  const child = {id: 'c', parent: 'p', kind: 'text', seq: 1};
  expect(rows([child]).map(r => r.item.id)).toEqual(['c']);
  const [run] = rows([child, call('p', 'agent', 0, {seq: 2})]);
  expect(run.calls[0].children.map(r => r.item.id)).toEqual(['c']);
});

test('message calls join runs and draw as messages', () => {
  const [run] = rows([call('a', 'read', 1000), {id: 'm', kind: 'tool', server: 'hypercube', tool: 'send_message', input: {to: 'lead', text: 'Result'}}, call('b', 'bash', 3000)]);
  expect(run.calls.map(c => c.type)).toEqual(['tool', 'message', 'tool']);
});

test('ancestorKeys names the runs and calls around an item', () => {
  const items = transcript([call('a', 'agent'), call('b', 'inner', 0, {parent: 'a'}), {id: 'q', parent: 'b', kind: 'attention'}]);
  expect(ancestorKeys(items, 'q')).toEqual(['run:a', 'a', 'run:b', 'b']);
  expect(ancestorKeys(items, 'missing')).toEqual([]);
});
