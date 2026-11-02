// A message bubble: the user's prompt, a message from another session (From),
// or a message this session sends (To), with a spinner until it is delivered
// and the failure if it was not. A long message between sessions shows its
// first lines until it is expanded.
import {useStore} from '../store.js';
import {sessionOf} from '../sessions/selection.js';
import {memo} from 'preact/compat';
import {isLong} from './transcript-tree.js';
import {TranscriptImages} from './transcript-images.jsx';
import './message-item.css';

function parts(item, state) {
  if (item.kind === 'user') return {heading: item.from ? 'From ' + (sessionOf(state, item.from)?.session.title || item.from) : '', text: item.text ?? '', between: !!item.from, own: true};
  const input = item.input && typeof item.input === 'object' ? item.input : {};
  return {heading: 'To ' + (input.to ?? ''), text: typeof input.text === 'string' ? input.text : '', between: true, own: false};
}

export const MessageItem = memo(({item, expanded, setOpen}) => {
  const [state] = useStore();
  const {heading, text, between, own} = parts(item, state);
  const long = between && isLong(text);
  return (
    <div class={own ? 'message-item message-item-user' : 'message-item'}>
      <div class="message-item-bubble">
        {heading && (
          <div class="message-item-heading">
            {heading}
            {!own && !item.done && <vscode-icon name="loading" spin role="img" title="Running" />}
          </div>
        )}
        <div class={long && !expanded ? 'message-item-text message-item-clamped' : 'message-item-text'}>{text}</div>
        {long && (
          <vscode-button secondary class="message-item-more" onClick={() => setOpen(item.id, !expanded)}>
            {expanded ? 'Show less' : 'Show more'}
          </vscode-button>
        )}
        {!own && item.failed && <div class="message-item-failure">{item.output ?? ''}</div>}
        {own && <TranscriptImages images={item.images} />}
      </div>
    </div>
  );
});
