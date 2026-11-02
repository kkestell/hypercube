// A parent's nested sessions under a guide line. An expanded parent lists its
// running sessions and the selected one, then a row that reveals the stopped
// ones. A collapsed parent lists only those waiting on the user, then a row
// summarizing the rest that expands it.
export function NestedSessions({parent, selected, open, revealed, onExpand, onReveal, row}) {
  const nested = parent.children;
  if (!nested.length) return null;
  const shown = !open ? nested.filter(s => s.running && s.attention)
    : revealed ? nested
    : nested.filter(s => s.running || s.id === selected);
  const rest = nested.filter(s => !shown.includes(s));
  const unread = rest.filter(s => s.unseen).length;
  return (
    <div class="sidebar-nested">
      {shown.map(row)}
      {rest.length > 0 && open && (
        <button class="sidebar-nested-summary" onClick={onReveal}>
          Show {rest.length} stopped {rest.length === 1 ? 'session' : 'sessions'}
        </button>
      )}
      {rest.length > 0 && !open && (
        <button class="sidebar-nested-summary" onClick={onExpand}>
          {rest.length} more nested {rest.length === 1 ? 'session' : 'sessions'}
          {unread > 0 && <><span class="sidebar-nested-separator"> · </span><span class="sidebar-nested-unread">{unread} unread</span></>}
        </button>
      )}
    </div>
  );
}
