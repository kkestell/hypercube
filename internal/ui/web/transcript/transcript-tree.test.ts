import {expect, test} from 'bun:test';
import {toolSummary, toolTitle, transcriptRows, type ActivityRow, type Row, type ToolRow} from './transcript-tree.ts';
import type {AttentionItem, TextItem, ToolItem, TranscriptItem, UserItem} from './types.ts';

const transcript = (items: TranscriptItem[]) => ({order: items.map(item => item.id), items: Object.fromEntries(items.map(item => [item.id, item]))});
const rows = (items: TranscriptItem[]) => transcriptRows(transcript(items));
const tool = (extra: Partial<ToolItem>): ToolItem => ({id: 'tool', kind: 'tool', ...extra});
const call = (id: string, tool: string, at: number, extra: Partial<ToolItem> & {seq?: number} = {}): TranscriptItem => ({id, kind: 'tool', tool, at, seq: 0, ...extra});
const text = (id: string, at: number, extra: Partial<TextItem> = {}): TranscriptItem => ({id, kind: 'text', text: id, at, seq: 0, ...extra});
const user = (id: string, at: number, extra: Partial<UserItem> = {}): TranscriptItem => ({id, kind: 'user', text: id, at, seq: 0, ...extra});
const attention = (id: string, at: number, extra: Partial<AttentionItem> = {}): TranscriptItem => ({id, kind: 'attention', at, seq: 0, ...extra});
// Hypercube's state items, which name their launch before the colon.
const working = (id: string, at: number, launch = 'L1'): TranscriptItem => ({id: `${launch}:${id}`, kind: 'state', state: 'working', done: true, at, seq: 0});
const idle = (id: string, at: number, final: string[] = [], launch = 'L1'): TranscriptItem => ({id: `${launch}:${id}`, kind: 'state', state: 'idle', done: true, at, final, seq: 0});
const ids = (list: Row[]) => list.map(row => row.type === 'activity' ? row.key : row.item.id);
const shape = (list: Row[]) => list.map(row => row.type === 'activity' ? ['activity', row.rows.map(r => r.item.id)] : row.item.id);
const activityAt = (list: Row[], index: number): ActivityRow => {
  const row = list[index];
  if (row?.type !== 'activity') throw new Error(`row ${index} is not an activity`);
  return row;
};
const toolAt = (list: Row[], index: number): ToolRow => {
  const row = list[index];
  if (row?.type !== 'tool') throw new Error(`row ${index} is not a tool call`);
  return row;
};

