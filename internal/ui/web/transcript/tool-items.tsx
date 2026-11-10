// A collapsed tool call shows its status and title. Opening it reveals its
// input, output, nested items, and images.
import type {ReactNode} from 'react';
import {BusyDot} from '../shared/busy-dot.tsx';
import {Icon} from '../shared/icon.tsx';
import {tones} from '../shared/tones.ts';
import {Disclosure} from './disclosure.tsx';
import {TranscriptDetails} from './transcript-details.tsx';
import {TranscriptImages} from './transcript-images.tsx';
import {TranscriptRow} from './transcript-item.tsx';
import type {ToolRow} from './transcript-tree.ts';
import type {ToolItem} from './types.ts';

// Tool input fields, in the order that best says what a call does. A search's
// pattern says more than the directory it searches.
const summaryKeys = ['command', 'file_path', 'notebook_path', 'pattern', 'path', 'filePath', 'query', 'url', 'skill', 'subject', 'description', 'prompt', 'code'];

// toolSummary is the one line that tells a tool call apart: the input's most
// telling text, shortened.
export function toolSummary(input: unknown): string {
  let text = '';
  if (typeof input === 'string') {
    text = input;
  } else if (typeof input === 'object' && input !== null) {
    const strings = new Map<string, string>();
    for (const [key, value] of Object.entries(input)) if (typeof value === 'string') strings.set(key, value);
    text = summaryKeys.map(key => strings.get(key)).find(value => value !== undefined) ?? strings.values().next().value ?? '';
  }
  text = text.replace(/\s+/g, ' ').trim();
  return text.length > 120 ? text.slice(0, 119) + '…' : text;
}

// Hypercube's own tools: what each call does and the input field that tells
// calls apart. send_message draws as a message instead.
const hypercubeTools = new Map<string, [name: string, field?: string]>([
  ['create_session', ['Create session', 'title']],
  ['end_session', ['End session', 'name']],
  ['list_sessions', ['List sessions']],
  ['list_models', ['List models', 'harness']],
  ['read_session', ['Read session', 'to']],
  ['set_title', ['Set title', 'title']],
  ['open_file', ['Open file', 'path']],
]);

// Native calls have no MCP server. These labels replace names that do not read
// as plain phrases, matched without case so that Claude Code's Bash and Pi's
// bash read alike. Other native tools are titled from their names.
const nativeTools = new Map([
  ['bash', 'Run command'],
  ['shell', 'Run command'],
  ['powershell', 'Run command'],
  ['agent', 'Run agent'],
  ['task', 'Run agent'],
  ['subagent', 'Run agent'],
  ['codemode', 'Run code'],
  ['execute', 'Run code'],
  ['ls', 'List files'],
  ['notebookedit', 'Edit notebook'],
  ['webfetch', 'Web fetch'],
  ['websearch', 'Web search'],
  ['todowrite', 'Update todos'],
  ['experimental_image_generate', 'Generate image'],
  ['listmcpresourcestool', 'List MCP resources'],
  ['opencode_list_mcp_resources', 'List MCP resources'],
  ['readmcpresourcetool', 'Read MCP resource'],
  ['opencode_read_mcp_resource', 'Read MCP resource'],
  ['readmcpresourcedirtool', 'Read MCP resource directory'],
]);

// toolTitle is a tool call's name and summary.
export function toolTitle({server, tool, input}: ToolItem): [name: string, summary: string] {
  const named = server === 'hypercube' && tool !== undefined ? hypercubeTools.get(tool) : undefined;
  if (named !== undefined) {
    const [name, field] = named;
    const detail = field && typeof input === 'object' && input !== null ? Reflect.get(input, field) : '';
    return [name, toolSummary(typeof detail === 'string' ? detail : '')];
  }
  if (server) return [server + ' - ' + tool, toolSummary(input)];
  const words = (tool ?? 'tool').replace(/([a-z])([A-Z])/g, '$1 $2').replace(/[_-]+/g, ' ');
  const name = nativeTools.get(tool?.toLowerCase() ?? '') ?? words.charAt(0).toUpperCase() + words.slice(1);
  return [name, toolSummary(input)];
}

// A tool's input and output show this many characters.
const shownLimit = 20000;
const clip = (text: string): string => text.length > shownLimit ? text.slice(0, shownLimit) + '\n… (' + (text.length - shownLimit) + ' more characters)' : text;

const toolInput = (input: unknown): string => input === undefined || input === null ? '' : clip(typeof input === 'string' ? input : JSON.stringify(input, null, 2));

// ToolStatus shows whether a call is running, failed, or done.
export function ToolStatus({item}: {item: ToolItem}) {
  if (!item.done) return <BusyDot title="Running" />;
  return item.failed ? <Icon name="error" className={tones.error} role="img" title="Failed" /> : <Icon name="check" role="img" title="Done" />;
}

function Title({item}: {item: ToolItem}) {
  const [name, detail] = toolTitle(item);
  return <span className="min-w-0 truncate text-sm" title={[name, detail].filter(Boolean).join(' ')}>{name}{detail && <span className="ml-2 font-mono text-xs text-muted-foreground">{detail}</span>}</span>;
}

const pre = (content: string) => <pre className="overflow-x-auto whitespace-pre-wrap break-words font-mono text-xs">{content}</pre>;

export function ToolCall({call}: {call: ToolRow}) {
  const {item, children} = call;
  const input = toolInput(item.input), output = clip(item.output ?? '');
  const sections: [string, ReactNode][] = [];
  if (input) sections.push(['Arguments', pre(input)]);
  if (output) sections.push(['Output', pre(output)]);
  return (
    <Disclosure id={item.id} className="tool-items-call [:is(.tool-items-call,.attention-item)[data-state=closed]+&]:-mt-3"
      marker={<ToolStatus item={item} />} summary={<Title item={item} />}>
      <div className="mt-2 space-y-2">
        {!sections.length && !children.length && !item.images?.length && (
          <p className="text-xs text-muted-foreground">{item.done ? 'No details recorded.' : 'Waiting for tool details…'}</p>
        )}
        {sections.length > 0 && <TranscriptDetails sections={sections} />}
        {children.length > 0 && (
          <div className="flex flex-col gap-3">
            {children.map(row => <TranscriptRow key={row.item.id} row={row} />)}
          </div>
        )}
        <TranscriptImages images={item.images} />
      </div>
    </Disclosure>
  );
}
