// Side-panel tabs keep both views mounted when switching and use the same
// tab geometry as the editor. Arrow keys, Home, and End select and focus tabs.
import {useId} from 'preact/hooks';
import './tabs.css';
import './panel-tabs.css';

export function PanelTabs({label, tabs, selected, onSelect, actions, children}) {
  const id = useId();
  const keyDown = e => {
    const index = tabs.findIndex(tab => tab.id === selected);
    let next;
    switch (e.key) {
      case 'ArrowRight': next = (index + 1) % tabs.length; break;
      case 'ArrowLeft': next = (index + tabs.length - 1) % tabs.length; break;
      case 'Home': next = 0; break;
      case 'End': next = tabs.length - 1; break;
      default: return;
    }
    e.preventDefault();
    e.currentTarget.querySelectorAll('[role="tab"]')[next].focus();
    onSelect?.(tabs[next].id);
  };

  return (
    <div class="panel-tabs">
      <div class="tabs-bar">
        <div class="tabs-strip" role="tablist" aria-label={label} onKeyDown={keyDown}>
          {tabs.map(tab => (
            <div key={tab.id} class={`tabs-tab${tab.id === selected ? ' tabs-tab-active' : ''}`}>
              <button type="button" class="tabs-select" role="tab" id={`${id}-${tab.id}-tab`}
                aria-controls={`${id}-${tab.id}-panel`} aria-selected={tab.id === selected}
                tabIndex={tab.id === selected ? 0 : -1} title={tab.title} aria-label={tab.title}
                onClick={() => onSelect?.(tab.id)}>
                <vscode-icon name={tab.icon} />
              </button>
            </div>
          ))}
        </div>
        {actions && <vscode-toolbar-container class="tabs-actions">{actions}</vscode-toolbar-container>}
      </div>
      {tabs.map((tab, index) => (
        <div key={tab.id} class="panel-tabs-content" role="tabpanel" tabIndex={0} id={`${id}-${tab.id}-panel`}
          aria-labelledby={`${id}-${tab.id}-tab`} hidden={tab.id !== selected}>
          {Array.isArray(children) ? children[index] : children}
        </div>
      ))}
    </div>
  );
}
