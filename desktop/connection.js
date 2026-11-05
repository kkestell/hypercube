// The desktop connection: the backend's /desktop WebSocket, where the backend
// sends requests and alerts and the app replies to each request with its ID.
// The app dials again one second after a connection ends, so a backend that
// restarts or starts late is found.

// connect dials the backend on port and keeps dialing. open runs when a
// connection opens, alert for each alert, and request for each request, which
// returns the fields of its reply or throws its error message. A reply goes
// only to the connection its request came from, because the backend numbers
// requests again on each connection.
function connect(port, {open, alert, request}) {
  const dial = () => {
    const ws = new WebSocket(`ws://localhost:${port}/desktop`);
    ws.onopen = open;
    ws.onclose = () => setTimeout(dial, 1000);
    ws.onmessage = async ({data}) => {
      const message = JSON.parse(data);
      if (message.type === 'alert') return alert(message);
      let reply;
      try {
        reply = await request(message);
      } catch (err) {
        reply = {error: err?.message ?? String(err)};
      }
      if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({...reply, id: message.id}));
    };
  };
  dial();
}

module.exports = {connect};
