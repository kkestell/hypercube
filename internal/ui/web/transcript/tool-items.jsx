// Tool runs and tool calls, as the chat's inline disclosures. A run shows
// Working… and its latest call on separate lines while open, then how long it
// worked; expanding it lists its calls. Each call shows its own spinner,
// input, output, and nested items, with its images below.
import {clip, toolInput, toolTitle} from './transcript-tree.js';
import {MessageItem} from './message-item.jsx';
import {TranscriptImages} from './transcript-images.jsx';
import {TranscriptRow} from './transcript-item.jsx';
import './tool-items.css';

const Spinner = () => <vscode-icon name="loading" spin role="img" title="Running" />;

function Disclosure({open, onToggle, summary, children}) {
  return (
    <details class="tool-items-disclosure" open={open} onToggle={e => onToggle(e.currentTarget.open)}>
      <summary class="tool-items-summary">
        <vscode-icon name="chevron-right" class="tool-items-chevron" />
        {summary}
      </summary>
      {children}
    </details>
  );
}

function Title({item}) {
  const [name, detail] = toolTitle(item);
  return <span class="tool-items-title"><span class="tool-items-name">{name}</span> {detail}</span>;
}

export function ToolRun({run, session, expanded, setOpen}) {
  const latest = run.calls[run.calls.length - 1].item;
  const summary = run.closed
    ? <span class="tool-items-title">{run.duration}</span>
    : <span class="tool-items-working"><span>Working…</span><Title item={latest} /></span>;
  return (
    <Disclosure open={!!expanded[run.key]} onToggle={open => setOpen(run.key, open)} summary={summary}>
      <div class="tool-items-calls">
        {run.calls.map(call => call.type === 'message'
          ? <MessageItem key={call.item.id} item={call.item} expanded={!!expanded[call.item.id]} setOpen={setOpen} />
          : <ToolCall key={call.item.id} call={call} session={session} expanded={expanded} setOpen={setOpen} />)}
      </div>
    </Disclosure>
  );
}

function ToolCall({call, session, expanded, setOpen}) {
  const {item, children} = call;
  const input = toolInput(item.input), output = clip(item.output ?? '');
  const summary = <>
    {!item.done && <Spinner />}
    {item.done && item.failed && <vscode-icon name="error" class="tool-items-failed" role="img" title="Failed" />}
    <Title item={item} />
  </>;
  return (
    <div class="tool-items-call">
      <Disclosure open={!!expanded[item.id]} onToggle={open => setOpen(item.id, open)} summary={summary}>
        {input && <pre class="tool-items-text tool-items-input">{input}</pre>}
        {output && <pre class="tool-items-text">{output}</pre>}
        {children.length > 0 && (
          <div class="tool-items-children">
            {children.map(row => <TranscriptRow key={row.key ?? row.item.id} row={row} session={session} expanded={expanded} setOpen={setOpen} />)}
          </div>
        )}
      </Disclosure>
      <TranscriptImages images={item.images} />
    </div>
  );
}
