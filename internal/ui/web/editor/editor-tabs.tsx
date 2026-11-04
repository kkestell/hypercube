// Stock tabs with a separate close action and the existing drag/drop behavior.
import {useRef} from 'react';
import {flushSync} from 'react-dom';
import {toggleWrap} from '../app/workbench.ts';
import {Button} from '../components/ui/button';
import {Tabs, TabsList, TabsTrigger} from '../components/ui/tabs';
import {setSource} from '../files/files.ts';
import {FileIcon} from '../files/file-icon.tsx';
import {usePathMenu} from '../files/path-menu.tsx';
import {Icon} from '../shared/icon.tsx';
import {useStore} from '../store.ts';
import {closeTab, keepOpen, selectTab, setDragging} from './editor.ts';
import type {Pane, TabEntry, TabId} from './types.ts';

// With no pane, the bar is empty: no session is shown.
export function EditorTabs({pane, entries}: {pane: Pane | null; entries: TabEntry[]}) {
  const [state, store] = useStore();
  const strip = useRef<HTMLDivElement>(null);
  const menu = usePathMenu();
  const active = entries.find(entry => entry.id === pane?.active);
  const modeLabel = active?.kind === 'file' && active.source ? 'Show Preview' : 'Show Source';
  const close = (id: TabId) => {
    const area = strip.current?.closest('.editor-area');
    flushSync(() => store.update(s => closeTab(s, id)));
    (strip.current ?? area)?.querySelector<HTMLElement>('[role="tab"][data-state="active"]')?.focus();
  };
  return <div className="editor-tabs-bar flex min-w-0 shrink-0 items-center justify-between gap-2 border-b p-2">
    <Tabs className="min-w-0 flex-1 overflow-x-auto overflow-y-hidden" value={pane?.active ?? ''} onValueChange={id => { if (pane) store.update(s => selectTab(s, pane.id, id)); }}>
      <TabsList ref={strip} className="editor-tabs max-w-full justify-start" aria-label="Open tabs">
        {entries.map(entry => <div key={entry.id} data-tab={entry.id} draggable className="flex min-w-0 max-w-full shrink-0 items-center"
          onDragStart={e => {
            e.dataTransfer.setData('text/plain', entry.id);
            e.dataTransfer.effectAllowed = 'move';
            store.update(s => setDragging(s, entry.id));
          }} onDragEnd={() => store.update(s => setDragging(s, null))}>
          <TabsTrigger {...(entry.kind === 'file' ? menu.handlers(entry.path) : {})} className="min-w-0 max-w-64" value={entry.id} title={entry.title} onDoubleClick={() => store.update(s => keepOpen(s, entry.id))}>
            {entry.kind === 'agent' ? <Icon name="comment-discussion" /> : entry.kind === 'browser' ? <Icon name="globe" /> : <FileIcon name={entry.name} />}
            <span className={`editor-tabs-label min-w-0 truncate ${entry.preview ? 'italic' : ''} ${entry.kind === 'file' && entry.deleted ? 'line-through' : ''}`}>{entry.label}</span>
          </TabsTrigger>
          {entry.kind !== 'agent' && <Button className="editor-tabs-close" variant="ghost" size="icon" aria-label={`Close ${entry.label}`} title={`Close ${entry.label}`} onClick={() => close(entry.id)}><Icon name="close" /></Button>}
        </div>)}
      </TabsList>
    </Tabs>
    {active?.kind === 'file' && active.source !== null && <Button className="editor-tabs-actions" variant="ghost" size="icon" aria-label={modeLabel} title={modeLabel}
      onClick={() => store.update(s => setSource(s, active.path, !active.source))}><Icon name={active.source ? 'open-preview' : 'go-to-file'} /></Button>}
    {(active?.kind === 'file' || active?.kind === 'diff') && <Button variant="ghost" size="icon" aria-label="Toggle Word Wrap" title="Toggle Word Wrap (Option+Z)" aria-pressed={state.workbench.wrap} onClick={() => store.update(toggleWrap)}><Icon name="wrap" /></Button>}
    {menu.element}
  </div>;
}
