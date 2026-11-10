import {expect, test} from 'bun:test';
import {toolSummary, toolTitle} from './tool-items.tsx';
import type {ToolItem} from './types.ts';

const tool = (extra: Partial<ToolItem>): ToolItem => ({id: 'tool', kind: 'tool', ...extra});

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
