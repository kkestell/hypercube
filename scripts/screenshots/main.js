// Makes the documentation screenshots: for each scene file, a light and a dark
// PNG of the desktop window in docs/images.
//
//   electron scripts/screenshots/main.js <scene files>
//
// A scene holds what the backend sends the page: its socket messages and HTTP
// responses. A server in this process serves the built page and answers from
// the scene. The window renders off screen at twice the scene size, and the
// driver draws its macOS frame.
const {app, BrowserWindow, ipcMain, nativeTheme} = require('electron');
const crypto = require('node:crypto');
const fs = require('node:fs');
const http = require('node:http');
const os = require('node:os');
const path = require('node:path');
const {createMainWindow} = require('../../desktop/window.js');

const root = path.resolve(__dirname, '../..');
const images = path.join(root, 'docs/images');
const pagesDir = path.join(root, 'internal/ui/dist');
const sceneFiles = process.argv.slice(2).filter(arg => arg.endsWith('.json'));
const offscreen = {deviceScaleFactor: 2};
// A step that takes longer than this has failed.
const timeout = 5000;

// Animations would make captures differ from run to run.
const still = '*, *::before, *::after { animation: none !important; transition: none !important; caret-color: transparent !important; }';

const profile = fs.mkdtempSync(path.join(os.tmpdir(), 'hypercube-screenshots-'));
const cleanUp = () => fs.rmSync(profile, {recursive: true, force: true});

// fail ends the run in the terminal: never in Electron's error dialog.
let failed = false;
function fail(error) {
  if (failed) return;
  failed = true;
  console.error('screenshots:', error?.message ?? error);
  cleanUp();
  app.exit(1);
}
process.on('uncaughtException', fail);
process.on('unhandledRejection', fail);
// The run ends after the last scene.
app.on('window-all-closed', () => {});

if (sceneFiles.length === 0) {
  console.error('usage: electron scripts/screenshots/main.js <scene files>');
  app.exit(2);
} else {
  app.setPath('userData', profile);
  app.whenReady().then(async () => {
    fs.mkdirSync(images, {recursive: true});
    const server = await serve();
    const window = createMainWindow({platform: 'darwin', origin: server.origin, pagesDir, userData: profile, offscreen});
    await window.contents.loadURL(server.origin + '/blank');
    window.contents.debugger.attach('1.3');
    for (const file of sceneFiles) await shoot(file, server, window);
    BrowserWindow.fromWebContents(window.contents).destroy();
    server.close();
    cleanUp();
    app.quit();
  }).catch(fail);
}

