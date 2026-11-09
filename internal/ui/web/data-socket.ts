// The /state socket. The server sends workspaces, confirmation and file open
// requests, and the transcript of the session the page names in {select}. The page names
// its attached session when the socket opens and whenever it changes; after a
// reconnect the server sends the transcript whole again. A page reloads when
// the server serves a different browser build than the one it first saw.
import type {GitHubAccount} from './github/types.ts';
import type {Limit} from './app/types.ts';
import type {FileOpenRequest} from './editor/types.ts';
import {openPendingFiles, openRequestedFile} from './editor/editor.ts';
import type {AttentionRequest} from './notifications/types.ts';
import {receivedRequests} from './notifications/notifications.ts';
import {receivedWorkspaces} from './sessions/selection.ts';
import type {Workspace} from './sessions/types.ts';
import type {Store} from './store.ts';
import type {TranscriptItem} from './transcript/types.ts';
import {receivedTranscript} from './transcript/transcripts.ts';

export interface StateMessage {
  type: 'state';
  build: string;
  workspaces: Workspace[];
  requests: AttentionRequest[];
  limits: Limit[];
  accounts: GitHubAccount[];
}

// TranscriptMessage carries new and changed items of a session's transcript,
// or, with reset, the whole transcript.
export interface TranscriptMessage {
  type: 'transcript';
  session: string;
  reset: boolean;
  items: TranscriptItem[];
}

export interface OpenMessage extends FileOpenRequest {
  type: 'open';
}

export type ServerMessage = StateMessage | TranscriptMessage | OpenMessage;

export function startDataSocket(store: Store) {
  let socket: WebSocket | null = null;
  let build: string | null = null;
  let announced: string | null = null;

  const announce = () => {
    openPendingFiles(store);
    const id = store.get().session?.id ?? '';
    if (socket?.readyState !== WebSocket.OPEN || id === announced) return;
    announced = id;
    socket.send(JSON.stringify({select: id}));
  };

  const receivedState = (message: StateMessage) => {
    if (build !== null && message.build !== build) {
      location.reload();
      return;
    }
    build = message.build;
    store.update(state => ({...receivedRequests(receivedWorkspaces(state, message.workspaces), message.requests), limits: message.limits, accounts: message.accounts, loaded: true}));
  };

  const received = (message: ServerMessage) => {
    switch (message.type) {
      case 'open': return openRequestedFile(store, message);
      case 'state': return receivedState(message);
      case 'transcript': return store.update(state => receivedTranscript(state, message));
    }
  };

  const connect = () => {
    socket = new WebSocket(new URL('state', location.href.replace(/^http/, 'ws')));
    socket.onopen = () => {
      announced = null;
      announce();
    };
    socket.onmessage = (e: MessageEvent<string>) => received(JSON.parse(e.data));
    socket.onclose = () => {
      socket = null;
      setTimeout(connect, 1000);
    };
  };

  store.subscribe(announce);
  connect();
}
