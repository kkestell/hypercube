// A pane's tab strip, as VS Code's: click or the arrow keys, Home, and End
// select a tab; the close button closes it; a tab drags to another strip or a
// pane's edge; double-clicking a preview tab keeps it open. A preview tab's
// label is italic and a deleted file's is struck through.
import {useRef} from 'preact/hooks';
import {FileIcon} from '../files/file-icon.jsx';
import {AgentStatus} from '../shared/agent-status.jsx';
import {useStore} from '../store.js';
import {closeTab, keepOpen, selectTab, setDragging} from './editor.js';
import './editor-tabs.css';

const keyMoves = {
  ArrowRight: (index, count) => (index + 1) % count,
  ArrowLeft: (index, count) => (index + count - 1) % count,
  Home: () => 0,
  End: (index, count) => count - 1,
};

// lead is the lead's agent ID, whose text box takes the focus when the last
// tab closes.
export function EditorTabs({pane, entries, lead}) {
  const [, store] = useStore();
  const strip = useRef(null);

  // The store redraws the page after a handler returns, so the focus moves
  // once the new tabs are drawn. A closed pane's strip is gone; the focus
  // goes to another pane's active tab, or to the lead.
  const focusActiveTab = () => {
    const area = strip.current.closest('.editor-area');
    requestAnimationFrame(() => {
      const tab = (strip.current ?? area).querySelector('.editor-tabs-tab-active > .editor-tabs-select');
      (tab ?? document.querySelector(`.composer[data-agent="${lead}"] textarea`))?.focus();
    });
  };

  const close = id => {
    focusActiveTab();
    store.update(s => closeTab(s, id));
  };

  const key = e => {
    const move = keyMoves[e.key];
    if (!move) return;
    e.preventDefault();
    const next = pane.tabs[move(pane.tabs.indexOf(pane.active), pane.tabs.length)];
    focusActiveTab();
    store.update(s => selectTab(s, pane.id, next));
  };

  return (
    <div ref={strip} class="editor-tabs" role="tablist" aria-label="Open tabs" onKeyDown={key}>
      {entries.map(entry => {
        const active = entry.id === pane.active;
        const classes = ['editor-tabs-tab', active && 'editor-tabs-tab-active', entry.preview && 'editor-tabs-tab-preview', entry.deleted && 'editor-tabs-tab-deleted'];
        return (
          <div
            key={entry.id}
            class={classes.filter(Boolean).join(' ')}
            data-tab={entry.id}
            draggable
            onDragStart={e => {
              e.dataTransfer.setData('text/plain', entry.id);
              e.dataTransfer.effectAllowed = 'move';
              store.update(s => setDragging(s, entry.id));
            }}
            onDragEnd={() => store.update(s => setDragging(s, null))}
          >
            <button
              type="button"
              role="tab"
              class="editor-tabs-select"
              aria-selected={active}
              tabIndex={active ? 0 : -1}
              title={entry.title}
              onClick={() => store.update(s => selectTab(s, pane.id, entry.id))}
              onDblClick={() => store.update(s => keepOpen(s, entry.id))}
            >
              {entry.path !== undefined && <FileIcon name={entry.label} />}
              <span class="editor-tabs-label">{entry.label}</span>
              {entry.agent && <AgentStatus agent={entry.agent} />}
            </button>
            <button type="button" class="editor-tabs-close" title={`Close ${entry.label}`} aria-label={`Close ${entry.label}`} onClick={() => close(entry.id)}>
              <vscode-icon name="close" />
            </button>
          </div>
        );
      })}
    </div>
  );
}
