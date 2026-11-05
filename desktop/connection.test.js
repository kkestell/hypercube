const {afterEach, expect, test} = require('bun:test');
const {connect} = require('./connection.js');

// A backend stand-in. Each connection is recorded with the frames it received;
// onOpen sends the connection its first frames.
const servers = [];
afterEach(() => servers.splice(0).forEach(s => s.stop(true)));
function backend(onOpen) {
  const connections = [];
  const server = Bun.serve({
    port: 0,
    hostname: '127.0.0.1',
    fetch: (req, srv) => srv.upgrade(req) ? undefined : new Response('no', {status: 400}),
    websocket: {
      open(ws) {
        const connection = {ws, received: []};
        ws.data = connection;
        connections.push(connection);
        onOpen?.(connection);
      },
      message: (ws, message) => ws.data.received.push(JSON.parse(message)),
    },
  });
  servers.push(server);
  return {port: server.port, connections};
}

// until waits for check, which polls.
async function until(check) {
  for (let i = 0; i < 300 && !check(); i++) await Bun.sleep(10);
  expect(check()).toBeTruthy();
}

test('a request gets a reply with its ID, an error reply when it throws, and alerts are passed on', async () => {
  const {port, connections} = backend(({ws}) => {
    ws.send(JSON.stringify({type: 'navigate', id: 1, session: 's', url: 'a'}));
    ws.send(JSON.stringify({type: 'eval', id: 2, session: 's', script: 'x'}));
    ws.send(JSON.stringify({type: 'alert', session: 's', title: 'Needs you'}));
  });
  const alerts = [];
  connect(port, {
    open() {},
    alert: alert => alerts.push(alert.title),
    request: async ({type}) => {
      if (type === 'eval') throw new Error('boom');
      return {url: 'https://a/', title: 'A'};
    },
  });
  await until(() => connections[0]?.received.length === 2);
  expect(connections[0].received).toEqual(expect.arrayContaining([{id: 1, url: 'https://a/', title: 'A'}, {id: 2, error: 'boom'}]));
  expect(alerts).toEqual(['Needs you']);
});

test('the app dials again after the connection ends and never sends the old connection\'s reply to the new one', async () => {
  let release;
  const slow = new Promise(resolve => { release = resolve; });
  const {port, connections} = backend(({ws}) => {
    // Only the first connection asks. Its request has ID 1, as the next
    // connection's first request would.
    if (connections.length === 1) ws.send(JSON.stringify({type: 'navigate', id: 1, session: 's', url: 'a'}));
  });
  let opened = 0;
  connect(port, {open: () => opened++, alert() {}, request: () => slow});
  await until(() => connections.length === 1 && opened === 1);
  connections[0].ws.close();
  await until(() => connections.length === 2 && opened === 2);
  release({url: 'late'});
  await Bun.sleep(100);
  expect(connections[1].received).toEqual([]);
});
