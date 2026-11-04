import {Alert, AlertDescription} from '../components/ui/alert';
// Draws one transcript row with the component for its kind, and draws text
// and error items, which need no component of their own.
import {memo} from 'react';
import {CopyButton} from './copy-button.jsx';
import {ActivityItem} from './activity-item.jsx';
import {AttentionItem} from './attention-item.jsx';
import {Markdown} from './markdown.jsx';
import {MessageItem} from './message-item.jsx';
import {ReasoningItem} from './reasoning-item.jsx';
import {ToolCall} from './tool-items.jsx';
import {TranscriptImages} from './transcript-images.jsx';

const TextItem = memo(({item}) => (
  <div className="transcript-text space-y-3">
    {item.text && <Markdown text={item.text} style="chat" />}
    <TranscriptImages images={item.images} />
    {item.text && <div className="copy-button-actions flex justify-end"><CopyButton text={item.text} label="Copy message" /></div>}
  </div>
));

const ErrorItem = memo(({item}) => <Alert className="transcript-error" variant="destructive"><AlertDescription className="whitespace-pre-wrap">{item.text ?? ''}</AlertDescription></Alert>);

// TranscriptRow draws a row of transcriptRows. expanded holds the keys the
// user opened, and setOpen(key, open) changes one.
export function TranscriptRow({row, session, expanded, setOpen}) {
  const {item} = row;
  switch (row.type) {
    case 'activity': return <ActivityItem row={row} session={session} expanded={expanded} setOpen={setOpen} />;
    case 'tool': return <ToolCall call={row} session={session} expanded={expanded} setOpen={setOpen} />;
    case 'message': return <MessageItem item={item} inline />;
    case 'reasoning': return <ReasoningItem item={item} />;
    case 'user': return <MessageItem item={item} inline={row.inline} expanded={!!expanded[item.id]} setOpen={setOpen} />;
    case 'attention': return <AttentionItem item={item} session={session} />;
    case 'error': return <ErrorItem item={item} />;
    default: return <TextItem item={item} />;
  }
}
