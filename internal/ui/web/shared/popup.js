// Keep a popup inside the window and close it on Escape, an outside press,
// or when the window loses focus.
import {useEffect, useLayoutEffect} from 'preact/hooks';

export function usePopup(ref, x, y, onClose) {
  useLayoutEffect(() => {
    const box = ref.current;
    box.style.left = Math.max(0, Math.min(x, innerWidth - box.offsetWidth)) + 'px';
    box.style.top = Math.max(0, Math.min(y, innerHeight - box.offsetHeight)) + 'px';
  }, [x, y]);

  useEffect(() => {
    const pressed = e => {
      if (!ref.current.contains(e.target)) onClose();
    };
    const key = e => {
      if (e.key === 'Escape') onClose();
    };
    addEventListener('pointerdown', pressed, true);
    addEventListener('keydown', key);
    addEventListener('blur', onClose);
    return () => {
      removeEventListener('pointerdown', pressed, true);
      removeEventListener('keydown', key);
      removeEventListener('blur', onClose);
    };
  }, [onClose]);
}
