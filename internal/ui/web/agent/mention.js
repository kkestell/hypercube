// Locate the complete @ token around the caret, but match only what precedes it.
export function mentionAt(text, start, end) {
  if (start !== end) return null;
  const before = text.slice(0, start);
  const match = before.match(/(?:^|\s)@([^\s@]*)$/);
  if (!match) return null;
  const tail = text.slice(start).match(/^[^\s@]*/)[0];
  return {start: start - match[1].length - 1, end: start + tail.length, query: match[1].toLowerCase()};
}

export function prepareFiles(paths) {
  return paths.map(path => ({path, lower: path.toLowerCase()})).sort((a, b) => a.path.length - b.path.length || a.path.localeCompare(b.path));
}

// Scan the best match category first; prepared paths already put shorter paths first.
export function mentionChoices(files, query, limit = 30) {
  const result = [];
  for (let rank = 0; rank < 3 && result.length < limit; rank++) {
    for (const file of files) {
      const base = file.lower.slice(file.lower.lastIndexOf('/') + 1);
      const contiguous = file.lower.includes(query);
      let i = 0;
      for (const c of file.lower) if (c === query[i]) i++;
      if (i !== query.length) continue;
      const category = base.includes(query) ? 0 : contiguous ? 1 : 2;
      if (category === rank) result.push(file.path);
      if (result.length === limit) break;
    }
  }
  return result;
}

export function insertMention(text, mention, path) {
  return text.slice(0, mention.start) + '@' + path + text.slice(mention.end);
}
