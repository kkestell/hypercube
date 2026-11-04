// One session: its transcript above its composer, or the tesseract in place of
// the transcript before its first prompt. Images dropped anywhere on the
// pane attach to its draft; tab drags carry no files and pass through.
import {useState} from 'preact/hooks';
import {useStore} from '../store.js';
import {Tesseract} from '../tesseract/tesseract.jsx';
import {Transcript} from '../transcript/transcript.jsx';
import {Composer} from './composer.jsx';
import {attachImages, unsentKey} from './drafts.js';
import {WorkspaceSelect} from './workspace-select.jsx';
import './agent-pane.css';

const hasFiles = e => e.dataTransfer?.types.includes('Files');

export function AgentPane({session}) {
  const [state, store] = useStore();
  const [dropping, setDropping] = useState(false);
  const unsent = session.id === unsentKey;

  const over = e => {
    if (!hasFiles(e)) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'copy';
    setDropping(true);
  };
  const left = e => {
    if (!e.currentTarget.contains(e.relatedTarget)) setDropping(false);
  };
  const dropped = e => {
    if (!hasFiles(e)) return;
    e.preventDefault();
    setDropping(false);
    attachImages(store, session.id, [...e.dataTransfer.files]);
  };

  const classes = ['agent-pane', dropping && 'agent-pane-dropping'].filter(Boolean).join(' ');
  return (
    <article class={classes} onDragOver={over} onDragLeave={left} onDrop={dropped}>
      {unsent ? <Tesseract fading={state.unsent.sending} /> : <Transcript session={session} />}
      <Composer session={session} />
      {unsent && <WorkspaceSelect />}
    </article>
  );
}
