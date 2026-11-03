// User prompts are bubbles. Messages between sessions are disclosures with
// their sender or recipient and a first-line preview until expanded.
import {useStore} from '../store.js';
import {sessionOf} from '../sessions/selection.js';
import {Disclosure} from './disclosure.jsx';
import {TranscriptImages} from './transcript-images.jsx';
import './message-item.css';

export function MessageItem({item, expanded, setOpen}) {
  const [state] = useStore();
  const outgoing = item.kind === 'tool';
  if (!outgoing && !item.from) return (
    <div class="message-item message-item-user">
      <div class="message-item-bubble">
        <div class="message-item-text">{item.text ?? ''}</div>
        <TranscriptImages images={item.images} />
      </div>
    </div>
  );

  const input = item.input && typeof item.input === 'object' ? item.input : {};
  const heading = <>
    {outgoing ? 'To ' + (input.to ?? '') : 'From ' + (sessionOf(state, item.from)?.session.title || item.from)}
    {outgoing && !item.done && <vscode-icon name="loading" spin role="img" title="Running" />}
    {outgoing && item.failed && <vscode-icon name="error" class="message-item-failure" role="img" title="Failed" />}
  </>;
  const text = outgoing ? (typeof input.text === 'string' ? input.text : '') : item.text ?? '';
  return (
    <Disclosure class="message-item message-item-agent" open={expanded} onToggle={open => setOpen(item.id, open)} summary={heading} preview={text.split(/\r?\n/, 1)[0]}>
      {expanded && (
        <div class="message-item-body">
          <div class="message-item-text">{text}</div>
          {outgoing && item.failed && <div class="message-item-failure">{item.output ?? ''}</div>}
          {!outgoing && <TranscriptImages images={item.images} />}
        </div>
      )}
    </Disclosure>
  );
}
