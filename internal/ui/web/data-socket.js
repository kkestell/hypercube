// The /state socket. The server sends the workspaces and open requests, and
// the transcript of the session the page names in {select}. The page names
// its attached session when the socket opens and whenever it changes; after a
// reconnect the server sends the transcript whole again. A page reloads when
// the server serves a different browser build than the one it first saw.
import {receivedRequests} from './notifications/notifications.js';
import {receivedWorkspaces} from './sessions/selection.js';
import {receivedTranscript} from './transcript/transcripts.js';

export function startDataSocket(store) {
  let socket = null, build = null, announced = null;

  const announce = () => {
    const id = store.get().session?.id ?? '';
    if (socket?.readyState !== WebSocket.OPEN || id === announced) return;
    announced = id;
    socket.send(JSON.stringify({select: id}));
  };

  const receivedState = message => {
    if (build !== null && message.build !== build) {
      location.reload();
      return;
    }
    build = message.build;
    store.update(state => receivedRequests(receivedWorkspaces(state, message.workspaces), message.requests));
  };

  const received = message => {
    if (message.type === 'state') receivedState(message);
    else if (message.type === 'transcript') store.update(state => receivedTranscript(state, message));
  };

  const connect = () => {
    socket = new WebSocket(new URL('state', location.href.replace(/^http/, 'ws')));
    socket.onopen = () => {
      announced = null;
      announce();
    };
    socket.onmessage = e => received(JSON.parse(e.data));
    socket.onclose = () => {
      socket = null;
      setTimeout(connect, 1000);
    };
  };

  store.subscribe(announce);
  connect();
}
