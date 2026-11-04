// Mention is the @ token around the caret: its range in the text and the
// lowercase query after the @.
export interface Mention {
  start: number;
  end: number;
  query: string;
}

// MentionFile is a path with its lowercase form, which matching compares.
export interface MentionFile {
  path: string;
  lower: string;
}

// Locate the complete @ token around the caret, but match only what precedes it.
export function mentionAt(text: string, start: number, end: number): Mention | null {
  if (start !== end) return null;
  const before = text.slice(0, start);
  const match = before.match(/(?:^|\s)@([^\s@]*)$/);
  if (!match) return null;
  const tail = text.slice(start).search(/[\s@]|$/);
  return {start: start - match[1].length - 1, end: start + tail, query: match[1].toLowerCase()};
}

export function prepareFiles(paths: string[]): MentionFile[] {
  return paths.map(path => ({path, lower: path.toLowerCase()})).sort((a, b) => a.path.length - b.path.length || a.path.localeCompare(b.path));
}

// Paths are already sorted by length; retain that order within each rank.
export function mentionChoices(files: MentionFile[], query: string, limit = 30): string[] {
  const buckets: string[][] = [[], [], []];
  for (const file of files) {
    const base = file.lower.slice(file.lower.lastIndexOf('/') + 1);
    const contiguous = file.lower.includes(query);
    let i = 0;
    for (const c of file.lower) if (c === query[i]) i++;
    if (i !== query.length) continue;
    const rank = base.includes(query) ? 0 : contiguous ? 1 : 2;
    if (buckets[rank].length < limit) buckets[rank].push(file.path);
    if (buckets[0].length === limit) break;
  }
  return buckets.flat().slice(0, limit);
}

export function insertMention(text: string, mention: Mention, path: string): string {
  return text.slice(0, mention.start) + '@' + path + text.slice(mention.end);
}
