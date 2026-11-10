// A workspace's sessions as a keyboard-navigable tree, with each parent's
// visible children following it. Enter and Space choose a session; other keys,
// such as Escape, pass on to the app. A parent's children slide open and
// closed beneath it, and leave the keyboard order while it is collapsed.
import {Fragment, useRef, type KeyboardEvent} from 'react';
import {Collapsible, CollapsibleContent} from '../components/ui/collapsible';
import {useTreeFocus} from '../shared/use-tree-focus.ts';
import {chooseSession} from '../sessions/session-actions.ts';
import {expandSession} from '../app/workbench.ts';
import {shownId} from '../sessions/selection.ts';
import {useStore} from '../store.ts';
import {SessionItem} from './session-item.tsx';
import {groupRows, type SessionGroup, type SidebarRow} from './sidebar-rows.ts';

export function SessionTree({label, groups}: {label: string; groups: SessionGroup[]}) {
  const [state, store] = useStore();
  const ref = useRef<HTMLDivElement>(null);
  const rows = groupRows(groups);
  const setExpanded = (id: string, open: boolean) => store.update(s => expandSession(s, id, open));
  const {tabbable, treeProps, keyDown} = useTreeFocus(ref, rows.map(row => ({id: row.id, expandable: row.hasChildren, expanded: row.expanded, parent: row.parent})),
    shownId(state), setExpanded);

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (keyDown(e)) return;
    const row = e.target instanceof HTMLElement ? e.target.closest<HTMLElement>('[data-row]')?.dataset.row : undefined;
    if (row === undefined || (e.key !== 'Enter' && e.key !== ' ')) return;
    e.preventDefault();
    chooseSession(store, row);
  };

  const item = (row: SidebarRow) => <SessionItem key={row.id} row={row} tabbable={row.id === tabbable}
    onToggle={() => setExpanded(row.id, !row.expanded)} />;

  return (
    <div ref={ref} className="flex flex-col" role="tree" aria-label={label} onKeyDown={onKeyDown} {...treeProps}>
      {groups.map(({parent, children}) => <Fragment key={parent.id}>
        {item(parent)}
        {children.length > 0 && <Collapsible open={parent.expanded}>
          <CollapsibleContent className="flex flex-col">{children.map(item)}</CollapsibleContent>
        </Collapsible>}
      </Fragment>)}
    </div>
  );
}
