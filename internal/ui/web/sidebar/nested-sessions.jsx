// A parent's nested sessions under a guide line. A collapsed parent lists only
// those waiting on the user, then a row summarizing the rest that expands it.
export function NestedSessions({parent, open, onExpand, row}) {
  const nested = parent.children;
  if (!nested.length) return null;
  const shown = open ? nested : nested.filter(s => s.running && s.attention);
  const rest = nested.filter(s => !shown.includes(s));
  const unread = rest.filter(s => s.unseen).length;
  return (
    <div class="sidebar-nested">
      {shown.map(row)}
      {rest.length > 0 && (
        <button class="sidebar-nested-summary" onClick={onExpand}>
          {rest.length} more nested {rest.length === 1 ? 'session' : 'sessions'}
          {unread > 0 && <><span class="sidebar-nested-separator"> · </span><span class="sidebar-nested-unread">{unread} unread</span></>}
        </button>
      )}
    </div>
  );
}
