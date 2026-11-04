// The box below a transcript for the user's next message: its text, attached
// images, attach and send buttons, and its profile and usage. A new session
// chooses its profile and worktree there; after the first prompt the profile
// shows as text, and a worktree session shows its branch beside it. The text
// takes focus on mount.
import {useEffect, useLayoutEffect, useRef, useState} from 'preact/hooks';
import {mentionFiles} from '../api.js';
import {imageSrc, imageTypes} from '../images.js';
import {sendPrompt} from '../sessions/session-actions.js';
import {AgentStatus} from '../shared/agent-status.jsx';
import {submitsOnKey} from '../shared/submit-key.js';
import {useStore} from '../store.js';
import {attachImages, canSend, draftOf, removeImage, typed, unsentKey} from './drafts.js';
import {promptHistory} from './history.js';
import {insertMention, mentionAt, mentionChoices, prepareFiles} from './mention.js';
import {ProfileSelect} from './profile-select.jsx';
import {Usage} from './usage.jsx';
import {SessionBranch} from './session-branch.jsx';
import {WorktreeToggle} from './worktree-toggle.jsx';
import './composer.css';

export function Composer({session}) {
  const [state, store] = useStore();
  const draft = draftOf(state, session.id);
  const field = useRef(null), picker = useRef(null);
  const [history, setHistory] = useState(null);
  const [mention, setMention] = useState(null);
  const [selected, setSelected] = useState(0);
  const [files, setFiles] = useState([]);
  const [wantFiles, setWantFiles] = useState(false);
  const caret = useRef(null), activeOption = useRef(null), suppressSelect = useRef(false);
  const text = history === null ? draft.text : history.edits[history.index];
  const choices = mention ? mentionChoices(files, mention.query) : [];
  const active = choices.length ? Math.min(selected, choices.length - 1) : 0;
  const workspace = session.id === unsentKey ? state.unsent?.workspace : null;

  useLayoutEffect(() => {
    if (caret.current !== null) {
      field.current.setSelectionRange(caret.current, caret.current);
      caret.current = null;
    }
  });
  useEffect(() => { activeOption.current?.scrollIntoView?.({block: 'nearest'}); }, [active, choices[active]]);
  useEffect(() => { if (mention) setWantFiles(true); }, [!!mention]);
  useEffect(() => {
    if (!wantFiles || (session.id === unsentKey && !workspace)) return;
    const controller = new AbortController();
    setFiles([]);
    mentionFiles(session.id === unsentKey ? {workspace} : {session: session.id}, controller.signal)
      .then(paths => { if (!controller.signal.aborted) setFiles(prepareFiles(paths)); }).catch(() => {});
    return () => controller.abort();
  }, [session.id, workspace, wantFiles]);

  const updateMention = target => {
    const next = mentionAt(target.value, target.selectionStart, target.selectionEnd);
    setMention(next);
    setSelected(0);
  };
  const changeText = value => {
    if (history === null) store.update(s => typed(s, session.id, value));
    else setHistory({...history, edits: {...history.edits, [history.index]: value}});
  };
  const input = e => { suppressSelect.current = false; changeText(e.currentTarget.value); updateMention(e.currentTarget); };
  const choose = path => {
    const current = mentionAt(field.current.value, field.current.selectionStart, field.current.selectionEnd);
    if (!current) { setMention(null); return; }
    suppressSelect.current = true;
    changeText(insertMention(text, current, path));
    caret.current = current.start + path.length + 1;
    setMention(null);
    field.current.focus();
  };

  // Preserve keyboard focus in the file tab strip when a session mounts.
  useEffect(() => {
    if (!document.activeElement?.closest('.editor-tabs')) field.current.focus();
  }, []);

  const send = () => {
    if (!canSend(state, session, text)) return;
    if (history !== null) store.update(s => typed(s, session.id, text));
    setHistory(null);
    setMention(null);
    sendPrompt(store, session);
  };
  const key = e => {
    if (e.isComposing) return;
    const plain = !e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey;
    if (plain && mention && choices.length) {
      if (e.key === 'Escape') { e.preventDefault(); suppressSelect.current = true; setMention(null); return; }
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        setSelected((active + (e.key === 'ArrowDown' ? 1 : -1) + choices.length) % choices.length);
        return;
      }
      if (e.key === 'Enter' || e.key === 'Tab') { e.preventDefault(); choose(choices[active]); return; }
    }
    if (plain && e.currentTarget.selectionStart === e.currentTarget.selectionEnd) {
      const pos = e.currentTarget.selectionStart;
      const up = e.key === 'ArrowUp' && !text.slice(0, pos).includes('\n');
      const down = e.key === 'ArrowDown' && history !== null && !text.slice(pos).includes('\n');
      if (up || down) {
        const transcript = state.session?.id === session.id ? state.session.transcript : null;
        const prompts = promptHistory(transcript);
        if (prompts.length) {
          e.preventDefault();
          const index = up ? (history === null ? prompts.length - 1 : Math.max(0, history.index - 1)) : history.index + 1;
          suppressSelect.current = true;
          setMention(null);
          if (index >= prompts.length) setHistory(null);
          else setHistory({index, edits: {...Object.fromEntries(prompts.map((value, i) => [i, value])), ...history?.edits}});
          return;
        }
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
      {choices.length > 0 && <div class="composer-mentions" role="listbox" id="composer-mentions" aria-label="Files">
        {choices.map((path, i) => <div role="option" ref={i === active ? activeOption : undefined} id={'composer-file-' + i} aria-selected={i === active}
          onMouseDown={e => { e.preventDefault(); choose(path); }}>{path}</div>)}
      </div>}
      <textarea ref={field} rows={1} aria-label="Message" class="composer-text" value={text}
        role="combobox" aria-expanded={choices.length ? 'true' : undefined} aria-autocomplete="list" aria-controls={choices.length ? 'composer-mentions' : undefined}
        aria-activedescendant={choices.length ? 'composer-file-' + active : undefined}
        onInput={input} onKeyDown={key} onBlur={() => setMention(null)}
        onClick={e => { suppressSelect.current = false; updateMention(e.currentTarget); }} onSelect={e => { if (!suppressSelect.current) updateMention(e.currentTarget); }}
        onKeyUp={e => {
          if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) suppressSelect.current = false;
          if (!suppressSelect.current && !['Escape', 'Enter', 'Tab'].includes(e.key)) updateMention(e.currentTarget);
        }} />
      <input ref={picker} type="file" accept={imageTypes.join(',')} multiple hidden onChange={picked} />
      <div class="composer-bar">
        <vscode-button icon-only secondary icon="attach" title="Attach Image" aria-label="Attach Image" onClick={() => picker.current.click()} />
        <div class="composer-details">
          <AgentStatus session={session} />
          <ProfileSelect session={session} />
          {session.id === unsentKey ? <WorktreeToggle /> : <SessionBranch session={session} />}
          <Usage usage={session.usage} />
        </div>
        <vscode-button icon-only secondary icon="send" title="Send" aria-label="Send" disabled={!canSend(state, session, text)} onClick={send} />
      </div>
    </form>
  );
}
