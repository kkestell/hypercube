// Stock tabs with a separate close action and the existing drag/drop behavior.
import {useRef} from 'react';
import {flushSync} from 'react-dom';
import {Button} from '../components/ui/button';
import {Tabs, TabsList, TabsTrigger} from '../components/ui/tabs';
import {setSource} from '../files/files.js';
import {FileIcon} from '../files/file-icon.jsx';
import {Icon} from '../shared/icon.jsx';
import {useStore} from '../store.js';
import {closeTab, keepOpen, selectTab, setDragging} from './editor.js';

export function EditorTabs({pane, entries}) {
  const [, store] = useStore();
  const strip = useRef(null);
  const active = entries.find(entry => entry.id === pane?.active);
  const modeLabel = active?.source ? 'Show Preview' : 'Show Source';
  const close = id => {
    const area = strip.current.closest('.editor-area');
    flushSync(() => store.update(s => closeTab(s, id)));
    (strip.current ?? area)?.querySelector('[role="tab"][data-state="active"]')?.focus();
  };
  return <div className="editor-tabs-bar flex min-w-0 shrink-0 items-center justify-between gap-2 border-b p-2">
    <Tabs className="min-w-0 flex-1 overflow-x-auto" value={pane?.active ?? ''} onValueChange={id => store.update(s => selectTab(s, pane.id, id))}>
      <TabsList ref={strip} className="editor-tabs max-w-full justify-start" aria-label="Open tabs">
        {entries.map(entry => <div key={entry.id} data-tab={entry.id} draggable className="flex min-w-0 max-w-full shrink-0 items-center"
          onDragStart={e => {
            e.dataTransfer.setData('text/plain', entry.id);
            e.dataTransfer.effectAllowed = 'move';
            store.update(s => setDragging(s, entry.id));
          }} onDragEnd={() => store.update(s => setDragging(s, null))}>
          <TabsTrigger className="min-w-0 max-w-64" value={entry.id} title={entry.title} onDoubleClick={() => store.update(s => keepOpen(s, entry.id))}>
            {entry.agent ? <Icon name="comment-discussion" /> : entry.browser ? <Icon name="globe" /> : <FileIcon name={entry.name} />}
            <span className={`editor-tabs-label min-w-0 truncate ${entry.preview ? 'italic' : ''} ${entry.deleted ? 'line-through' : ''}`}>{entry.label}</span>
          </TabsTrigger>
          {!entry.agent && <Button className="editor-tabs-close" variant="ghost" size="icon" aria-label={`Close ${entry.label}`} title={`Close ${entry.label}`} onClick={() => close(entry.id)}><Icon name="close" /></Button>}
        </div>)}
      </TabsList>
    </Tabs>
    {typeof active?.source === 'boolean' && <Button className="editor-tabs-actions" variant="ghost" size="icon" aria-label={modeLabel} title={modeLabel}
      onClick={() => store.update(s => setSource(s, active.path, !active.source))}><Icon name={active.source ? 'open-preview' : 'go-to-file'} /></Button>}
  </div>;
}
