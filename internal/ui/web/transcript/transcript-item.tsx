import {Alert, AlertDescription} from '../components/ui/alert';
// Draws one transcript row with the component for its kind, and draws text
// and error items, which need no component of their own.
import {memo} from 'react';
import type {Session} from '../sessions/types.ts';
import {CopyButton} from './copy-button.tsx';
import {ActivityItem} from './activity-item.tsx';
import {AnsweredItem} from './attention-item.tsx';
import {ProposalItem} from './proposal-item.tsx';
import {Markdown} from './markdown.tsx';
import {MessageItem} from './message-item.tsx';
import {ReasoningItem} from './reasoning-item.tsx';
import {ToolCall} from './tool-items.tsx';
import {TranscriptImages} from './transcript-images.tsx';
import type {Row} from './transcript-tree.ts';
import type {ErrorItem, TextItem} from './types.ts';

const TextBlock = memo(({item}: {item: TextItem}) => (
  <div className="transcript-text group relative my-2 [--copy-bg:color-mix(in_oklab,var(--background),var(--card))] flex flex-col gap-3">
    {item.text && <Markdown text={item.text} style="chat" />}
    <TranscriptImages images={item.images} />
    {item.text && <CopyButton text={item.text} label="Copy message" />}
  </div>
));

const ErrorAlert = memo(({item}: {item: ErrorItem}) => <Alert className="transcript-error border-0" variant="destructive"><AlertDescription className="whitespace-pre-wrap">{item.text ?? ''}</AlertDescription></Alert>);

// RowControls are what a row needs besides its data: the session, the keys
// the user opened in expanded, and setOpen(key, open) to change one.
export interface RowControls {
  session: Pick<Session, 'id' | 'ended'>;
  expanded: Record<string, true>;
  setOpen: (key: string, open: boolean) => void;
}

// TranscriptRow draws a row of transcriptRows. A row redraws only when its
// row or controls change, so typing and other page changes leave a long
// transcript alone.
export const TranscriptRow = memo(({row, session, expanded, setOpen}: RowControls & {row: Row}) => {
  switch (row.type) {
    case 'activity': return <ActivityItem row={row} session={session} expanded={expanded} setOpen={setOpen} />;
    case 'tool': return <ToolCall call={row} session={session} expanded={expanded} setOpen={setOpen} />;
    case 'message': return <MessageItem item={row.item} expanded={!!expanded[row.item.id]} setOpen={setOpen} />;
    case 'reasoning': return <ReasoningItem item={row.item} />;
    case 'user': return <MessageItem item={row.item} expanded={!!expanded[row.item.id]} setOpen={setOpen} />;
    case 'attention': return <AnsweredItem item={row.item} expanded={!!expanded[row.item.id]} setOpen={setOpen} />;
    case 'proposal': return <ProposalItem item={row.item} session={session} expanded={!!expanded[row.item.id]} setOpen={setOpen} />;
    case 'error': return <ErrorAlert item={row.item} />;
    case 'text': return <TextBlock item={row.item} />;
  }
});
