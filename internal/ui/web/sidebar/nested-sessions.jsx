// A parent's running children, the selected one,
// and stopped children when the parent's menu reveals them.
export function NestedSessions({parent, selected, showStopped, row}) {
  const shown = parent.children.filter(s => s.running || showStopped || s.id === selected);
  if (!shown.length) return null;
  return <div class="sidebar-nested">{shown.map(row)}</div>;
}
