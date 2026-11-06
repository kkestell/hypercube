import type {State} from '../state.ts';
import type {Config} from './types.ts';
import type {Unsent} from '../sessions/types.ts';

// Share saved profiles with composers and repair a draft whose choice was removed.
// Existing sessions keep their stored selections.
export function receivedConfig(state: State, config: Config): State {
  const reconcile = (draft: Unsent) => !draft.sending && !config.profiles.some(p => p.name === draft.profile)
    ? {...draft, profile: config.default_profile} : draft;
  const unsent = state.unsent && reconcile(state.unsent);
  const unsentDrafts = Object.fromEntries(Object.entries(state.unsentDrafts).map(([key, draft]) => [key, reconcile(draft)]));
  return {...state, config, unsent, unsentDrafts};
}
