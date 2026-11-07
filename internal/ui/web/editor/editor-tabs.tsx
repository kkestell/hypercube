// Stock tabs with a separate close action and the existing drag/drop behavior.
import {useRef} from 'react';
import {flushSync} from 'react-dom';
import {tabBackground} from '../shared/tab-background.ts';
import {barTabs, topRow, topRowStart} from '../app/title-bar.ts';
import {LayoutToggles} from '../app/layout-toggles.tsx';
import {Button} from '../components/ui/button';
import {Tabs, TabsList, TabsTrigger} from '../components/ui/tabs';
import {FileIcon} from '../files/file-icon.tsx';
import {usePathMenu} from '../files/path-menu.tsx';
import {Icon} from '../shared/icon.tsx';
import {useStore} from '../store.ts';
import {closeTab, keepOpen, selectTab, setDragging} from './editor.ts';
import type {Pane, Placement, TabEntry, TabId} from './types.ts';
import {ViewOptions} from './view-options.tsx';

// With no pane, the bar is empty: no session is shown. A pane on the top row
// is in the window's title bar, so it is as tall as the title bar; the one at
// the left clears the window's buttons when the sidebar is hidden.
export function EditorTabs({pane, entries, placement}: {pane: Pane | null; entries: TabEntry[]; placement: Placement}) {
  const [state, store] = useStore();
  const strip = useRef<HTMLDivElement>(null);
  const menu = usePathMenu();
  const active = entries.find(entry => entry.id === pane?.active);
  const close = (id: TabId) => {
    const area = strip.current?.closest('.editor-area');
    flushSync(() => store.update(s => closeTab(s, id)));
    (strip.current ?? area)?.querySelector<HTMLElement>('[role="tab"][data-state="active"]')?.focus();
  };
  const start = placement.top && placement.left && !state.workbench.sidebar;
  return <div className={`editor-tabs-bar flex min-w-0 shrink-0 items-center justify-between gap-1 px-2 ${tabBackground} ${placement.top ? `${topRow} border-b` : 'py-1'} ${start ? topRowStart : ''}`}>
    {entries.length > 0 ? <Tabs className="min-w-0 flex-1 overflow-x-auto overflow-y-hidden [scrollbar-width:none]" value={pane?.active ?? ''} onValueChange={id => { if (pane) store.update(s => selectTab(s, pane.id, id)); }}>
      <TabsList ref={strip} className={`editor-tabs max-w-full justify-start ${barTabs}`} aria-label="Open tabs">
        {entries.map(entry => <div key={entry.id} data-tab={entry.id} draggable className="flex min-w-24 max-w-64 items-center"
          onDragStart={e => {
            e.dataTransfer.setData('text/plain', entry.id);
            e.dataTransfer.effectAllowed = 'move';
            store.update(s => setDragging(s, entry.id));
          }} onDragEnd={() => store.update(s => setDragging(s, null))}>
          <TabsTrigger {...(entry.kind === 'file' ? menu.handlers(entry.path) : {})} className="min-w-0" value={entry.id} title={entry.title} onDoubleClick={() => store.update(s => keepOpen(s, entry.id))}>
            {(entry.kind === 'agent' || entry.kind === 'dashboard') ? <Icon name={entry.kind === 'dashboard' ? 'files' : 'comment-discussion'} /> : <FileIcon name={entry.name} />}
            <span className={`editor-tabs-label min-w-0 truncate ${entry.preview ? 'italic' : ''} ${entry.kind === 'file' && entry.deleted ? 'line-through' : ''}`}>{entry.label}</span>
          </TabsTrigger>
          {entry.kind !== 'agent' && entry.kind !== 'dashboard' && <Button className="editor-tabs-close" variant="ghost" size="icon-xs" aria-label={`Close ${entry.label}`} title={`Close ${entry.label}`} onClick={() => close(entry.id)}><Icon name="close" /></Button>}
        </div>)}
      </TabsList>
    </Tabs> : <div className="flex-1" />}
    <ViewOptions active={active} />
    {placement.top && placement.right && <LayoutToggles />}
    {menu.element}
  </div>;
}
