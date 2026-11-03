// A pane's tab strip, as VS Code's: click or the arrow keys, Home, and End
// select a tab; a file's or the browser's close button closes it, and the
// agent's tab has none;
// a tab drags to another strip or a pane's edge; double-clicking a preview tab
// keeps it open. A preview tab's label is italic and a deleted file's is
// struck through.
import {useRef} from 'preact/hooks';
import {setSource} from '../files/files.js';
import {FileIcon} from '../files/file-icon.jsx';
import {useStore} from '../store.js';
import {closeTab, keepOpen, selectTab, setDragging} from './editor.js';
import './editor-tabs.css';

const keyMoves = {
  ArrowRight: (index, count) => (index + 1) % count,
  ArrowLeft: (index, count) => (index + count - 1) % count,
  Home: () => 0,
  End: (index, count) => count - 1,
};

export function EditorTabs({pane, entries}) {
  const [, store] = useStore();
  const strip = useRef(null);
  const active = entries.find(entry => entry.id === pane.active);
  const modeLabel = active?.source ? 'Show Preview' : 'Show Source';

  // The store redraws the page after a handler returns, so the focus moves
  // once the new tabs are drawn. A closed pane's strip is gone; the focus
  // goes to another pane's active tab, and the agent's pane never closes.
  const focusActiveTab = () => {
    const area = strip.current.closest('.editor-area');
    requestAnimationFrame(() => (strip.current ?? area).querySelector('.editor-tabs-tab-active > .editor-tabs-select').focus());
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
    <div class="editor-tabs-bar">
      <div ref={strip} class="editor-tabs" role="tablist" aria-label="Open tabs" onKeyDown={key}>
        {entries.map(entry => {
          const active = entry.id === pane.active;
          const classes = ['editor-tabs-tab', active && 'editor-tabs-tab-active', entry.agent && 'editor-tabs-tab-agent', entry.preview && 'editor-tabs-tab-preview', entry.deleted && 'editor-tabs-tab-deleted'];
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
                {entry.agent ? <vscode-icon name="comment-discussion" /> : entry.browser ? <vscode-icon name="globe" /> : <FileIcon name={entry.label} />}
                <span class="editor-tabs-label">{entry.label}</span>
              </button>
              {!entry.agent && (
                <button type="button" class="editor-tabs-close" title={`Close ${entry.label}`} aria-label={`Close ${entry.label}`} onClick={() => close(entry.id)}>
                  <vscode-icon name="close" />
                </button>
              )}
            </div>
          );
        })}
      </div>
      {typeof active?.source === 'boolean' && (
        <vscode-toolbar-container class="editor-tabs-actions">
          <vscode-toolbar-button
            icon={active.source ? 'open-preview' : 'go-to-file'}
            label={modeLabel}
            title={modeLabel}
            onClick={() => store.update(state => setSource(state, active.path, !active.source))}
          />
        </vscode-toolbar-container>
      )}
    </div>
  );
}
