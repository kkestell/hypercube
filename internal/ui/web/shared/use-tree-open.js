// vscode-tree-item opens and closes itself, by mouse or keyboard, and fires no
// event when it does. useTreeOpen watches the reflected open attribute of the
// items in a tree and calls onToggle(key, open) with the item's data-key, so
// the component that owns the expansion state learns of every change.
import {useEffect, useRef} from 'preact/hooks';

export function useTreeOpen(ref, onToggle) {
  const latest = useRef(onToggle);
  latest.current = onToggle;
  useEffect(() => {
    const observer = new MutationObserver(mutations => {
      for (const {target} of mutations) {
        if (target.dataset.key !== undefined) latest.current(target.dataset.key, target.hasAttribute('open'));
      }
    });
    observer.observe(ref.current, {subtree: true, attributes: true, attributeFilter: ['open']});
    return () => observer.disconnect();
  }, []);
}
