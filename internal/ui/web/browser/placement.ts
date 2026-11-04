// Places a session's browser over its tab's body. The window draws the
// browser above the whole page, so it hides while hidden is set, while an
// overlay covers the body, and once the body unmounts. Only changes reach the
// window.
import {useLayoutEffect, useRef, type RefObject} from 'react';
import {covered} from '../shared/overlays.ts';
import {useStore} from '../store.ts';
import {post} from './browser.ts';
import type {BrowserRect} from './types.ts';

// useBrowserPlacement keeps the session's browser over the element in ref
// after each render and as the element resizes.
export function useBrowserPlacement(ref: RefObject<HTMLElement | null>, session: string, hidden: boolean): void {
  const [state] = useStore();
  const placed = useRef('');
  const update = useRef<() => void>(() => {});

  // place shows the session's browser at rect, in CSS pixels, or with a null
  // session hides every browser.
  const place = (session: string | null, rect: BrowserRect | null) => {
    const message = {type: 'place' as const, session, rect};
    const key = JSON.stringify(message);
    if (key === placed.current) return;
    placed.current = key;
    post(message);
  };
  update.current = () => {
    if (!ref.current) return;
    const {left: x, top: y, width, height} = ref.current.getBoundingClientRect();
    const rect = {x, y, width, height};
    if (hidden || !width || !height || covered(state, rect)) place(null, null);
    else place(session, rect);
  };
  useLayoutEffect(() => update.current());
  useLayoutEffect(() => {
    const observer = new ResizeObserver(() => update.current());
    if (ref.current) observer.observe(ref.current);
    return () => {
      observer.disconnect();
      place(null, null);
    };
  }, []);
}
