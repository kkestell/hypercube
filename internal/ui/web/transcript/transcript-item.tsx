// Draws one transcript row with the component for its kind, and draws text,
// reasoning, and error items, which need no component of their own.
import {memo} from 'react';
import {Alert, AlertDescription} from '../components/ui/alert';
import {ActivityItem} from './activity-item.tsx';
import {AnsweredItem} from './attention-item.tsx';
import {CopyButton} from './copy-button.tsx';
import {Markdown} from './markdown.tsx';
import {MessageItem} from './message-item.tsx';
import {AnsweredProposal} from './proposal-item.tsx';
import {ToolCall} from './tool-items.tsx';
import {TranscriptImages} from './transcript-images.tsx';
import type {Row} from './transcript-tree.ts';
import type {TextItem} from './types.ts';

const TextBlock = memo(({item}: {item: TextItem}) => (
  <div className="group relative my-2 flex flex-col gap-3 [--copy-bg:var(--color-tab)]">
    {item.text && <Markdown text={item.text} style="chat" />}
    <TranscriptImages images={item.images} />
    {item.text && <CopyButton text={item.text} label="Copy message" />}
  </div>
));

// TranscriptRow draws a row of transcriptRows. A row redraws only when it
// changes, so a long transcript is drawn again only where it changed. The
// history holds only answered requests and proposals; open ones are pinned
// below it.
export const TranscriptRow = memo(({row}: {row: Row}) => {
  switch (row.type) {
    case 'activity': return <ActivityItem row={row} />;
    case 'tool': return <ToolCall call={row} />;
    case 'message': return <MessageItem item={row.item} />;
    case 'user': return <MessageItem item={row.item} />;
    case 'reasoning': return row.item.text ? <Markdown text={row.item.text} style="chat" muted /> : null;
    case 'attention': return <AnsweredItem item={row.item} />;
    case 'proposal': return <AnsweredProposal item={row.item} />;
    case 'error': return <Alert variant="destructive"><AlertDescription className="whitespace-pre-wrap">{row.item.text ?? ''}</AlertDescription></Alert>;
    case 'text': return <TextBlock item={row.item} />;
  }
});
