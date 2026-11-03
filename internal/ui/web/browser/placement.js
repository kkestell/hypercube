// Places a session's browser over its tab's body. The window draws the
// browser above the whole page, so it hides while hidden is set, while an
// overlay covers the body, and once the body unmounts. Only changes reach the
// window.
import {useLayoutEffect, useRef} from 'preact/hooks';
import {covered, onOverlaysChange} from '../shared/overlays.js';
import {post} from './browser.js';

let placed = '';

// place shows the session's browser at rect, in CSS pixels, or with a null
// session hides every browser.
function place(session, rect) {
  const message = {type: 'place', session, rect};
  const key = JSON.stringify(message);
  if (key === placed) return;
  placed = key;
  post(message);
}

// useBrowserPlacement keeps the session's browser over the element in ref
// after each render and as the element resizes.
export function useBrowserPlacement(ref, session, hidden) {
  const update = useRef(null);
  update.current = () => {
    const {left: x, top: y, width, height} = ref.current.getBoundingClientRect();
    const rect = {x, y, width, height};
    if (hidden || !width || !height || covered(rect)) place(null, null);
    else place(session, rect);
  };
  useLayoutEffect(() => update.current());
  useLayoutEffect(() => {
    const changed = () => update.current();
    const observer = new ResizeObserver(changed);
    observer.observe(ref.current);
    const stop = onOverlaysChange(changed);
    return () => {
      observer.disconnect();
      stop();
      place(null, null);
    };
  }, []);
}
