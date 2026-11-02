// One session: its transcript above its composer, with a border while it waits
// on the user. Images dropped anywhere on the pane attach to its draft; tab
// drags carry no files and pass through.
import {useState} from 'preact/hooks';
import {Transcript} from '../transcript/transcript.jsx';
import {useStore} from '../store.js';
import {Composer} from './composer.jsx';
import {attachImages} from './drafts.js';
import './agent-pane.css';

const hasFiles = e => e.dataTransfer?.types.includes('Files');

export function AgentPane({session}) {
  const [, store] = useStore();
  const [dropping, setDropping] = useState(false);

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

  const classes = ['agent-pane', session.attention && 'agent-pane-attention', dropping && 'agent-pane-dropping'].filter(Boolean).join(' ');
  return (
    <article class={classes} onDragOver={over} onDragLeave={left} onDrop={dropped}>
      <Transcript session={session} />
      <Composer session={session} />
    </article>
  );
}
