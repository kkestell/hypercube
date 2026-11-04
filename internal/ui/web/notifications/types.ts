// The notifications the page keeps: open attention requests and its own errors.
import type {Session} from '../sessions/types.ts';
import type {AttentionItem} from '../transcript/types.ts';

// AttentionRequest is a session agent's open attention item.
export interface AttentionRequest {
  session: string;
  item: AttentionItem;
}

// PageError is an error shown to the user until closed.
export interface PageError {
  key: string;
  message: string;
}

// Card is one notification the page shows. A request or reply card names its
// session, that session's workspace, and its parent when it is a child.
export type Card =
  | {key: string; kind: 'error'; text: string}
  | {key: string; kind: 'request'; workspace: string; session: Session; parent: Session | null; item: AttentionItem}
  | {key: string; kind: 'reply'; workspace: string; session: Session; parent: Session | null; text: string};
