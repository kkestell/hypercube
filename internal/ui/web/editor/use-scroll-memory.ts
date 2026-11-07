// A view that is hidden, moved to another pane, or rebuilt by a split mounts
// again. This hook owns restoration and recording, including a saved target
// that incomplete images may temporarily clamp. Transcripts also ask it to
// follow the end or preserve a disclosure's position.
import {useLayoutEffect, useRef, useState, type RefObject} from 'react';
import {useStore} from '../store.ts';
import {rememberScroll} from './editor.ts';
import type {ScrollMemory, TabId} from './types.ts';

export function useScrollMemory(ref: RefObject<HTMLElement | null>, id: TabId, ready = true, trackFollowing = false) {
  const [state, store] = useStore();
  const memory = useRef<ScrollMemory>(state.editor?.scroll[id] ?? {top: 0, left: 0, following: true});
  const viewKey = state.editor?.id;
  const contentReady = useRef(ready);
  contentReady.current = ready;
  const pendingRestore = useRef<{top: number; left: number} | null>(null);
  const lastTop = useRef(memory.current.top);
  const [away, setAway] = useState(!memory.current.following);

  const atEnd = (view: HTMLElement) => view.scrollHeight - view.scrollTop - view.clientHeight < 2;
  const save = () => {
    const view = ref.current;
    if (view && contentReady.current && !pendingRestore.current) memory.current = {...memory.current, top: view.scrollTop, left: view.scrollLeft};
    if (viewKey) store.update(s => rememberScroll(s, viewKey, id, memory.current));
  };
  const followEnd = (force = false) => {
    const view = ref.current;
    if (!contentReady.current || !view || (!force && !memory.current.following)) return;
    pendingRestore.current = null;
    memory.current = {...memory.current, following: true};
    view.scrollTop = view.scrollHeight;
    lastTop.current = view.scrollTop;
    setAway(false);
    save();
  };
  const holdPosition = (delta: number) => {
    const view = ref.current;
    if (!contentReady.current || !view) return;
    // A disclosure is an explicit change of reading position.
    pendingRestore.current = null;
    view.scrollTop += delta;
    memory.current = {...memory.current, following: atEnd(view)};
    lastTop.current = view.scrollTop;
    setAway(!memory.current.following);
    save();
  };

  useLayoutEffect(() => {
    const view = ref.current;
    if (!view || !ready) return;
    // A split may unmount the old view after this view rendered.
    memory.current = store.get().editor?.scroll[id] ?? memory.current;
    // Keep the saved position until images settle, unless the user scrolls.
    const images = [...view.querySelectorAll('img')].filter(image => !image.complete);
    const restore = () => {
      view.scrollTop = memory.current.top;
      view.scrollLeft = memory.current.left;
      lastTop.current = view.scrollTop;
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
    const scrolled = () => {
      if (!contentReady.current) return;
      const pending = pendingRestore.current;
      if (pending && view.scrollTop === pending.top && view.scrollLeft === pending.left) return;
      pendingRestore.current = null;
      if (trackFollowing) {
        const end = atEnd(view);
        if (end) memory.current = {...memory.current, following: true};
        else if (view.scrollTop < lastTop.current) memory.current = {...memory.current, following: false};
        setAway(!end);
      }
      lastTop.current = view.scrollTop;
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

  return {away, followEnd, holdPosition};
}
