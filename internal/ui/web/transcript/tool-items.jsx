// Work runs contain thinking, messages, and tool calls as inline disclosures.
// Each call shows its status, input, output, and nested items, with images below.
import {clip, toolInput, toolTitle} from './transcript-tree.js';
import {Disclosure} from './disclosure.jsx';
import {Elapsed} from './elapsed.jsx';
import {MessageItem} from './message-item.jsx';
import {ReasoningItem} from './reasoning-item.jsx';
import {TranscriptImages} from './transcript-images.jsx';
import {TranscriptRow} from './transcript-item.jsx';
import './tool-items.css';

function Title({item}) {
  const [name, detail] = toolTitle(item);
  return <span class="tool-items-title"><span class="tool-items-name">{name}</span> {detail}</span>;
}

export function ToolRun({run, session, expanded, setOpen}) {
  const summary = <Elapsed live={!run.closed} present="Working" past="Worked" start={run.start} until={run.until} />;
  return (
    <Disclosure open={!!expanded[run.key]} onToggle={open => setOpen(run.key, open)} summary={summary}>
      <div class="tool-items-calls">
        {run.calls.map(call => {
          if (call.type === 'message') return <MessageItem key={call.item.id} item={call.item} expanded={!!expanded[call.item.id]} setOpen={setOpen} />;
          if (call.type === 'reasoning') return <ReasoningItem key={call.item.id} row={call} expanded={!!expanded[call.item.id]} setOpen={setOpen} />;
          return <ToolCall key={call.item.id} call={call} session={session} expanded={expanded} setOpen={setOpen} />;
        })}
      </div>
    </Disclosure>
  );
}

function ToolCall({call, session, expanded, setOpen}) {
  const {item, children} = call;
  const input = toolInput(item.input), output = clip(item.output ?? '');
  const status = !item.done ? <vscode-icon name="loading" spin role="img" title="Running" />
    : item.failed ? <vscode-icon name="error" class="tool-items-failed" role="img" title="Failed" />
    : <vscode-icon name="check" role="img" title="Done" />;
  const summary = <>
    <span class="tool-items-status">{status}</span>
    <Title item={item} />
  </>;
  return (
    <div class="tool-items-call">
      <Disclosure open={!!expanded[item.id]} onToggle={open => setOpen(item.id, open)} summary={summary}>
        {(input || output || children.length > 0) && (
          <div class="tool-items-body">
            {input && <pre class="tool-items-text tool-items-input">{input}</pre>}
            {output && <pre class="tool-items-text">{output}</pre>}
            {children.length > 0 && (
              <div class="tool-items-children">
                {children.map(row => <TranscriptRow key={row.key ?? row.item.id} row={row} session={session} expanded={expanded} setOpen={setOpen} />)}
              </div>
            )}
          </div>
        )}
      </Disclosure>
      <TranscriptImages images={item.images} />
    </div>
  );
}
