import {Card, CardContent} from '../components/ui/card';
import {Icon} from '../shared/icon.jsx';
// User prompts are bubbles. Messages between sessions show their sender or
// recipient: a disclosure with a first-line preview until expanded, or, inline
// within a turn's activity, the whole message.
import {useStore} from '../store.js';
import {sessionOf} from '../sessions/selection.js';
import {CopyButton} from './copy-button.jsx';
import {Disclosure} from './disclosure.jsx';
import {TranscriptImages} from './transcript-images.jsx';

export function MessageItem({item, expanded, setOpen, inline}) {
  const [state] = useStore();
  const outgoing = item.kind === 'tool';
  if (!outgoing && !item.from) return (
    <div className="message-item message-item-user my-3 flex justify-end">
      <div className="message-item-user-content group relative max-w-full pr-8">
        <Card className="message-item-bubble py-2"><CardContent className="px-3">
          <div className="message-item-text whitespace-pre-wrap break-words">{item.text ?? ''}</div>
          <TranscriptImages images={item.images} />
        </CardContent></Card>
        <CopyButton text={item.text ?? ''} label="Copy message" />
      </div>
    </div>
  );

  const input = item.input && typeof item.input === 'object' ? item.input : {};
  const heading = <>
    {outgoing ? 'To ' + (input.to ?? '') : 'From ' + (sessionOf(state, item.from)?.session.title || item.from)}
    {outgoing && !item.done && <Icon name="loading" spin role="img" title="Running" />}
    {outgoing && item.failed && <Icon name="error" className="message-item-failure text-destructive" role="img" title="Failed" />}
  </>;
  const text = outgoing ? (typeof input.text === 'string' ? input.text : '') : item.text ?? '';
  const body = (
    <div className="message-item-body space-y-2 p-3">
      <div className="message-item-text whitespace-pre-wrap break-words">{text}</div>
      {outgoing && item.failed && <div className="message-item-failure text-destructive">{item.output ?? ''}</div>}
      {!outgoing && <TranscriptImages images={item.images} />}
    </div>
  );
  if (inline) return (
    <div className="message-item message-item-agent space-y-2">
      <div className="message-item-heading flex items-center gap-2 font-medium">{heading}</div>
      {body}
    </div>
  );
  return (
    <Disclosure className="message-item message-item-agent space-y-2" open={expanded} onToggle={open => setOpen(item.id, open)} summary={heading} preview={text.split(/\r?\n/, 1)[0]}>
      {expanded && body}
    </Disclosure>
  );
}
