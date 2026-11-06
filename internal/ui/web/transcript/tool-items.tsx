import {Card, CardContent} from '../components/ui/card';
import {BusyDot} from '../shared/busy-dot.tsx';
import {Icon} from '../shared/icon.tsx';
// A collapsed tool call shows its status and title. Opening it reveals its
// input, output, nested items, and images.
import {Disclosure} from './disclosure.tsx';
import {clip, toolInput, toolTitle, type ToolRow} from './transcript-tree.ts';
import {TranscriptImages} from './transcript-images.tsx';
import {TranscriptRow, type RowControls} from './transcript-item.tsx';
import type {ToolItem} from './types.ts';

function Title({item}: {item: ToolItem}) {
  const [name, detail] = toolTitle(item);
  return <span className="tool-items-title min-w-0 truncate text-sm" title={[name, detail].filter(Boolean).join(' ')}><span className="tool-items-name">{name}</span> {detail && <span className="tool-items-detail ml-1 font-mono text-xs text-muted-foreground">{detail}</span>}</span>;
}

export function ToolCall({call, session, expanded, setOpen}: RowControls & {call: ToolRow}) {
  const {item, children} = call;
  const open = !!expanded[item.id];
  const input = toolInput(item.input), output = clip(item.output ?? '');
  const status = !item.done ? <BusyDot title="Running" />
    : item.failed ? <Icon name="error" className="tool-items-failed text-destructive" role="img" title="Failed" />
    : <Icon name="check" role="img" title="Done" />;
  return (
    <Disclosure className="tool-items-call [.tool-items-call[data-state=closed]+&]:-mt-3" open={open} onToggle={open => setOpen(item.id, open)}
      marker={<span className="tool-items-status">{status}</span>} summary={<Title item={item} />}>
      {open && (
        <div className="tool-items-body mt-2 space-y-2">
          {(input || output) && <Card className="gap-0 overflow-hidden border-0 py-0">
            {input && <CardContent className="space-y-2 px-3 py-2">
              <h4 className="text-xs font-medium text-muted-foreground">Arguments</h4>
              <pre className="tool-items-text tool-items-input overflow-x-auto whitespace-pre-wrap break-words font-mono text-xs">{input}</pre>
            </CardContent>}
            {output && <CardContent className={`space-y-2 px-3 py-2 ${input ? 'border-t' : ''}`}>
              <h4 className="text-xs font-medium text-muted-foreground">Output</h4>
              <pre className="tool-items-text overflow-x-auto whitespace-pre-wrap break-words font-mono text-xs">{output}</pre>
            </CardContent>}
          </Card>}
          {children.length > 0 && (
            <div className="tool-items-children flex flex-col gap-3">
              {children.map(row => <TranscriptRow key={row.item.id} row={row} session={session} expanded={expanded} setOpen={setOpen} />)}
            </div>
          )}
          <TranscriptImages images={item.images} />
        </div>
      )}
    </Disclosure>
  );
}
