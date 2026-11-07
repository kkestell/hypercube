// Hypercube Desktop, the Electron app that shows the Hypercube backend: the
// window, menus, menu bar icon (macOS), and notifications. It connects to a
// backend running on its own, as a background service or in a terminal, and
// quitting it leaves that backend and its agents running.
const {app, ipcMain} = require('electron');
const os = require('node:os');
const path = require('node:path');
const {pathToFileURL} = require('node:url');
const {createAttention} = require('./attention.js');
const {connect} = require('./connection.js');
const {setMenu} = require('./menu.js');
const {createSetup} = require('./setup.js');
const {createMainWindow} = require('./window.js');

const platform = process.platform;
const setup = createSetup(platform);

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
  const win = createMainWindow({platform, origin, pagesDir, userData: app.getPath('userData'), onMessage});
  const attention = createAttention({
    platform,
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
    if (id === 'about') return showAbout(origin, platform);
    if (id in zooms) return win.zoom(zooms[id]);
    win.show();
    win.send('hypercube-shortcut', {id});
  };
  setMenu([], run, platform);

  // The page's messages that are not the window's own.
  function onMessage(message) {
    switch (message.type) {
      case 'badge':
        if (Number.isInteger(message.count)) attention.setBadge(message.count);
        break;
      case 'shortcuts':
        if (Array.isArray(message.items)) setMenu(message.items, run, platform);
        break;
    }
  }

  // The waiting page's setup requests.
  const waitingURL = pathToFileURL(path.join(pagesDir, 'waiting.html')).href;
  const fromWaitingPage = event => {
    if (event.senderFrame !== win.contents.mainFrame || event.senderFrame.url !== waitingURL) throw new Error('refused');
  };
  // action is what the waiting page offers; only setup runs the installer.
  ipcMain.handle('setup-status', async event => {
    fromWaitingPage(event);
    return {address: origin + '/', log: path.join(stateDir, 'ui.log'), bundle: setup.bundle, ...await setup.status(port)};
  });
  ipcMain.handle('setup-install', async event => {
    fromWaitingPage(event);
    if ((await setup.status(port)).action !== 'setup') return 'The background service cannot be set up from here.';
    return setup.install(os.userInfo().shell);
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
  });

  app.on('activate', win.show);
  app.on('second-instance', win.show);
}

// showAbout shows the standard About panel with the running backend's build,
// which can be newer than this app until the app restarts after a reinstall.
async function showAbout(origin, platform) {
  const identity = await fetch(`${origin}/version`).then(response => response.text(), () => 'Hypercube unavailable');
  // "Hypercube v0.1.0 (commit)" shows as "Version 0.1.0 (commit)". The Linux
  // panel has no field for the commit, so it joins the version.
  const [, version, commit] = identity.trim().split(' ');
  const number = version.replace(/^v/, '');
  const hash = commit?.slice(1, -1);
  app.setAboutPanelOptions(platform === 'darwin'
    ? {applicationVersion: number, ...(hash && {version: hash})}
    : {applicationVersion: hash ? `${number} (${hash})` : number, iconPath: path.join(process.resourcesPath, 'com.github.kkestell.hypercube.png')});
  app.showAboutPanel();
}
