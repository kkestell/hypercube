// The main window: the Hypercube page, or the waiting page until the backend
// answers. The page draws the title bar's row, its tab bars, beside the
// window's buttons, and the window reports that row's size.
const {app, BrowserWindow, ipcMain, nativeTheme, screen, shell} = require('electron');
const fs = require('node:fs');
const path = require('node:path');
const dock = require('./dock.js');

// The buttons' position and the title bar row they sit in, in points. The row
// is as tall as a toolbar and centers the buttons; inset is the width the
// buttons take from the left, with the same margin on the right as the left.
const trafficLightPosition = {x: 19, y: 19};
const titleBar = {height: 52, inset: 98};

const external = url => ['http:', 'https:', 'mailto:'].includes(new URL(url).protocol) && shell.openExternal(url);

// savedBounds are the normal bounds of the last session when they fit a
// display.
function savedBounds(file) {
  try {
    const bounds = JSON.parse(fs.readFileSync(file, 'utf8'));
    const fits = ({x, y, width, height}) => screen.getAllDisplays().some(d => {
      const area = d.workArea;
      return x >= area.x && y >= area.y && x + width <= area.x + area.width && y + height <= area.y + area.height;
    });
    if (fits(bounds)) return bounds;
  } catch {}
  return {width: 1280, height: 800};
}

// createMainWindow makes the window, hidden until its first page is ready. The
// backend page is at origin; the waiting page and its sibling pages are in
// pagesDir. The window answers the page's title bar and theme messages and
// passes the page's other messages to onMessage.
function createMainWindow({origin, pagesDir, userData, onMessage}) {
  const boundsFile = path.join(userData, 'window.json');
  const save = () => fs.writeFileSync(boundsFile, JSON.stringify(win.getNormalBounds()));
  const win = new BrowserWindow({
    ...savedBounds(boundsFile),
    minWidth: 640,
    minHeight: 400,
    show: false,
    title: 'Hypercube',
    titleBarStyle: 'hidden',
    trafficLightPosition,
    webPreferences: {preload: path.join(__dirname, 'preload.js')},
  });
  const contents = win.webContents;

  let quitting = false;
  app.on('before-quit', () => { quitting = true; });
  // Closing the window hides it, so the service's window comes back as it was.
  win.on('close', e => {
    save();
    if (quitting) return;
    e.preventDefault();
    win.hide();
    dock.hideIfNoWindows();
  });
  win.on('resized', save);
  win.on('moved', save);
  win.once('ready-to-show', () => show());

  const send = (name, detail) => contents.send('event', name, detail);
  // reportTitleBar tells the page the title bar's size in its own pixels. A
  // full screen window has no title bar.
  const reportTitleBar = () => {
    const factor = contents.getZoomFactor();
    send('hypercube-title-bar', win.isFullScreen() ? {height: 0, inset: 0} : {height: titleBar.height / factor, inset: titleBar.inset / factor});
  };
  win.on('enter-full-screen', reportTitleBar);
  win.on('leave-full-screen', reportTitleBar);

  // Messages from the Hypercube page, which the window's bridge sends, until
  // the window is destroyed.
  const fromPage = event => event.senderFrame === contents.mainFrame && new URL(event.senderFrame.url).origin === origin;
  const received = (event, message) => {
    if (!fromPage(event)) return;
    switch (message.type) {
      case 'title-bar':
        reportTitleBar();
        break;
      // The Settings theme also sets the app's native appearance, which the
      // window's buttons, the menus, and the
      // waiting page follow.
      case 'theme':
        if (['system', 'light', 'dark'].includes(message.theme)) nativeTheme.themeSource = message.theme;
        break;
      default:
        onMessage?.(message);
    }
  };
  ipcMain.on('page', received);
  win.on('closed', () => ipcMain.removeListener('page', received));

  // Links away from the page open in the default browser.
  contents.on('will-navigate', (e, url) => {
    if (new URL(url).origin === origin) return;
    e.preventDefault();
    external(url);
  });
  contents.setWindowOpenHandler(({url}) => {
    external(url);
    return {action: 'deny'};
  });
  contents.on('render-process-gone', () => contents.reload());

  const show = () => {
    dock.show();
    if (win.isMinimized()) win.restore();
    win.show();
    app.focus({steal: true});
  };

  return {
    contents,
    send,
    show,
    isFocused: () => win.isVisible() && win.isFocused(),
    showWaiting: () => win.loadFile(path.join(pagesDir, 'waiting.html')),
    showBackend: () => win.loadURL(origin + '/'),
    // zoom changes the page's zoom level by delta, or resets it.
    zoom: delta => {
      contents.setZoomLevel(delta === 0 ? 0 : contents.getZoomLevel() + delta);
      reportTitleBar();
    },
  };
}

module.exports = {createMainWindow, external};
