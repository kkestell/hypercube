// The elements drawn over the page, such as menus and notification cards. The
// Hypercube window draws a session's browser above the whole page, so the
// browser hides while an overlay covers it. state.overlays holds each mounted
// overlay's rect by ID.
import {useId, useLayoutEffect, useRef, useState} from 'react';
import type {State} from '../state.ts';
import {useStore} from '../store.ts';
import {focusedElement} from './focus.ts';
import type {Rect} from './types.ts';

function overlayMoved(state: State, id: string, {left, top, right, bottom}: Rect): State {
  const old = state.overlays[id];
  if (old && old.left === left && old.top === top && old.right === right && old.bottom === bottom) return state;
  return {...state, overlays: {...state.overlays, [id]: {left, top, right, bottom}}};
}

function overlayRemoved(state: State, id: string): State {
  const overlays = {...state.overlays};
  delete overlays[id];
  return {...state, overlays};
}

// useOverlay returns a ref that keeps the element’s rect in state while it is
// mounted, as its contents resize it and the window moves it.
export function useOverlay() {
  const [element, setElement] = useState<HTMLElement | null>(null);
  const [, store] = useStore();
  const id = useId();
  useLayoutEffect(() => {
    if (!element) return;
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
  }, [element, id, store]);
  return setElement;
}

// useModal covers the window and returns focus to the element that opened it.
export function useModal(open: boolean) {
  const [, store] = useStore();
  const id = useId();
  const invoker = useRef<HTMLElement | null>(null);
  useLayoutEffect(() => {
    if (!open) return;
    store.update(state => overlayMoved(state, id, {left: -Infinity, top: -Infinity, right: Infinity, bottom: Infinity}));
    return () => store.update(state => overlayRemoved(state, id));
  }, [open, id, store]);
  return {
    onOpenAutoFocus: () => { invoker.current = focusedElement(); },
    onCloseAutoFocus: (event: Event) => { event.preventDefault(); invoker.current?.focus(); },
  };
}

// covered reports whether an overlay covers part of the rect, in CSS pixels.
export const covered = (state: State, {x, y, width, height}: {x: number; y: number; width: number; height: number}) =>
  Object.values(state.overlays).some(r => r.left < x + width && x < r.right && r.top < y + height && y < r.bottom);
