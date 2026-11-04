import {Card, CardContent} from '../components/ui/card';
import {Icon} from '../shared/icon.jsx';
// A tool call shows its status, title, input, output, and nested items, with
// images below.
import {clip, toolInput, toolTitle} from './transcript-tree.js';
import {TranscriptImages} from './transcript-images.jsx';
import {TranscriptRow} from './transcript-item.jsx';

function Title({item}) {
  const [name, detail] = toolTitle(item);
  return <span className="tool-items-title min-w-0 break-words text-sm"><span className="tool-items-name font-medium">{name}</span> {detail}</span>;
}

export function ToolCall({call, session, expanded, setOpen}) {
  const {item, children} = call;
  const input = toolInput(item.input), output = clip(item.output ?? '');
  const status = !item.done ? <Icon name="loading" spin role="img" title="Running" />
    : item.failed ? <Icon name="error" className="tool-items-failed text-destructive" role="img" title="Failed" />
    : <Icon name="check" role="img" title="Done" />;
  return (
    <div className="tool-items-call space-y-2">
      <div className="tool-items-header flex items-start gap-2">
        <span className="tool-items-status shrink-0">{status}</span>
        <Title item={item} />
      </div>
      {(input || output || children.length > 0) && (
        <div className="tool-items-body space-y-2 pl-6">
          {input && <Card><CardContent><pre className="tool-items-text tool-items-input overflow-x-auto whitespace-pre-wrap break-words font-mono text-xs">{input}</pre></CardContent></Card>}
          {output && <Card><CardContent><pre className="tool-items-text overflow-x-auto whitespace-pre-wrap break-words font-mono text-xs">{output}</pre></CardContent></Card>}
          {children.length > 0 && (
            <div className="tool-items-children space-y-3 border-l pl-3">
              {children.map(row => <TranscriptRow key={row.item.id} row={row} session={session} expanded={expanded} setOpen={setOpen} />)}
            </div>
          )}
        </div>
      )}
      <TranscriptImages images={item.images} />
    </div>
  );
}
