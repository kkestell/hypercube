// The page asks one question at a time, held in state.confirmation and drawn
// by ConfirmDialog.
import type {Store} from '../store.ts';
import type {Confirmation} from './types.ts';

// askToConfirm shows the dialog for {message, detail, primary, accept}, and
// runs accept if the user confirms.
export const askToConfirm = (store: Store, confirmation: Confirmation) => store.update(state => ({...state, confirmation}));
