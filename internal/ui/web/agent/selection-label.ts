// Display forms of a session's settings. A harness ID gets its proper
// spelling; a model shows the harness's label when the harness has reported
// one, and its native ID otherwise.
import type {Model, Settings} from './types.ts';

const properNames: Record<string, string | undefined> = {
  claude: 'Claude',
  codex: 'Codex',
  opencode: 'OpenCode',
  pi: 'Pi',
};

// displayName names a harness by its ID.
export const displayName = (harness = '') => properNames[harness] ?? harness;

// modelLabel is the model's label among the harness's models, or its ID.
export const modelLabel = (model: string, models: Model[] = []) => models.find(m => m.id === model)?.label || model;

// settingsText describes settings in words, for hover text and screen readers.
export const settingsText = (s: Settings, models?: Model[]) =>
  `${displayName(s.harness)} ${modelLabel(s.model, models)}, ${s.effort ? `effort ${s.effort}` : 'default effort'}`;

// sameSettings compares two sessions' settings.
export const sameSettings = (a: Settings, b: Settings) => a.harness === b.harness && a.model === b.model && a.effort === b.effort;
