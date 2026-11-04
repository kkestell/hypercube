// The attached session's files: the Files tree's listings and expansion and
// the files open in tabs, by path relative to the session's working directory.

// FileEntry is one entry of a folder listing.
export interface FileEntry {
  name: string;
  dir: boolean;
}

// LineRange highlights inclusive, one-based source lines.
export interface LineRange {
  start: number;
  end?: number;
}

// OpenFile is a file open in a tab. It holds its text, or the message shown in
// its place, and the ETag of the read that returned the text. source shows a
// previewable file as source; reveal scrolls its lines into view once.
export interface OpenFile {
  text: string | null;
  message: string | null;
  etag: string | null;
  deleted: boolean;
  source: boolean;
  lines: LineRange[];
  reveal: boolean;
}

// SessionFiles holds each listed folder's entries, null for a missing folder,
// the expanded folders, and the open files.
export interface SessionFiles {
  listings: Record<string, FileEntry[] | null>;
  expanded: Record<string, true>;
  open: Record<string, OpenFile>;
}

// FileRead is the result of reading an open file.
export type FileRead =
  | {unchanged: true}
  | {deleted: true}
  | {message: string}
  | {error: string}
  | {text: string; etag: string | null};

// SearchFile is a file whose lines match a search; each line's parts
// alternate between unmatched and matched text.
export interface SearchFile {
  path: string;
  lines: SearchLine[];
}

export interface SearchLine {
  line: number;
  parts: string[];
}
