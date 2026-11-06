// Hypercube Desktop, the Electron app that shows the Hypercube backend: the
// window, menus, menu bar icon, notifications, and the agents' browsers. It
// connects to a backend running on its own, as a login service or in a
// terminal, and quitting it leaves that backend and its agents running.
const {app, ipcMain} = require('electron');
const os = require('node:os');
const path = require('node:path');
const {pathToFileURL} = require('node:url');
const {createAttention} = require('./attention.js');
const {createBrowsers} = require('./browser.js');
const {connect} = require('./connection.js');
const {setMenu} = require('./menu.js');
const setup = require('./setup.js');
const {createMainWindow} = require('./window.js');

const portFlag = process.argv.find(arg => arg.startsWith('--port='));
const port = portFlag ? Number(portFlag.slice('--port='.length)) : setup.defaultPort;
if (!Number.isInteger(port) || port < 1 || port > 65535) {
  console.error(`Hypercube Desktop: invalid ${portFlag}`);
  app.exit(2);
}

// The profile is in the backend's state directory, so a development backend's
// app has a profile and instance lock of its own.
const stateDir = path.join(process.env.XDG_STATE_HOME || path.join(os.homedir(), '.local/state'), 'hypercube');
app.setPath('userData', path.join(stateDir, 'chromium'));

if (app.requestSingleInstanceLock()) {
  app.whenReady().then(start);
} else {
  app.quit();
}

function start() {
  const origin = `http://localhost:${port}`;
  const pagesDir = path.join(process.resourcesPath, 'ui');
  const win = createMainWindow({origin, pagesDir, userData: app.getPath('userData'), onMessage});
  const attention = createAttention({
    resourcesDir: process.resourcesPath,
    show: win.show,
    clicked: session => {
      win.show();
      win.send('hypercube-notification', {session});
    },
  });

  // The menus run the page's shortcuts in its window; zoom and About are the
  // app's.
  const zooms = {zoomIn: 1, zoomOut: -1, zoomReset: 0};
  const run = id => {
    if (id === 'about') return showAbout(origin);
    if (id in zooms) return win.zoom(zooms[id]);
    win.show();
    win.send('hypercube-shortcut', {id});
  };
  setMenu([], run);

  // The page's messages that are not the window's own.
  function onMessage(message) {
    switch (message.type) {
      case 'badge':
        if (Number.isInteger(message.count)) attention.setBadge(message.count);
        break;
      case 'shortcuts':
        if (Array.isArray(message.items)) setMenu(message.items, run);
        break;
    }
  }

  // The waiting page's setup requests.
  const waitingURL = pathToFileURL(path.join(pagesDir, 'waiting.html')).href;
  const fromWaitingPage = event => {
    if (event.senderFrame !== win.contents.mainFrame || event.senderFrame.url !== waitingURL) throw new Error('refused');
  };
  const bundle = path.resolve(process.execPath, '../../..');
  // action is what the waiting page offers. A port other than the default is
  // not the login service's, and the service is never replaced once loaded.
  // A downloaded app runs from a temporary location until it is moved, and the
  // login service must not name that.
  const action = async () => {
    if (port !== setup.defaultPort) return 'headless';
    if (await setup.loaded(process.getuid())) return 'kickstart';
    return bundle.includes('/AppTranslocation/') ? 'move' : 'setup';
  };
  ipcMain.handle('setup-status', async event => {
    fromWaitingPage(event);
    return {address: origin + '/', port, log: path.join(stateDir, 'ui.log'), bundle, service: setup.service, uid: process.getuid(), action: await action()};
  });
  ipcMain.handle('setup-install', async event => {
    fromWaitingPage(event);
    if (await action() !== 'setup') return 'The login service cannot be set up from here.';
    return setup.install(bundle, os.userInfo().shell);
  });

  win.showWaiting();
  let found = false;
  connect(port, {
    // The page is loaded once, when the backend first answers; it keeps its own
    // connection to the backend after that.
    open() {
      if (found) return;
      found = true;
      win.showBackend();
    },
    alert(alert) {
      if (!win.isFocused()) attention.notify(alert);
    },
    request: createBrowsers(pagesDir),
  });

  app.on('activate', win.show);
  app.on('second-instance', win.show);
}

// showAbout shows the standard About panel with the running backend's build,
// which can be newer than this app until the app restarts after a reinstall.
async function showAbout(origin) {
  const identity = await fetch(`${origin}/version`).then(response => response.text(), () => 'Hypercube unavailable');
  // "Hypercube v0.1.0 (commit)" shows as "Version 0.1.0 (commit)".
  const [, version, commit] = identity.trim().split(' ');
  app.setAboutPanelOptions({applicationVersion: version.replace(/^v/, ''), ...(commit && {version: commit.slice(1, -1)})});
  app.showAboutPanel();
}
