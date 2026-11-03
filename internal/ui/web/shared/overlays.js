// The elements drawn over the page, such as menus and notification cards. The
// Hypercube window draws a session's browser above the whole page, so the
// browser hides while an overlay covers it.
import {useLayoutEffect} from 'preact/hooks';

const overlays = new Set();
const listeners = new Set();

const changed = () => {
  for (const listener of listeners) listener();
};

// useOverlay counts the element in ref as an overlay while it is mounted.
export function useOverlay(ref) {
  useLayoutEffect(() => {
    const element = ref.current;
    overlays.add(element);
    changed();
    return () => {
      overlays.delete(element);
      changed();
    };
  }, []);
}

// covered reports whether an overlay covers part of the rect, in CSS pixels.
export function covered({x, y, width, height}) {
  return [...overlays].some(element => {
    const r = element.getBoundingClientRect();
    return r.left < x + width && x < r.right && r.top < y + height && y < r.bottom;
  });
}

// onOverlaysChange calls listener when an overlay mounts or unmounts, and
// returns a function that stops it.
export function onOverlaysChange(listener) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}