test('only exact adjacent text copies of structured questions are omitted', () => {
  const question = attention('q', 2, {text: 'Which size?', choices: ['Small', 'Large'], done: true, answer: 'Small'});
  const copy: TextItem & {seq: number} = {id: 'copy', kind: 'text', seq: 0, text: 'Which size?\n- Small\n- Large'};
  expect(ids(rows([copy, question]))).toEqual(['q']);
  expect(ids(rows([{...copy, text: 'Context.\n' + copy.text}, question]))).toEqual(['copy', 'q']);
  expect(ids(rows([{...copy, images: ['/transcript-image?index=0']}, question]))).toEqual(['copy', 'q']);
  expect(ids(rows([copy, {...question, parent: 'other'}]))).toEqual(['copy', 'q']);
  expect(ids(rows([copy, text('between', 2), question]))).toEqual(['copy', 'between', 'q']);
});

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
  expect(toolTitle(tool({server: 'hypercube', tool: 'end_session', input: {name: 'auditor'}}))).toEqual(['End session', 'auditor']);
  expect(toolTitle(tool({server: 'hypercube', tool: 'create_session', input: {name: 'w1', title: 'Fix the parser', profile: 'claude', message: 'the brief'}}))).toEqual(['Create session', 'Fix the parser']);
  expect(toolTitle(tool({server: 'hypercube', tool: 'create_session', input: {initiation: 'offer', title: 'Fix the parser'}}))).toEqual(['Create session', 'Fix the parser']);
  expect(toolTitle(tool({server: 'hypercube', tool: 'list_sessions', input: {}}))).toEqual(['List sessions', '']);
  expect(toolTitle(tool({server: 'hypercube', tool: 'list_models', input: {harness: 'claude'}}))).toEqual(['List models', 'claude']);
  expect(toolTitle(tool({server: 'hypercube', tool: 'read_session', input: {to: 'w1', cursor: 4}}))).toEqual(['Read session', 'w1']);
  expect(toolTitle(tool({server: 'hypercube', tool: 'set_title', input: {title: 'Fix tool rendering'}}))).toEqual(['Set title', 'Fix tool rendering']);
  expect(toolTitle(tool({server: 'hypercube', tool: 'open_file', input: {path: 'main.go', lines: [{start: 10}]}}))).toEqual(['Open file', 'main.go']);
  expect(toolTitle(tool({server: 'hypercube', tool: 'end_session'}))).toEqual(['End session', '']);
  expect(toolTitle(tool({server: 'hypercube', tool: 'open_file', input: {path: 42}}))).toEqual(['Open file', '']);
  expect(toolTitle(tool({server: 'hypercube', tool: 'unknown', input: {query: 'go'}}))).toEqual(['hypercube - unknown', 'go']);
  expect(toolTitle(tool({server: 'context7', tool: 'query-docs', input: {query: 'go'}}))).toEqual(['context7 - query-docs', 'go']);
  expect(toolTitle(tool({tool: 'bash', input: {command: 'ls'}}))).toEqual(['Run command', 'ls']);
});

