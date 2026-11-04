import type {ReactNode} from 'react';
import {Card, CardContent} from '../components/ui/card';
import {BusyDot} from '../shared/busy-dot.tsx';
import {Icon} from '../shared/icon.tsx';
// User prompts are bubbles. Messages between sessions show their sender or
// recipient: a disclosure with a first-line preview until expanded, or, inline
// within a turn's activity, the whole message.
import {useStore} from '../store.ts';
import {sessionOf} from '../sessions/selection.ts';
import {CopyButton} from './copy-button.tsx';
import {Disclosure} from './disclosure.tsx';
import {Markdown} from './markdown.tsx';
import {TranscriptImages} from './transcript-images.tsx';
import type {ToolItem, UserItem} from './types.ts';

// sentMessage is the recipient and text of a send_message call's input.
function sentMessage(input: unknown): {to: string; text: string} {
  if (typeof input !== 'object' || input === null) return {to: '', text: ''};
  const to = 'to' in input ? input.to : undefined;
  const text = 'text' in input ? input.text : undefined;
  return {to: typeof to === 'string' ? to : '', text: typeof text === 'string' ? text : ''};
}

interface MessageItemProps {
  item: UserItem | ToolItem;
  expanded: boolean;
  setOpen: (key: string, open: boolean) => void;
  inline?: boolean;
}

function UserBubble({item}: {item: UserItem}) {
  return (
    <div className="message-item message-item-user my-3 flex justify-end">
      <div className="message-item-user-content group relative max-w-full">
        <Card className="message-item-bubble py-3"><CardContent className="px-4">
          <div className="message-item-text whitespace-pre-wrap break-words">{item.text ?? ''}</div>
          <TranscriptImages images={item.images} />
        </CardContent></Card>
        <CopyButton text={item.text ?? ''} label="Copy message" gutter />
      </div>
    </div>
  );
}

export function MessageItem({item, expanded, setOpen, inline}: MessageItemProps) {
  const [state] = useStore();
  let heading: ReactNode, text: string, details: ReactNode;
  if (item.kind === 'tool') {
    const sent = sentMessage(item.input);
    heading = <>
      {'To ' + sent.to}
      {!item.done && <BusyDot title="Running" />}
      {item.failed && <Icon name="error" className="message-item-failure text-destructive" role="img" title="Failed" />}
    </>;
    text = sent.text;
    details = item.failed && <div className="message-item-failure text-destructive">{item.output ?? ''}</div>;
  } else if (item.from) {
    heading = 'From ' + (sessionOf(state, item.from)?.session.title || item.from);
    text = item.text ?? '';
    details = <TranscriptImages images={item.images} />;
  } else {
    return <UserBubble item={item} />;
  }
  const body = (
    <div className="message-item-body space-y-2">
      <div className="message-item-text"><Markdown text={text} style="chat" /></div>
      {details}
    </div>
  );
  if (inline) return (
    <div className="message-item message-item-agent space-y-2">
      <div className="message-item-heading flex items-center gap-2 font-medium">{heading}</div>
      {body}
    </div>
  );
  return (
    <Disclosure className="message-item message-item-agent" open={expanded} onToggle={open => setOpen(item.id, open)} summary={heading} preview={text.split(/\r?\n/, 1)[0]}>
      {expanded && <div className="pl-8 pr-3">{body}</div>}
    </Disclosure>
  );
}
