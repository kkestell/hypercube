// Display forms of settings, using native model IDs.
import type {Settings} from './types.ts';

const properNames: Record<string, string | undefined> = {
  claude: 'Claude',
  opencode: 'OpenCode',
  pi: 'Pi',
};

// displayName names a harness by its ID.
export const displayName = (harness = '') => properNames[harness] ?? harness;

// settingsText describes settings in words, for hover text and screen readers.
export const settingsText = (s: Settings) =>
  `${displayName(s.harness)} ${s.model}, ${s.effort ? `effort ${s.effort}` : 'default effort'}`;

// sameSettings compares two sessions' settings.
export const sameSettings = (a: Settings, b: Settings) => a.harness === b.harness && a.model === b.model && a.effort === b.effort;
