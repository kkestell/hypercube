// The elements drawn over the page, such as menus and notification cards. The
// Hypercube window draws a session's browser above the whole page, so the
// browser hides while an overlay covers it. state.overlays holds each mounted
// overlay's rect by ID.
import {useId, useLayoutEffect} from 'preact/hooks';
import {useStore} from '../store.js';

function overlayMoved(state, id, {left, top, right, bottom}) {
  const old = state.overlays[id];
  if (old && old.left === left && old.top === top && old.right === right && old.bottom === bottom) return state;
  return {...state, overlays: {...state.overlays, [id]: {left, top, right, bottom}}};
}

function overlayRemoved(state, id) {
  const overlays = {...state.overlays};
  delete overlays[id];
  return {...state, overlays};
}

// useOverlay keeps the rect of the element in ref in the state while it is
// mounted, as its contents resize it and the window moves it.
export function useOverlay(ref) {
  const [, store] = useStore();
  const id = useId();
  useLayoutEffect(() => {
    const element = ref.current;
    const measure = () => store.update(state => overlayMoved(state, id, element.getBoundingClientRect()));
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    addEventListener('resize', measure);
    return () => {
      observer.disconnect();
      removeEventListener('resize', measure);
      store.update(state => overlayRemoved(state, id));
    };
  }, []);
}

// covered reports whether an overlay covers part of the rect, in CSS pixels.
export const covered = (state, {x, y, width, height}) =>
  Object.values(state.overlays).some(r => r.left < x + width && x < r.right && r.top < y + height && y < r.bottom);
