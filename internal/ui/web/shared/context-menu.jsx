// A menu at a point, kept inside the window. It closes when an item is
// chosen, on Escape, on a press outside it, or when the window loses focus.
// Escape and selection return focus to where it was before the menu took it.
// items are [{label, run}].
import {useEffect, useLayoutEffect, useRef, useState} from 'preact/hooks';
import {useOverlay} from './overlays.js';
import './context-menu.css';

export function ContextMenu({x, y, items, onClose}) {
  const ref = useRef(null);
  // The menu focuses itself once it renders, after this first render.
  const [invoker] = useState(() => document.activeElement);

  // The menu draws its items after this render, so it is kept inside the
  // window again once it has its size. This comes before useOverlay so that
  // the overlay measures the moved menu.
  useLayoutEffect(() => {
    const box = ref.current;
    const clamp = () => {
      box.style.left = Math.max(0, Math.min(x, innerWidth - box.offsetWidth)) + 'px';
      box.style.top = Math.max(0, Math.min(y, innerHeight - box.offsetHeight)) + 'px';
    };
    clamp();
    const observer = new ResizeObserver(clamp);
    observer.observe(box);
    return () => observer.disconnect();
  }, [x, y]);
  useOverlay(ref);

  useEffect(() => {
    const pressed = e => {
      if (!ref.current.contains(e.target)) onClose();
    };
    const key = e => {
      if (e.key !== 'Escape') return;
      onClose();
      invoker.focus();
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

  const chosen = e => {
    onClose();
    invoker.focus();
    items[Number(e.detail.value)].run();
  };

  return (
    <div ref={ref} class="context-menu">
      <vscode-context-menu
        data={items.map((item, index) => ({label: item.label, value: String(index)}))}
        show
        onvsc-context-menu-select={chosen}
      />
    </div>
  );
}
