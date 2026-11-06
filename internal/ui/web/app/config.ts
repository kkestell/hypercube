import {withDraftDefaults} from '../agent/drafts.ts';
import type {State} from '../state.ts';
import type {Config} from './types.ts';

// Share saved preferences with the page. A draft keeps its settings; one
// still without them takes the default.
export const receivedConfig = (state: State, config: Config): State => withDraftDefaults({...state, config});