// serve starts the scene server: the built page's files, an empty page at
// /blank, and the current scene's responses and socket messages. Its config
// response carries the current theme. A request the scene does not answer
// fails the run.
async function serve() {
  const server = http.createServer((req, res) => {
    const url = new URL(req.url, 'http://localhost');
    const body = [];
    req.on('data', chunk => body.push(chunk));
    req.on('end', () => {
      try {
        answer(server, req.method, url, Buffer.concat(body).toString(), res);
      } catch (error) {
        fail(error);
      }
    });
  });
  server.on('upgrade', (req, socket) => {
    const url = new URL(req.url, 'http://localhost');
    const messages = server.scene.sockets[url.pathname + url.search];
    if (!messages) return fail(new Error(`${server.name} has no socket ${url.pathname + url.search}`));
    const accept = crypto.createHash('sha1').update(req.headers['sec-websocket-key'] + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').digest('base64');
    socket.write(`HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ${accept}\r\n\r\n`);
    for (const message of messages) socket.write(frame(message));
    socket.on('data', () => {});
    socket.on('error', () => {});
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  server.origin = `http://127.0.0.1:${server.address().port}`;
  return server;
}

const types = {'.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml'};

function answer(server, method, url, body, res) {
  if (method === 'GET' && url.pathname === '/blank') return res.end('<!doctype html>');
  const file = path.join(pagesDir, url.pathname === '/' ? 'index.html' : url.pathname);
  if (method === 'GET' && file.startsWith(pagesDir) && fs.existsSync(file) && fs.statSync(file).isFile()) {
    res.writeHead(200, {'Content-Type': types[path.extname(file)] ?? 'application/octet-stream'});
    return res.end(fs.readFileSync(file));
  }
  const key = `${method} ${url.pathname}${url.search}${body ? ' ' + body : ''}`;
  const response = server.scene.responses[key];
  // The page reports actions such as reading a session; they change nothing
  // a scene shows.
  if (!response && method === 'POST') return res.end();
  if (!response) throw new Error(`${server.name} has no response for ${key}`);
  if (response.json !== undefined) {
    const json = url.pathname === '/config' ? {...response.json, theme: server.theme} : response.json;
    res.writeHead(response.status, {'Content-Type': 'application/json'});
    return res.end(JSON.stringify(json));
  }
  res.writeHead(response.status, {'Content-Type': response.type});
  res.end(response.base64 !== undefined ? Buffer.from(response.base64, 'base64') : response.body);
}

// frame encodes a socket message as an unmasked WebSocket frame: JSON and
// text as text frames, base64 bytes as a binary frame.
function frame(message) {
  const binary = message.binary !== undefined;
  const payload = binary ? Buffer.from(message.binary, 'base64') : Buffer.from(message.json !== undefined ? JSON.stringify(message.json) : message.text);
  const n = payload.length;
  const header = n < 126 ? Buffer.from([0, n]) : n < 65536 ? Buffer.alloc(4) : Buffer.alloc(10);
  header[0] = binary ? 0x82 : 0x81;
  if (n >= 65536) {
    header[1] = 127;
    header.writeBigUInt64BE(BigInt(n), 2);
  } else if (n >= 126) {
    header[1] = 126;
    header.writeUInt16BE(n, 2);
  }
  return Buffer.concat([header, payload]);
}

async function shoot(file, server, window) {
  const name = path.basename(file, '.json');
  const scene = JSON.parse(fs.readFileSync(file, 'utf8'));
  place(scene, window);
  if (scene.waiting) return shootWaiting(scene, name, window);
  server.scene = scene;
  server.name = name;
  server.theme = 'light';
  const contents = window.contents;
  await freezeClock(contents, scene.now);
  await contents.loadURL(server.origin + '/blank');
  await contents.executeJavaScript(`localStorage.clear(); localStorage.setItem('hypercube.views', ${JSON.stringify(JSON.stringify(scene.views))})`);
  const address = server.origin + '/' + scene.address;
  await contents.loadURL(address);
  await shot(scene, contents, name, 'light', () => settled(scene, contents, 'light'));
  server.theme = 'dark';
  // A new session clears the hash, so the scene's address is put back
  // before the reload.
  await contents.executeJavaScript(`history.replaceState(null, '', ${JSON.stringify(address)})`);
  const reloaded = new Promise(resolve => contents.once('did-finish-load', resolve));
  contents.reload();
  await reloaded;
  await shot(scene, contents, name, 'dark', () => settled(scene, contents, 'dark'));
}

// shootWaiting captures the waiting page that offers to set up the background
// service, answering its status request as a freshly installed macOS app's
// setup would.
async function shootWaiting(scene, name, window) {
  ipcMain.handle('setup-status', () => ({
    address: 'http://localhost:4280/', log: '~/.local/state/hypercube/ui.log', bundle: '~/Applications/Hypercube.app', action: 'setup', command: '',
  }));
  const contents = window.contents;
  try {
    await window.showWaiting();
    const ready = theme => async () => {
      nativeTheme.themeSource = theme;
      await contents.insertCSS(still);
      await contents.executeJavaScript('document.fonts.ready.then(() => true)');
      await pageCondition(contents, `document.querySelector('main button') && document.documentElement.classList.contains('dark') === ${theme === 'dark'}`);
      await frames(contents);
    };
    await shot(scene, contents, name, 'light', ready('light'));
    await shot(scene, contents, name, 'dark', ready('dark'));
  } finally {
    nativeTheme.themeSource = 'system';
    ipcMain.removeHandler('setup-status');
  }
}

// freezeClock makes the page's clock read the scene's time from the next page
// load, so relative times read as they did when the scene was made.
let clock = null;
async function freezeClock(contents, now) {
  if (clock) await contents.debugger.sendCommand('Page.removeScriptToEvaluateOnNewDocument', {identifier: clock});
  ({identifier: clock} = await contents.debugger.sendCommand('Page.addScriptToEvaluateOnNewDocument', {source: `(() => {
    const Real = Date;
    globalThis.Date = class extends Real {
      constructor(...args) { super(...(args.length ? args : [${now}])); }
      static now() { return ${now}; }
    };
  })()`}));
}

// place sizes the window for the scene.
function place(scene, window) {
  const win = BrowserWindow.fromWebContents(window.contents);
  const {width, height} = scene.size;
  win.setSize(width, height);
  const [shownWidth, shownHeight] = win.getSize();
  if (shownWidth !== width || shownHeight !== height) throw new Error(`the window is ${shownWidth}×${shownHeight}, not ${width}×${height}`);
}

const frames = contents => contents.executeJavaScript('new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve(true))))');

// settled performs the scene's actions and waits until the page has the theme,
// has drawn the scene's data, and has highlighted its code.
async function settled(scene, contents, theme) {
  const actions = scene.actions ?? {};
  await contents.insertCSS(still);
  await contents.executeJavaScript('document.fonts.ready.then(() => true)');
  await pageCondition(contents, `document.documentElement.classList.contains('dark') === ${theme === 'dark'} && document.querySelector('main')`);
  if (actions.comparison === 'branch') {
    await contents.executeJavaScript(`[...document.querySelectorAll('[role="tab"]')].find(tab => tab.textContent === 'Committed').dispatchEvent(new MouseEvent('mousedown', {bubbles: true, button: 0}))`);
    await pageCondition(contents, `document.querySelector('.git-panel')?.textContent.match(/ ahead of |Compared with/)`);
  }
  if (actions.openModelPicker) {
    await contents.executeJavaScript(`document.querySelector('button[aria-label^="Model:"]').click()`);
    await pageCondition(contents, `document.querySelector('[data-slot=popover-content] [role=option]')`);
  }
  if (actions.openSettings) {
    await contents.executeJavaScript(`document.querySelector('button[aria-label="Settings"]').click()`);
    await pageCondition(contents, `document.querySelector('nav[aria-label="Settings sections"]')`);
    await contents.executeJavaScript(`[...document.querySelectorAll('nav[aria-label="Settings sections"] button')].find(button => button.textContent === ${JSON.stringify(actions.openSettings)}).click()`);
    await pageCondition(contents, `document.querySelector('nav[aria-label="Settings sections"] [aria-current=page]')?.textContent === ${JSON.stringify(actions.openSettings)}
      && !document.querySelector('[role=dialog]').textContent.includes('Loading settings')`);
    // The dialog focuses its first section, whose ring would mark it chosen.
    await contents.executeJavaScript(`document.activeElement.blur()`);
  }
  if (actions.newSession) {
    await pageCondition(contents, `document.querySelector('[data-workspace="${actions.newSession}"] button[aria-label="New Session"]')`);
    await contents.executeJavaScript(`document.querySelector('[data-workspace="${actions.newSession}"] button[aria-label="New Session"]').click()`);
    await pageCondition(contents, `document.querySelector('button[aria-label="Location"]')`);
  }
  if (actions.newBranch) {
    // Radix opens a menu on pointerdown and chooses an item on click.
    await contents.executeJavaScript(`document.querySelector('button[aria-label="Location"]').dispatchEvent(new PointerEvent('pointerdown', {bubbles: true, button: 0}))`);
    await pageCondition(contents, `document.querySelector('[role=menuitemradio]')`);
    await contents.executeJavaScript(`[...document.querySelectorAll('[role=menuitemradio]')].find(item => item.textContent.startsWith('New worktree on branch')).click()`);
    // The source button is enabled once the workspace's branches load.
    await pageCondition(contents, `document.querySelector('#new-branch-source:not(:disabled)')`);
    await type(contents, '#new-branch-name', actions.newBranch);
  }
  if (actions.sessionMenu) {
    const row = `document.querySelector('[data-session="${actions.sessionMenu.session}"]')`;
    await pageCondition(contents, row);
    await contents.executeJavaScript(`(() => {
      const row = ${row};
      const {left, bottom} = row.getBoundingClientRect();
      row.dispatchEvent(new MouseEvent('contextmenu', {bubbles: true, cancelable: true, clientX: left + 24, clientY: bottom}));
    })()`);
    await pageCondition(contents, `document.querySelector('.context-menu [role=menuitem]')`);
    await contents.executeJavaScript(`[...document.querySelectorAll('.context-menu [role=menuitem]')].find(item => item.textContent === ${JSON.stringify(actions.sessionMenu.item)}).click()`);
    await pageCondition(contents, `!document.querySelector('.context-menu')`);
  }
  if (actions.openSearch) {
    await contents.executeJavaScript(`document.querySelector('button[aria-label="Search sessions"]').click()`);
    await pageCondition(contents, `document.querySelector('[cmdk-input]')`);
    await type(contents, '[cmdk-input]', actions.openSearch);
    await pageCondition(contents, `document.querySelector('[cmdk-item]')`);
  }
  if (actions.terminal) {
    await pageCondition(contents, `[...document.querySelectorAll('.terminal-panel .xterm-rows > div')].some(row => row.textContent.trimEnd().endsWith(${JSON.stringify(actions.terminal)}))`);
  }
  await frames(contents);
  // File and diff highlighters run in workers after their text loads.
  await pageCondition(contents, `[...document.querySelectorAll('.file-view-code, .diff-view-lines')]
    .every(view => view.querySelector('code span[style*="--shiki"]'))`);
  await frames(contents);
  // An error card is the one card that names no session.
  const error = await contents.executeJavaScript(`[...document.querySelectorAll('.notification-card')]
    .find(card => !card.querySelector('.notification-card-session'))?.textContent ?? ''`);
  if (error) throw new Error(`the page shows an error: ${error}`);
}

// type sets an input's text as typing would: React takes it from the input's
// native value setter.
async function type(contents, selector, text) {
  await contents.executeJavaScript(`(() => {
    const input = document.querySelector(${JSON.stringify(selector)});
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(input, ${JSON.stringify(text)});
    input.dispatchEvent(new Event('input', {bubbles: true}));
  })()`);
  await pageCondition(contents, `document.querySelector(${JSON.stringify(selector)}).value === ${JSON.stringify(text)}`);
}

// shot waits until ready says the page shows the scene, then captures the
// page, framed as a macOS window, to docs/images.
async function shot(scene, contents, name, theme, ready) {
  const {width, height} = scene.size;
  await ready();
  console.log(`${name}: ${theme}`);
  const image = await contents.capturePage();
  const size = image.getSize();
  if (size.width !== 2 * width || size.height !== 2 * height) throw new Error(`${name}-${theme} is ${size.width}×${size.height}, not twice ${width}×${height}`);
  fs.writeFileSync(path.join(images, `${name}-${theme}.png`), await addFrame(contents, image.toPNG(), theme));
}

// addFrame returns a capture drawn as a macOS window: rounded corners, a
// hairline edge, the three window buttons where the window places them, and a
// drop shadow on a transparent margin. Sizes are in the capture's pixels,
// twice points.
async function addFrame(contents, png, theme) {
  const framed = await contents.executeJavaScript(`new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () => {
      const pad = 96, drop = 24, radius = 32;
      const dark = ${theme === 'dark'};
      const window = document.createElement('canvas');
      window.width = image.width;
      window.height = image.height;
      const w = window.getContext('2d');
      w.beginPath();
      w.roundRect(0, 0, image.width, image.height, radius);
      w.clip();
      w.drawImage(image, 0, 0);
      const buttons = dark
        ? [['#ff5c5f', '#e2302b'], ['#fac800', '#e0a800'], ['#34c759', '#1ea83c']]
        : [['#ff5c5f', '#e5403c'], ['#fac800', '#e3ad00'], ['#34c759', '#25ad48']];
      buttons.forEach(([fill, edge], i) => {
        w.beginPath();
        w.arc(52 + 46 * i, 52, 13.5, 0, 2 * Math.PI);
        w.fillStyle = fill;
        w.fill();
        w.lineWidth = 1;
        w.strokeStyle = edge;
        w.stroke();
      });
      w.beginPath();
      w.roundRect(0.5, 0.5, image.width - 1, image.height - 1, radius - 0.5);
      w.lineWidth = 1;
      w.strokeStyle = dark ? 'rgba(255, 255, 255, 0.16)' : 'rgba(0, 0, 0, 0.14)';
      w.stroke();
      const canvas = document.createElement('canvas');
      canvas.width = image.width + 2 * pad;
      canvas.height = image.height + 2 * pad;
      const context = canvas.getContext('2d');
      context.shadowColor = 'rgba(0, 0, 0, 0.35)';
      context.shadowBlur = 64;
      context.shadowOffsetY = drop;
      context.drawImage(window, pad, pad);
      resolve(canvas.toDataURL('image/png').split(',')[1]);
    };
    image.onerror = () => reject(new Error('could not load the capture'));
    image.src = 'data:image/png;base64,${png.toString('base64')}';
  })`);
  return Buffer.from(framed, 'base64');
}

// pageCondition waits until a page expression is true, checking it again as
// the page's document changes, and fails after the timeout.
function pageCondition(contents, expression) {
  return contents.executeJavaScript(`new Promise((resolve, reject) => {
    const observer = new MutationObserver(() => check());
    const timer = setTimeout(() => { observer.disconnect(); reject(new Error('timed out waiting for ' + ${JSON.stringify(expression.replace(/\s+/g, ' ').slice(0, 160))})); }, ${timeout});
    const check = () => { if (${expression}) { observer.disconnect(); clearTimeout(timer); resolve(true); } };
    observer.observe(document, {subtree: true, childList: true, characterData: true, attributes: true});
    check();
  })`);
}
