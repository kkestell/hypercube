// The box below a transcript for the user's next message: its text, attached
// images, attach and send buttons, and its profile and usage. A new session
// chooses its profile and worktree there; after the first prompt the profile
// shows as text, and a worktree session shows its branch beside it. The text
// takes focus on mount.
import {useEffect, useLayoutEffect, useRef, useState} from 'preact/hooks';
import {imageSrc, imageTypes} from '../images.js';
import {sendPrompt} from '../sessions/session-actions.js';
import {AgentStatus} from '../shared/agent-status.jsx';
import {submitsOnKey} from '../shared/submit-key.js';
import {useStore} from '../store.js';
import {attachImages, canSend, draftOf, removeImage, typed, unsentKey} from './drafts.js';
import {ProfileSelect} from './profile-select.jsx';
import {Usage} from './usage.jsx';
import {SessionBranch} from './session-branch.jsx';
import {WorktreeToggle} from './worktree-toggle.jsx';
import './composer.css';

export function Composer({session}) {
  const [state, store] = useStore();
  const draft = draftOf(state, session.id);
  const field = useRef(null), picker = useRef(null);
  const [historyIndex, setHistoryIndex] = useState(null);
  const [mention, setMention] = useState(null);
  const [files, setFiles] = useState([]);
  const caret = useRef(null);
  useLayoutEffect(() => { if (caret.current !== null) { field.current.setSelectionRange(caret.current, caret.current); caret.current = null; } });
  const transcript = state.session?.id === session.id ? state.session.transcript : null;
  const history = transcript ? transcript.order.map(id => transcript.items[id]).filter(item => item.kind === 'user' && !item.from && item.text).map(item => item.text) : [];
  const text = historyIndex === null ? draft.text : history[historyIndex] ?? draft.text;
  const choices = mention ? files.filter(path => {
    let i = 0;
    for (const c of path.toLowerCase()) if (c === mention.query[i]) i++;
    return i === mention.query.length;
  }).slice(0, 30) : [];

  useEffect(() => { setHistoryIndex(null); setMention(null); setFiles([]); }, [session.id]);
  useEffect(() => {
    if (!mention || (session.id === unsentKey && !state.unsent?.workspace)) return;
    const controller = new AbortController();
    fetch('/mention-files?' + new URLSearchParams(session.id === unsentKey ? {workspace: state.unsent.workspace} : {session: session.id}), {signal: controller.signal})
      .then(response => response.ok ? response.json() : []).then(setFiles).catch(() => {});
    return () => controller.abort();
  }, [session.id, state.unsent?.workspace, !!mention]);

  const input = e => {
    const value = e.currentTarget.value;
    setHistoryIndex(null);
    store.update(s => typed(s, session.id, value));
    const before = value.slice(0, e.currentTarget.selectionStart);
    const match = before.match(/(?:^|\s)@([^\s@]*)$/);
    setMention(match ? {start: before.length - match[1].length - 1, query: match[1].toLowerCase(), selected: 0} : null);
  };
  const choose = path => {
    const end = field.current.selectionStart;
    const value = text.slice(0, mention.start) + '@' + path + text.slice(end);
    caret.current = mention.start + path.length + 1;
    store.update(s => typed(s, session.id, value));
    setHistoryIndex(null);
    setMention(null);
    field.current.focus();
  };

  // Preserve keyboard focus in the file tab strip when a session mounts.
  useEffect(() => {
    if (!document.activeElement?.closest('.editor-tabs')) field.current.focus();
  }, []);

  const send = () => {
    if (historyIndex !== null) store.update(s => typed(s, session.id, text));
    setHistoryIndex(null);
    setMention(null);
    sendPrompt(store, session);
  };
  const key = e => {
    if (e.isComposing) return;
    if (mention) {
      if (e.key === 'Escape') { e.preventDefault(); setMention(null); return; }
      if (choices.length && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
        e.preventDefault();
        const step = e.key === 'ArrowDown' ? 1 : -1;
        setMention({...mention, selected: (mention.selected + step + choices.length) % choices.length});
        return;
      }
      if (choices.length && (e.key === 'Enter' || e.key === 'Tab')) {
        e.preventDefault(); choose(choices[mention.selected % choices.length]); return;
      }
    }
    if (!e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey && e.currentTarget.selectionStart === e.currentTarget.selectionEnd) {
      const pos = e.currentTarget.selectionStart;
      if (e.key === 'ArrowUp' && !text.slice(0, pos).includes('\n') && history.length) {
        e.preventDefault(); setHistoryIndex(historyIndex === null ? history.length - 1 : Math.max(0, historyIndex - 1)); return;
      }
      if (e.key === 'ArrowDown' && historyIndex !== null && !text.slice(pos).includes('\n')) {
        e.preventDefault(); setHistoryIndex(historyIndex + 1 >= history.length ? null : historyIndex + 1); return;
      }
    }
    if (!submitsOnKey(e)) return;
    e.preventDefault();
    send();
  };
  const picked = e => {
    const files = [...e.currentTarget.files];
    e.currentTarget.value = '';
    attachImages(store, session.id, files);
  };

  return (
    <form class="composer" data-session={session.id} onSubmit={e => { e.preventDefault(); send(); }}>
      {draft.images.length > 0 && (
        <div class="composer-images">
          {draft.images.map(image => (
            <div class="composer-image">
              <img src={imageSrc(image)} alt="Attached image" />
              <vscode-button icon-only secondary icon="close" class="composer-image-remove" title="Remove Image" aria-label="Remove Image"
                onClick={() => store.update(s => removeImage(s, session.id, image))} />
            </div>
          ))}
        </div>
      )}
      {mention && <div class="composer-mentions" role="listbox" id="composer-mentions" aria-label="Files">
        {choices.map((path, i) => <div role="option" id={'composer-file-' + i} aria-selected={i === mention.selected}
          onMouseDown={e => { e.preventDefault(); choose(path); }}>{path}</div>)}
      </div>}
      <textarea ref={field} rows={1} aria-label="Message" class="composer-text" value={text}
        role="combobox" aria-expanded={!!mention} aria-autocomplete="list" aria-controls={mention ? 'composer-mentions' : undefined}
        aria-activedescendant={mention && choices.length ? 'composer-file-' + mention.selected : undefined}
        onInput={input} onKeyDown={key} />
      <input ref={picker} type="file" accept={imageTypes.join(',')} multiple hidden onChange={picked} />
      <div class="composer-bar">
        <vscode-button icon-only secondary icon="attach" title="Attach Image" aria-label="Attach Image" onClick={() => picker.current.click()} />
        <div class="composer-details">
          <AgentStatus session={session} />
          <ProfileSelect session={session} />
          {session.id === unsentKey ? <WorktreeToggle /> : <SessionBranch session={session} />}
          <Usage usage={session.usage} />
        </div>
        <vscode-button icon-only secondary icon="send" title="Send" aria-label="Send" disabled={!canSend({...state, drafts: {...state.drafts, [session.id]: {...draft, text}}}, session)} onClick={send} />
      </div>
    </form>
  );
}
