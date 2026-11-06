const {afterEach, expect, test} = require('bun:test');
const {connect} = require('./connection.js');

// A backend stand-in. onOpen sends the connection its first frames.
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
        const connection = {ws};
        connections.push(connection);
        onOpen?.(connection);
      },
      message() {},
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

test('alerts are passed on', async () => {
  const {port} = backend(({ws}) => {
    ws.send(JSON.stringify({type: 'alert', session: 's', title: 'Needs you'}));
  });
  const alerts = [];
  connect(port, {open() {}, alert: alert => alerts.push(alert.title)});
  await until(() => alerts.length === 1);
  expect(alerts).toEqual(['Needs you']);
});

test('the app dials again after the connection ends', async () => {
  const {port, connections} = backend();
  let opened = 0;
  connect(port, {open: () => opened++, alert() {}});
  await until(() => connections.length === 1 && opened === 1);
  connections[0].ws.close();
  await until(() => connections.length === 2 && opened === 2);
});
