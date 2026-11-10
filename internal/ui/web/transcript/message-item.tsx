// User prompts are bubbles. Messages between sessions show their sender or
// recipient, as a disclosure with a first-line preview until expanded.
import type {ReactNode} from 'react';
import {Card, CardContent} from '../components/ui/card';
import {sessionOf} from '../sessions/selection.ts';
import {tones} from '../shared/tones.ts';
import {useStore} from '../store.ts';
import {CopyButton} from './copy-button.tsx';
import {Disclosure} from './disclosure.tsx';
import {Markdown, MarkdownText} from './markdown.tsx';
import {ToolStatus} from './tool-items.tsx';
import {TranscriptImages} from './transcript-images.tsx';
import type {ToolItem, UserItem} from './types.ts';

// sentMessage is the recipient and text of a send_message call's input.
function sentMessage(input: unknown): {to: string; text: string} {
  if (typeof input !== 'object' || input === null) return {to: '', text: ''};
  const to = 'to' in input ? input.to : undefined;
  const text = 'text' in input ? input.text : undefined;
  return {to: typeof to === 'string' ? to : '', text: typeof text === 'string' ? text : ''};
}

function UserBubble({item}: {item: UserItem}) {
  return (
    <div className="my-3 flex justify-end">
      <div className="group relative max-w-[75%] [--copy-bg:var(--card)]">
        <Card className="py-3"><CardContent className="px-4">
          <div className="whitespace-pre-wrap break-words">{item.text ?? ''}</div>
          <TranscriptImages images={item.images} />
        </CardContent></Card>
        <CopyButton text={item.text ?? ''} label="Copy message" />
      </div>
    </div>
  );
}

export function MessageItem({item}: {item: UserItem | ToolItem}) {
  const [state] = useStore();
  let heading: string, text: string, details: ReactNode, status: ReactNode = null;
  if (item.kind === 'tool') {
    const sent = sentMessage(item.input);
    heading = 'To ' + sent.to;
    status = <ToolStatus item={item} />;
    text = sent.text;
    details = <>{item.failed && <div className={tones.error}>{item.output ?? ''}</div>}<TranscriptImages images={item.images} /></>;
  } else if (item.automatic) {
    heading = 'Automatic message';
    text = item.text ?? '';
    details = <TranscriptImages images={item.images} />;
  } else if (item.from) {
    heading = 'From ' + (sessionOf(state, item.from)?.session.title || item.from);
    text = item.text ?? '';
    details = <TranscriptImages images={item.images} />;
  } else {
    return <UserBubble item={item} />;
  }
  return (
    <Disclosure id={item.id}
      summary={<span className="flex min-w-0 max-w-1/2 shrink-0 items-center gap-2"><span className="truncate">{heading}</span>{status}</span>}
      preview={<span className="min-w-0 flex-1 truncate text-muted-foreground"><MarkdownText text={text.split(/\r?\n/, 1)[0]} linkText /></span>}>
      <div className="space-y-2 pb-1 pr-3">
        <Markdown text={text} style="chat" muted />
        {details}
      </div>
    </Disclosure>
  );
}
