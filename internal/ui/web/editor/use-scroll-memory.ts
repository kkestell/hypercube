// A view that is hidden, moved to another pane, or rebuilt by a split mounts
// again. useScrollMemory restores its scroll from the store when it mounts,
// records the scroll as the user scrolls, and saves it to the store when it
// unmounts. It returns the record, {top, left, following}, which a transcript
// also uses to follow its end.
import {useLayoutEffect, useRef, type RefObject} from 'react';
import {useStore} from '../store.ts';
import {rememberScroll} from './editor.ts';
import type {ScrollMemory, TabId} from './types.ts';

export function useScrollMemory(ref: RefObject<HTMLElement | null>, id: TabId): RefObject<ScrollMemory> {
  const [state, store] = useStore();
  const memory = useRef<ScrollMemory>({top: 0, left: 0, following: true});
  const sessionId = state.session?.id;

  useLayoutEffect(() => {
    const view = ref.current;
    if (!view) return;
    // A split may unmount the old view after this view rendered.
    memory.current = store.get().session?.scroll[id] ?? memory.current;
    view.scrollTop = memory.current.top;
    view.scrollLeft = memory.current.left;
    const scrolled = () => {
      memory.current = {...memory.current, top: view.scrollTop, left: view.scrollLeft};
    };
    view.addEventListener('scroll', scrolled);
    return () => {
      view.removeEventListener('scroll', scrolled);
      if (sessionId) store.update(s => rememberScroll(s, sessionId, id, memory.current));
    };
  }, []);

  return memory;
}
