// Display forms of settings, using configured titles when available.
import type {ModelEntry, Settings} from './types.ts';

const properNames: Record<string, string | undefined> = {
  claude: 'Claude',
  opencode: 'OpenCode',
  pi: 'Pi',
};

// displayName names a harness by its ID.
export const displayName = (harness = '') => properNames[harness] ?? harness;

// settingsText describes settings in words, for hover text and screen readers.
export const settingsText = (s: Settings, entries: ModelEntry[] | null) =>
  `${displayName(s.harness)} ${modelTitle(s, entries)}${s.effort ? `, effort ${s.effort}` : ''}`;

// Unlisted models retain their native ID, including after an entry is deleted.
export const modelTitle = (s: Settings, entries: ModelEntry[] | null) =>
  entries?.find(e => e.harness === s.harness && e.model === s.model)?.title ?? s.model;

// sameSettings compares two sessions' settings.
export const sameSettings = (a: Settings, b: Settings) => a.harness === b.harness && a.model === b.model && a.effort === b.effort;
