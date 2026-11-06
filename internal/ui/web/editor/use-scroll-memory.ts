// A view that is hidden, moved to another pane, or rebuilt by a split mounts
// again. useScrollMemory restores its scroll from the store when it mounts,
// records scroll and pagehide events, and saves the final position when it
// unmounts. It returns the record, {top, left, following}, which a transcript
// also uses to follow its end.
import {useLayoutEffect, useRef, type RefObject} from 'react';
import {useStore} from '../store.ts';
import {rememberScroll} from './editor.ts';
import type {ScrollMemory, TabId} from './types.ts';

export function useScrollMemory(ref: RefObject<HTMLElement | null>, id: TabId, ready = true): RefObject<ScrollMemory> {
  const [state, store] = useStore();
  const memory = useRef<ScrollMemory>(state.session?.scroll[id] ?? {top: 0, left: 0, following: true});
  const sessionId = state.session?.id;
  const contentReady = useRef(ready);
  contentReady.current = ready;
  const pendingRestore = useRef<{top: number; left: number} | null>(null);

  useLayoutEffect(() => {
    const view = ref.current;
    if (!view || !ready) return;
    // A split may unmount the old view after this view rendered.
    memory.current = store.get().session?.scroll[id] ?? memory.current;
    // Markdown images can grow the content after its text arrives. Keep the
    // saved position until those images settle, unless the user scrolls first.
    const images = [...view.querySelectorAll('img')].filter(image => !image.complete);
    const restore = () => {
      view.scrollTop = memory.current.top;
      view.scrollLeft = memory.current.left;
      pendingRestore.current = images.some(image => !image.complete) ? {top: view.scrollTop, left: view.scrollLeft} : null;
    };
    const loaded = () => { if (pendingRestore.current) restore(); };
    for (const image of images) {
      image.addEventListener('load', loaded);
      image.addEventListener('error', loaded);
    }
    restore();
    return () => {
      for (const image of images) {
        image.removeEventListener('load', loaded);
        image.removeEventListener('error', loaded);
      }
    };
  }, [ready]);

  useLayoutEffect(() => {
    const view = ref.current;
    if (!view) return;
    const save = () => {
      if (contentReady.current && !pendingRestore.current) memory.current = {...memory.current, top: view.scrollTop, left: view.scrollLeft};
      if (sessionId) store.update(s => rememberScroll(s, sessionId, id, memory.current));
    };
    const scrolled = () => {
      if (!contentReady.current) return;
      const pending = pendingRestore.current;
      if (pending && view.scrollTop === pending.top && view.scrollLeft === pending.left) return;
      pendingRestore.current = null;
      memory.current = {...memory.current, top: view.scrollTop, left: view.scrollLeft};
      save();
    };
    view.addEventListener('scroll', scrolled);
    window.addEventListener('pagehide', save);
    return () => {
      view.removeEventListener('scroll', scrolled);
      window.removeEventListener('pagehide', save);
      save();
    };
  }, []);

  return memory;
}
