// The attached session's Git changes and open diffs.

export type ChangeStatus = 'modified' | 'added' | 'deleted' | 'untracked';

// Change is an uncommitted file, by path relative to the session's working
// directory. Added and removed line counts are absent for
// a binary file.
export interface Change {
  path: string;
  status: ChangeStatus;
  added?: number;
  removed?: number;
}

// ChangesResult is the checkout's branch and its uncommitted files.
export interface ChangesResult {
  branch: string;
  files: Change[];
}

// ChangeTree holds each changed file's change and each folder's status.
export interface ChangeTree {
  files: Map<string, Change>;
  folders: Map<string, ChangeStatus>;
}

// Changes is the Git state the Files and Git panels show: the changes, a folder
// that no longer exists, or why the changes could not be read. It is null before
// the first read and for a folder outside a repository.
export type Changes = ChangesResult | {missing: true} | {error: string} | null;

// OpenDiff is a diff open in a tab: its patch, or the message shown in its place.
export interface OpenDiff {
  text: string | null;
  message: string | null;
}

// DiffRead is the result of reading a diff.
export type DiffRead = {text: string} | {message: string} | {error: string};

// DiffRow is a line of a diff's whole-file patch with its line number on each
// side: an added line has no old number, and a removed line no new one.
export type DiffRow =
  | {kind: 'context'; old: number; new: number; text: string}
  | {kind: 'removed'; old: number; new: null; text: string}
  | {kind: 'added'; old: null; new: number; text: string};
