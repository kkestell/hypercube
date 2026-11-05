import type {State} from '../state.ts';
import type {Config} from './types.ts';

// Share saved profiles with composers and repair a draft whose choice was removed.
// Existing sessions keep their stored selections.
export function receivedConfig(state: State, config: Config): State {
  const draft = state.unsent;
  const unsent = draft && !draft.sending && !config.profiles.some(p => p.name === draft.profile)
    ? {...draft, profile: config.default_profile}
    : draft;
  return {...state, config, unsent};
}
