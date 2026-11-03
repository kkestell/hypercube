// Places a session's browser over its tab's body. The window draws the
// browser above the whole page, so it hides while hidden is set, while an
// overlay covers the body, and once the body unmounts. Only changes reach the
// window.
import {useLayoutEffect, useRef} from 'preact/hooks';
import {covered} from '../shared/overlays.js';
import {useStore} from '../store.js';
import {post} from './browser.js';

// useBrowserPlacement keeps the session's browser over the element in ref
// after each render and as the element resizes.
export function useBrowserPlacement(ref, session, hidden) {
  const [state] = useStore();
  const placed = useRef('');
  const update = useRef(null);

  // place shows the session's browser at rect, in CSS pixels, or with a null
  // session hides every browser.
  const place = (session, rect) => {
    const message = {type: 'place', session, rect};
    const key = JSON.stringify(message);
    if (key === placed.current) return;
    placed.current = key;
    post(message);
  };
  update.current = () => {
    const {left: x, top: y, width, height} = ref.current.getBoundingClientRect();
    const rect = {x, y, width, height};
    if (hidden || !width || !height || covered(state, rect)) place(null, null);
    else place(session, rect);
  };
  useLayoutEffect(() => update.current());
  useLayoutEffect(() => {
    const observer = new ResizeObserver(() => update.current());
    observer.observe(ref.current);
    return () => {
      observer.disconnect();
      place(null, null);
    };
  }, []);
}
