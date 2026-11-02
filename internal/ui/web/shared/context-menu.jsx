// A menu at a point. It closes when an item is chosen, on Escape, on a press
// outside it, or when the window loses focus. items are [{label, run}].
import {useRef} from 'preact/hooks';
import {usePopup} from './popup.js';
import './context-menu.css';

export function ContextMenu({x, y, items, onClose}) {
  const ref = useRef(null);

  usePopup(ref, x, y, onClose);

  const chosen = e => {
    onClose();
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
