// Rows from Git's unified whole-file patch, excluding patch metadata.
export function diffRows(patch) {
  const rows = [];
  let old, next;
  for (const line of patch.split('\n')) {
    const hunk = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(line);
    if (hunk) {
      old = Number(hunk[1]);
      next = Number(hunk[2]);
      continue;
    }
    if (old === undefined) continue;
    const text = line.slice(1);
    if (line[0] === ' ') rows.push({kind: 'context', old: old++, new: next++, text});
    else if (line[0] === '-') rows.push({kind: 'removed', old: old++, new: null, text});
    else if (line[0] === '+') rows.push({kind: 'added', old: null, new: next++, text});
  }
  return rows;
}

// Metadata-only patches still explain the change without any text rows.
export function diffNote(patch) {
  const modes = /^old mode (\d+)\nnew mode (\d+)$/m.exec(patch);
  if (modes) return `File mode changed from ${modes[1]} to ${modes[2]}`;
  if (/^(?:new|deleted) file mode /m.test(patch)) return 'Empty file';
  return 'No changes';
}
