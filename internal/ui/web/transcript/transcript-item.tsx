import {Alert, AlertDescription} from '../components/ui/alert';
// Draws one transcript row with the component for its kind, and draws text
// and error items, which need no component of their own.
import {memo} from 'react';
import type {Session} from '../sessions/types.ts';
import {CopyButton} from './copy-button.tsx';
import {ActivityItem} from './activity-item.tsx';
import {AttentionItem} from './attention-item.tsx';
import {Markdown} from './markdown.tsx';
import {MessageItem} from './message-item.tsx';
import {ReasoningItem} from './reasoning-item.tsx';
import {ToolCall} from './tool-items.tsx';
import {TranscriptImages} from './transcript-images.tsx';
import type {Row} from './transcript-tree.ts';
import type {ErrorItem, TextItem} from './types.ts';

const TextBlock = memo(({item, response}: {item: TextItem; response?: boolean}) => (
  <div className={`transcript-text group relative my-2 [--copy-bg:color-mix(in_oklab,var(--background),var(--card))] flex flex-col gap-3 ${response ? '[&_.prose]:text-foreground' : ''}`}>
    {item.text && <Markdown text={item.text} style="chat" />}
    <TranscriptImages images={item.images} />
    {item.text && <CopyButton text={item.text} label="Copy message" />}
  </div>
));

const ErrorAlert = memo(({item}: {item: ErrorItem}) => <Alert className="transcript-error border-0" variant="destructive"><AlertDescription className="whitespace-pre-wrap">{item.text ?? ''}</AlertDescription></Alert>);

// RowControls are what a row needs besides its data: the session, the keys
// the user opened in expanded, and setOpen(key, open) to change one.
export interface RowControls {
  session: Pick<Session, 'id'>;
  expanded: Record<string, true>;
  setOpen: (key: string, open: boolean) => void;
}

// TranscriptRow draws a row of transcriptRows.
export function TranscriptRow({row, session, expanded, setOpen, response}: RowControls & {row: Row; response?: boolean}) {
  switch (row.type) {
    case 'activity': return <ActivityItem row={row} session={session} expanded={expanded} setOpen={setOpen} />;
    case 'tool': return <ToolCall call={row} session={session} expanded={expanded} setOpen={setOpen} />;
    case 'message': return <MessageItem item={row.item} expanded={!!expanded[row.item.id]} setOpen={setOpen} />;
    case 'reasoning': return <ReasoningItem item={row.item} />;
    case 'user': return <MessageItem item={row.item} expanded={!!expanded[row.item.id]} setOpen={setOpen} />;
    case 'attention': return <AttentionItem item={row.item} session={session} />;
    case 'error': return <ErrorAlert item={row.item} />;
    case 'text': return <TextBlock item={row.item} response={response} />;
  }
}
