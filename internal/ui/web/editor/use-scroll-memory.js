// A view that is hidden, moved to another pane, or rebuilt by a split mounts
// again. useScrollMemory restores its scroll from the store when it mounts,
// records the scroll as the user scrolls, and saves it to the store when it
// unmounts. It returns the record, {top, left, following}, which a transcript
// also uses to follow its end.
import {useLayoutEffect, useRef} from 'preact/hooks';
import {useStore} from '../store.js';
import {rememberScroll} from './editor.js';

export function useScrollMemory(ref, id) {
  const [state, store] = useStore();
  const memory = useRef(state.session?.scroll[id] ?? {top: 0, left: 0, following: true});
  const sessionId = state.session?.id;

  useLayoutEffect(() => {
    const view = ref.current;
    view.scrollTop = memory.current.top;
    view.scrollLeft = memory.current.left;
    const scrolled = () => {
      memory.current = {...memory.current, top: view.scrollTop, left: view.scrollLeft};
    };
    view.addEventListener('scroll', scrolled);
    return () => {
      view.removeEventListener('scroll', scrolled);
      store.update(s => rememberScroll(s, sessionId, id, memory.current));
    };
  }, []);

  return memory;
}
