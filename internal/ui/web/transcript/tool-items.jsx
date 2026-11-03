// A tool call shows its status, title, input, output, and nested items, with
// images below.
import {clip, toolInput, toolTitle} from './transcript-tree.js';
import {TranscriptImages} from './transcript-images.jsx';
import {TranscriptRow} from './transcript-item.jsx';
import './tool-items.css';

function Title({item}) {
  const [name, detail] = toolTitle(item);
  return <span class="tool-items-title"><span class="tool-items-name">{name}</span> {detail}</span>;
}

export function ToolCall({call, session, expanded, setOpen}) {
  const {item, children} = call;
  const input = toolInput(item.input), output = clip(item.output ?? '');
  const status = !item.done ? <vscode-icon name="loading" spin role="img" title="Running" />
    : item.failed ? <vscode-icon name="error" class="tool-items-failed" role="img" title="Failed" />
    : <vscode-icon name="check" role="img" title="Done" />;
  return (
    <div class="tool-items-call">
      <div class="tool-items-header">
        <span class="tool-items-status">{status}</span>
        <Title item={item} />
      </div>
      {(input || output || children.length > 0) && (
        <div class="tool-items-body">
          {input && <pre class="tool-items-text tool-items-input">{input}</pre>}
          {output && <pre class="tool-items-text">{output}</pre>}
          {children.length > 0 && (
            <div class="tool-items-children">
              {children.map(row => <TranscriptRow key={row.item.id} row={row} session={session} expanded={expanded} setOpen={setOpen} />)}
            </div>
          )}
        </div>
      )}
      <TranscriptImages images={item.images} />
    </div>
  );
}