// The built-in tools of each harness, with their input fields, and the title
// each should draw. The lists come from Claude Code 2.1.319 (the tools of one of
// its sessions and the tool definitions in its binary, less those for
// Anthropic's own cloud products), OpenCode 2.2.6 (each opencode.tool.*
// registration in its binary), and Pi 1.4.0 (allToolNames in its package and
// its bundled extensions). MCP tools of other servers draw generic titles.
test.each([
  ['Claude Code', [
    ['Bash', {command: 'make check', description: 'Run checks'}, 'Run command', 'make check'],
    ['PowerShell', {command: 'dir'}, 'Run command', 'dir'],
    ['Monitor', {command: 'tail -f log', description: 'Watch the log'}, 'Monitor', 'tail -f log'],
    ['Read', {file_path: '/a/b.go', offset: 1}, 'Read', '/a/b.go'],
    ['Write', {file_path: '/a/b.go', content: 'x'}, 'Write', '/a/b.go'],
    ['Edit', {file_path: '/a/b.go', old_string: 'x', new_string: 'y'}, 'Edit', '/a/b.go'],
    ['NotebookEdit', {cell_id: 'c1', new_source: 'x', notebook_path: '/a/n.ipynb'}, 'Edit notebook', '/a/n.ipynb'],
    ['Glob', {path: '/a', pattern: '**/*.go'}, 'Glob', '**/*.go'],
    ['Grep', {path: '/a', pattern: 'func main', glob: '*.go'}, 'Grep', 'func main'],
    ['LSP', {operation: 'hover', filePath: '/a/b.go', line: 3}, 'LSP', '/a/b.go'],
    ['WebFetch', {url: 'https://go.dev', prompt: 'Summarize'}, 'Web fetch', 'https://go.dev'],
    ['WebSearch', {query: 'go generics'}, 'Web search', 'go generics'],
    ['Agent', {subagent_type: 'Explore', description: 'Find the parser', prompt: 'Look for…'}, 'Run agent', 'Find the parser'],
    ['Task', {subagent_type: 'Explore', description: 'Find the parser', prompt: 'Look for…'}, 'Run agent', 'Find the parser'],
    ['Skill', {args: 'x', skill: 'pdf'}, 'Skill', 'pdf'],
    ['ToolSearch', {query: 'select:WebFetch', max_results: 1}, 'Tool Search', 'select:WebFetch'],
    ['TodoWrite', {todos: [{content: 'a', status: 'pending', activeForm: 'Doing a'}]}, 'Update todos', ''],
    ['TaskCreate', {description: 'Long text', subject: 'Fix the parser'}, 'Task Create', 'Fix the parser'],
    ['TaskGet', {taskId: '1'}, 'Task Get', '1'],
    ['TaskList', {}, 'Task List', ''],
    ['TaskUpdate', {taskId: '1', status: 'completed'}, 'Task Update', '1'],
    ['TaskStop', {task_id: 'b1'}, 'Task Stop', 'b1'],
    ['CronCreate', {cron: '0 9 * * *', prompt: 'Check CI'}, 'Cron Create', 'Check CI'],
    ['CronDelete', {id: 'c1'}, 'Cron Delete', 'c1'],
    ['CronList', {}, 'Cron List', ''],
    ['EnterPlanMode', {}, 'Enter Plan Mode', ''],
    ['ExitPlanMode', {plan: 'Step one'}, 'Exit Plan Mode', 'Step one'],
    ['EnterWorktree', {name: 'fix'}, 'Enter Worktree', 'fix'],
    ['ExitWorktree', {action: 'keep'}, 'Exit Worktree', 'keep'],
    ['ListMcpResourcesTool', {server: 'docs'}, 'List MCP resources', 'docs'],
    ['ReadMcpResourceTool', {server: 'docs', uri: 'docs://a'}, 'Read MCP resource', 'docs'],
    ['ReadMcpResourceDirTool', {server: 'docs', uri: 'docs://'}, 'Read MCP resource directory', 'docs'],
    ['SendMessage', {to: 'researcher', message: 'Status?'}, 'Send Message', 'researcher'],
    ['ScheduleWakeup', {delaySeconds: 600, reason: 'Watching CI', prompt: '/loop'}, 'Schedule Wakeup', '/loop'],
    ['PushNotification', {message: 'Done'}, 'Push Notification', 'Done'],
  ]],
  ['OpenCode', [
    ['shell', {command: 'make check', workdir: '/a'}, 'Run command', 'make check'],
    ['read', {path: '/a/b.go', offset: 1}, 'Read', '/a/b.go'],
    ['write', {path: '/a/b.go', content: 'x'}, 'Write', '/a/b.go'],
    ['edit', {path: '/a/b.go', oldString: 'x', newString: 'y'}, 'Edit', '/a/b.go'],
    ['patch', {patchText: '*** Begin Patch\n*** Update File: a.go'}, 'Patch', '*** Begin Patch *** Update File: a.go'],
    ['glob', {path: '/a', pattern: '**/*.go'}, 'Glob', '**/*.go'],
    ['grep', {path: '/a', pattern: 'func main', include: '*.go'}, 'Grep', 'func main'],
    ['webfetch', {url: 'https://go.dev', format: 'markdown'}, 'Web fetch', 'https://go.dev'],
    ['websearch', {query: 'go generics'}, 'Web search', 'go generics'],
    ['subagent', {agent: 'explore', description: 'Find the parser', prompt: 'Look for…'}, 'Run agent', 'Find the parser'],
    ['skill', {id: 'pdf'}, 'Skill', 'pdf'],
    ['todowrite', {todos: [{content: 'a', status: 'pending'}]}, 'Update todos', ''],
    ['question', {questions: [{question: 'Which?'}]}, 'Question', ''],
    ['execute', {code: 'await tools.read({path: "a"})'}, 'Run code', 'await tools.read({path: "a"})'],
    ['experimental_image_generate', {prompt: 'A red square'}, 'Generate image', 'A red square'],
    ['opencode_list_mcp_resources', {}, 'List MCP resources', ''],
    ['opencode_read_mcp_resource', {server: 'docs', uri: 'docs://a'}, 'Read MCP resource', 'docs'],
    ['opencode_artifact_create', {title: 'Report', entry: 'index.html'}, 'Opencode artifact create', 'Report'],
    ['opencode_artifact_read', {id: 'art_1'}, 'Opencode artifact read', 'art_1'],
    ['opencode_artifact_update', {id: 'art_1', title: 'Report'}, 'Opencode artifact update', 'art_1'],
    ['opencode_artifact_delete', {id: 'art_1'}, 'Opencode artifact delete', 'art_1'],
    ['opencode_schedule_create', {name: 'CI', prompt: 'Check CI'}, 'Opencode schedule create', 'Check CI'],
    ['opencode_schedule_list', {}, 'Opencode schedule list', ''],
    ['opencode_schedule_pause', {id: 's1'}, 'Opencode schedule pause', 's1'],
    ['opencode_schedule_resume', {id: 's1'}, 'Opencode schedule resume', 's1'],
    ['opencode_schedule_delete', {id: 's1'}, 'Opencode schedule delete', 's1'],
  ]],
  ['Pi', [
    ['bash', {command: 'make check', timeout: 60}, 'Run command', 'make check'],
    ['powershell', {command: 'dir'}, 'Run command', 'dir'],
    ['read', {path: '/a/b.go', offset: 1}, 'Read', '/a/b.go'],
    ['write', {path: '/a/b.go', content: 'x'}, 'Write', '/a/b.go'],
    ['edit', {path: '/a/b.go', edits: [{oldText: 'x', newText: 'y'}]}, 'Edit', '/a/b.go'],
    ['grep', {path: '/a', pattern: 'func main', glob: '*.go'}, 'Grep', 'func main'],
    ['find', {path: '/a', pattern: '*.go'}, 'Find', '*.go'],
    ['ls', {path: '/a'}, 'List files', '/a'],
    ['codemode', {code: 'await tools.read({path: "a"})'}, 'Run code', 'await tools.read({path: "a"})'],
    ['tool_search', {query: 'docs'}, 'Tool search', 'docs'],
  ]],
] as const)('%s tools have readable titles', (_harness, tools) => {
  for (const [name, input, label, summary] of tools) expect([name, ...toolTitle(tool({tool: name, input}))]).toEqual([name, label, summary]);
});

