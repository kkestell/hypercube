// A tool call starts collapsed, showing its status and title. Opening it
// reveals its input, output, nested items, and images.
import {Disclosure} from './disclosure.jsx';
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
  const open = !!expanded[item.id];
  const input = toolInput(item.input), output = clip(item.output ?? '');
  const status = !item.done ? <vscode-icon name="loading" spin role="img" title="Running" />
    : item.failed ? <vscode-icon name="error" class="tool-items-failed" role="img" title="Failed" />
    : <vscode-icon name="check" role="img" title="Done" />;
  return (
    <Disclosure
      class="tool-items-call"
      open={open}
      onToggle={open => setOpen(item.id, open)}
      summary={<span class="tool-items-header"><span class="tool-items-status">{status}</span><Title item={item} /></span>}
    >
      {open && (
        <div class="tool-items-body">
          {input && <pre class="tool-items-text tool-items-input">{input}</pre>}
          {output && <pre class="tool-items-text">{output}</pre>}
          {children.length > 0 && (
            <div class="tool-items-children">
              {children.map(row => <TranscriptRow key={row.item.id} row={row} session={session} expanded={expanded} setOpen={setOpen} />)}
            </div>
          )}
          <TranscriptImages images={item.images} />
        </div>
      )}
    </Disclosure>
  );
}
