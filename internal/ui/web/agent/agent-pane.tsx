// One session: its transcript above its composer, or the tesseract in place of
// the transcript before its first prompt. Images dropped anywhere on the
// pane attach to its draft; tab drags carry no files and pass through.
import {useState, type DragEvent} from 'react';
import type {Session} from '../sessions/types.ts';
import {useStore} from '../store.ts';
import {Tesseract} from '../tesseract/tesseract.tsx';
import {Transcript} from '../transcript/transcript.tsx';
import {Composer} from './composer.tsx';
import {attachImages, unsentKey} from './drafts.ts';

const hasFiles = (e: DragEvent) => e.dataTransfer.types.includes('Files');

export function AgentPane({session, active}: {session: Session; active: boolean}) {
  const [state, store] = useStore();
  const [dropping, setDropping] = useState(false);
  const unsent = session.id === unsentKey;

  const over = (e: DragEvent<HTMLElement>) => {
    if (!hasFiles(e)) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'copy';
    setDropping(true);
  };
  const left = (e: DragEvent<HTMLElement>) => {
    if (!(e.relatedTarget instanceof Node && e.currentTarget.contains(e.relatedTarget))) setDropping(false);
  };
  const dropped = (e: DragEvent<HTMLElement>) => {
    if (!hasFiles(e)) return;
    e.preventDefault();
    setDropping(false);
    attachImages(store, session.id, [...e.dataTransfer.files]);
  };

  const classes = `agent-pane flex h-full min-h-0 flex-col bg-surface ${dropping ? 'ring-2 ring-ring ring-inset' : ''}`;
  return (
    <article className={classes} onDragOver={over} onDragLeave={left} onDrop={dropped}>
      {unsent ? <Tesseract fading={state.unsent?.sending ?? false} /> : <Transcript session={session} active={active} />}
      <Composer session={session} />
    </article>
  );
}