test('native tools without explicit labels have readable names', () => {
  expect(toolTitle(tool({tool: 'functions__some_tool', input: {path: 'a.ts'}}))).toEqual(['Functions some tool', 'a.ts']);
  expect(toolTitle(tool({tool: 'someTool'}))).toEqual(['Some Tool', '']);
});

test('a successful turn is its opener, one activity, and its response', () => {
  const result = rows([
    user('ask', 1000),
    working('w', 1500),
    {id: 'think', kind: 'reasoning', text: 'hm', at: 2000, seq: 0},
    call('read', 'read', 3000, {done: true}),
    text('progress', 4000),
    text('answer', 5000),
    idle('i', 9000, ['answer']),
  ]);
  expect(shape(result)).toEqual(['ask', ['activity', ['think', 'read', 'progress']], 'answer']);
  expect(result[1]).toMatchObject({key: 'activity:L1:w', start: 1000, until: 9000});
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
  const result = rows([user('ask', 1), working('w', 2), {id: 'think', kind: 'reasoning', text: 'hm', at: 3, seq: 0}, call('c', 'read', 4), text('progress', 5)]);
  expect(result.map(row => row.type)).toEqual(['user', 'reasoning', 'tool', 'text']);
});

test.each([['the user', undefined], ['another session', 'child']])('a message from %s during the turn stays in place between activities', (_source, from) => {
  const items = [user('ask', 1), working('w', 2), call('a', 'read', 3, {done: true}), user('more', 4, {from}), call('b', 'bash', 5, {done: true}), text('answer', 6), idle('i', 7, ['answer'])];
  const result = rows(items);
  expect(shape(result)).toEqual(['ask', ['activity', ['a']], 'more', ['activity', ['b']], 'answer']);
  expect(result[1]).toMatchObject({key: 'activity:L1:w', start: 1, until: 4});
  expect(result[3]).toMatchObject({key: 'activity:more', start: 4, until: 7});
});

