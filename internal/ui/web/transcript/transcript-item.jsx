// Draws one transcript row with the component for its kind, and draws text
// and error items, which need no component of their own.
import {memo} from 'preact/compat';
import {AttentionItem} from './attention-item.jsx';
import {Markdown} from './markdown.jsx';
import {MessageItem} from './message-item.jsx';
import {ReasoningItem} from './reasoning-item.jsx';
import {ToolRun} from './tool-items.jsx';
import {TranscriptImages} from './transcript-images.jsx';
import './transcript-item.css';

const TextItem = memo(({item}) => (
  <div class="transcript-text">
    {item.text && <Markdown text={item.text} style="chat" />}
    <TranscriptImages images={item.images} />
  </div>
));

const ErrorItem = memo(({item}) => <div class="transcript-error">{item.text ?? ''}</div>);

// TranscriptRow draws a row of transcriptRows. expanded holds the keys the
// user opened, and setOpen(key, open) changes one.
export function TranscriptRow({row, session, expanded, setOpen}) {
  const {item} = row;
  switch (row.type) {
    case 'run': return <ToolRun run={row} session={session} expanded={expanded} setOpen={setOpen} />;
    case 'user': return <MessageItem item={item} expanded={!!expanded[item.id]} setOpen={setOpen} />;
    case 'reasoning': return <ReasoningItem row={row} expanded={!!expanded[item.id]} setOpen={setOpen} />;
    case 'attention': return <AttentionItem item={item} session={session} />;
    case 'error': return <ErrorItem item={item} />;
    default: return <TextItem item={item} />;
  }
}