test('an idle state outside a turn leaves the message that will begin the next one', () => {
  const [opener, activity, answer] = rows([user('ask', 1000), idle('start', 2000), working('w', 5000), call('c', 'read', 6000), text('answer', 9000), idle('i', 10000, ['answer'])]);
  expect(ids([opener])).toEqual(['ask']);
  expect(activity).toMatchObject({type: 'activity', start: 1000, until: 10000});
  expect(ids([answer])).toEqual(['answer']);
});

test('of several messages before the working state, the first begins the turn and the last its activity', () => {
  const result = rows([user('first', 1), user('second', 2), working('w', 3), call('c', 'read', 4), text('answer', 5), idle('i', 6, ['answer'])]);
  expect(shape(result)).toEqual(['first', 'second', ['activity', ['c']], 'answer']);
  expect(result[2]).toMatchObject({key: 'activity:second', start: 2, until: 6});
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
  expect(ids(rows(items))).toEqual(items.filter(item => item.kind !== 'state').map(item => item.id));
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

test('state items stay hidden', () => {
  const result = rows([user('ask', 1), working('w', 2), text('note', 3), call('a', 'read', 4), idle('i', 5)]);
  expect(ids(result)).toEqual(['ask', 'note', 'a']);
  expect(result[1].type).toBe('text');
});

test('nested items draw under their call, in order', () => {
  const [outer, next] = rows([
    call('parent', 'agent', 1000),
    call('one', 'read', 2000, {parent: 'parent', done: true}),
    {id: 'child-text', parent: 'parent', kind: 'text', text: 'Child finished', at: 6000, seq: 0},
    call('next', 'edit', 7000),
  ]);
  expect(toolAt([outer], 0).children.map(row => row.item.id)).toEqual(['one', 'child-text']);
  expect(ids([next])).toEqual(['next']);
});

test('nested items inside a finished turn stay under their call', () => {
  const [, activity] = rows([user('ask', 1), working('w', 2), call('p', 'agent', 3), call('n', 'read', 4, {parent: 'p'}), text('answer', 5), idle('i', 6, ['answer'])]);
  expect(toolAt(activityAt([activity], 0).rows, 0).children.map(row => row.item.id)).toEqual(['n']);
});

test('an item that arrives before its parent moves under it', () => {
  const child = text('c', 0, {parent: 'p'});
  expect(ids(rows([child]))).toEqual(['c']);
  const [parent] = rows([child, call('p', 'agent', 0)]);
  expect(toolAt([parent], 0).children.map(r => r.item.id)).toEqual(['c']);
});

test('message calls draw as messages', () => {
  const rowsOfCalls = rows([call('a', 'read', 1000), {id: 'm', kind: 'tool', server: 'hypercube', tool: 'send_message', input: {to: 'lead', text: 'Result'}, seq: 0}]);
  expect(rowsOfCalls.map(row => row.type)).toEqual(['tool', 'message']);
});

test('history omits open requests and keeps answered requests under their calls', () => {
  const [parent] = rows([
    call('a', 'agent', 1),
    attention('q', 2, {parent: 'a'}),
    attention('answered', 3, {parent: 'a', done: true, answer: 'yes'}),
    attention('raw', 4),
  ]);
  expect(toolAt([parent], 0).children.map(row => row.item.id)).toEqual(['answered']);
});

test('a run of reasoning titles draws as its latest title', () => {
  const reasoning = (id: string, text: string): TranscriptItem => ({id, kind: 'reasoning', text, seq: 0});
  expect(ids(rows([
    reasoning('r1', '**Checking events**'), reasoning('r2', '**Reviewing stop**'), call('c', 'shell', 1),
    reasoning('r3', '**Running tests**'), reasoning('r4', 'Full reasoning text.'), reasoning('r5', '**Next step**'),
  ]))).toEqual(['r2', 'c', 'r3', 'r4', 'r5']);
});
