// Makes the documentation screenshots: for each scene file, a light and a dark
// PNG of the real desktop window showing the real backend, in docs/images.
//
//   electron scripts/screenshots/main.js --dir=<folder> <scene files>
//
// The folder holds hypercube and the scripted harness plugin, linked under
// each harness name, and receives the run's Electron profile and each scene's
// state, configuration, and repositories; `make screenshots` creates and
// deletes it. A scene's sessions play through the backend's own actions: the
// plugin answers each prompt with the turn the scene assigns to it.
const {app, BrowserWindow, nativeTheme, screen, session} = require('electron');
const {execFile, spawn} = require('node:child_process');
const fs = require('node:fs');
const net = require('node:net');
const path = require('node:path');
const {promisify} = require('node:util');
const {createMainWindow} = require('../../desktop/window.js');

const root = path.resolve(__dirname, '../..');
const fixtures = path.join(root, 'docs/screenshots/repos');
const images = path.join(root, 'docs/images');
const run = promisify(execFile);

const dirFlag = process.argv.find(arg => arg.startsWith('--dir='));
const dir = dirFlag && path.resolve(dirFlag.slice('--dir='.length));
const sceneFiles = process.argv.slice(2).filter(arg => arg.endsWith('.json'));

// The profiles the scenes name. They use the real harness names, so the page
// labels sessions as it does in real use.
const profiles = [
  {name: 'claude-opus-high', description: 'Opus at high effort for design and debugging.', harness: 'claude', model: 'opus', effort: 'high'},
  {name: 'claude-sonnet-high', description: 'Sonnet at high effort for ordinary implementation.', harness: 'claude', model: 'sonnet', effort: 'high'},
  {name: 'codex-gpt-terra-high', description: 'GPT Terra at high effort for implementation and review.', harness: 'codex', model: 'gpt-6.1-terra', effort: 'high'},
  {name: 'opencode-deepseek-flash-max', description: 'DeepSeek Flash at max effort for quick, cheap work.', harness: 'opencode', model: 'opencode-go/deepseek-v4.1-flash', effort: 'max'},
];

// The page's own default workbench, which a scene's workbench overrides.
const defaultWorkbench = {
  sidebar: true, rightPanel: true, panel: false, rightTab: 'files', wrap: false, fixedWidth: true,
  sizes: {sidebar: 260, rightPanel: 320, panel: 240}, sidebarTab: 'sessions',
  collapsedWorkspaces: {}, collapsedSessions: {}, sidebarScroll: {sessions: 0, archived: 0},
};

// Animations would make captures differ from run to run.
const still = '*, *::before, *::after { animation: none !important; transition: none !important; caret-color: transparent !important; }';

// Backends that are running, which a failure stops before the app exits.
const backends = new Set();

// fail ends the run in the terminal: never in Electron's error dialog.
async function fail(error) {
  console.error('screenshots:', error?.message ?? error);
  await Promise.all([...backends].map(backend => backend.stop()));
  app.exit(1);
}
process.on('uncaughtException', fail);
process.on('unhandledRejection', fail);
// Each scene destroys its window; the run ends after the last scene.
app.on('window-all-closed', () => {});

if (!dir || sceneFiles.length === 0) {
  console.error('usage: electron scripts/screenshots/main.js --dir=<folder> <scene files>');
  app.exit(2);
} else {
  app.setPath('userData', path.join(dir, 'chromium'));
  app.whenReady().then(async () => {
    fs.mkdirSync(images, {recursive: true});
    for (const file of sceneFiles) await shoot(file);
    app.quit();
  }).catch(fail);
}

// Waiting: the run's conditions are checked again whenever the backend, the
// page's requests, or the native theme change.
const waiters = new Set();
const notify = () => { for (const waiter of [...waiters]) waiter(); };
function until(check) {
  return new Promise((resolve, reject) => {
    const waiter = () => {
      try {
        const value = check();
        if (!value) return;
        waiters.delete(waiter);
        resolve(value);
      } catch (error) {
        waiters.delete(waiter);
        reject(error);
      }
    };
    waiters.add(waiter);
    waiter();
  });
}
nativeTheme.on('updated', notify);

async function shoot(file) {
  const name = path.basename(file, '.json');
  const scene = JSON.parse(fs.readFileSync(file, 'utf8'));
  validate(scene);
  const base = path.join(dir, name);
  fs.mkdirSync(path.join(base, 'config/hypercube'), {recursive: true});
  fs.writeFileSync(path.join(base, 'config/hypercube/config.json'),
    JSON.stringify({default_profile: profiles[0].name, theme: 'light', profiles}, null, 2));
  console.log(`${name}: playing`);
  const backend = await startBackend(base, path.resolve(file));
  let window;
  try {
    const ids = await play(scene, base, backend);
    window = createMainWindow({origin: backend.origin, pagesDir: path.join(root, 'internal/ui/dist'), userData: base});
    await capture(scene, ids, backend, window, name);
  } finally {
    if (window) BrowserWindow.fromWebContents(window.contents).destroy();
    await backend.stop();
  }
}

// validate checks what the plugin and this driver rely on: every turn has
// items and an end, children have no children, item IDs are unique within a
// session, and no prompt contains another, since the plugin finds a turn by
// the prompt its input contains.
function validate(scene) {
  const prompts = [];
  const check = (s, child) => {
    const ids = new Set();
    for (const t of s.turns) {
      prompts.push(t.prompt);
      if (!t.items?.length || !['idle', 'working', 'ask'].includes(t.end)) throw new Error(`turn "${t.prompt}" needs items and an end`);
      for (const item of t.items) {
        if (ids.has(item.id)) throw new Error(`item ${item.id} repeats in turn "${t.prompt}"`);
        ids.add(item.id);
      }
      if (child && t.children) throw new Error(`child "${s.title}" cannot have children`);
      for (const c of t.children ?? []) check(c, true);
    }
  };
  for (const w of scene.workspaces) for (const s of w.sessions) check(s, false);
  for (const [i, a] of prompts.entries()) {
    if (prompts.some((b, j) => i !== j && b.includes(a))) throw new Error(`prompt "${a}" is part of another prompt`);
  }
}

async function freePort() {
  const server = net.createServer().listen(0, '127.0.0.1');
  await new Promise(resolve => server.once('listening', resolve));
  const {port} = server.address();
  await new Promise(resolve => server.close(resolve));
  return port;
}

// startBackend runs `hypercube ui` with the scene's state and configuration
// and only the environment a fresh terminal gives it, and connects to its
// data socket. SIGINT stops it and ends its agents.
async function startBackend(base, sceneFile) {
  const port = await freePort();
  const origin = `http://localhost:${port}`;
  const env = {
    HOME: process.env.HOME, USER: process.env.USER, PATH: process.env.PATH, LANG: process.env.LANG || 'en_US.UTF-8',
    TERM: process.env.TERM || 'xterm-256color', XDG_STATE_HOME: path.join(base, 'state'), XDG_CONFIG_HOME: path.join(base, 'config'),
    HYPERCUBE_SCENE: sceneFile,
  };
  const child = spawn(path.join(dir, 'hypercube'), ['ui', String(port)], {env, stdio: ['ignore', 'pipe', 'inherit']});
  const exited = new Promise(resolve => child.once('close', resolve));
  await new Promise((resolve, reject) => {
    child.once('error', reject);
    child.once('exit', code => reject(new Error(`hypercube ui exited with ${code}`)));
    child.stdout.on('data', data => { if (String(data).includes('serving at')) resolve(); });
  });

  const live = {state: null, selected: null, transcript: new Map(), closed: false};
  const socket = new WebSocket(`ws://localhost:${port}/state`);
  socket.onmessage = event => {
    const message = JSON.parse(event.data);
    if (message.type === 'state') live.state = message;
    if (message.type === 'transcript' && message.session === live.selected) {
      if (message.reset) live.transcript = new Map();
      for (const item of message.items) live.transcript.set(item.id, item);
    }
    notify();
  };
  socket.onclose = () => { live.closed = true; notify(); };
  await until(() => live.state);

  const post = async (route, params, body) => {
    const response = await fetch(`${origin}/${route}?${new URLSearchParams(params)}`,
      {method: 'POST', headers: {Origin: origin}, body: body && JSON.stringify(body)});
    if (!response.ok) throw new Error(`${route} ${params.action ?? ''}: ${(await response.text()).trim()}`);
    return response;
  };
  const backend = {
    origin, live, post,
    select(id) {
      live.selected = id;
      live.transcript = new Map();
      socket.send(JSON.stringify({select: id}));
    },
    async stop() {
      backends.delete(backend);
      socket.close();
      child.kill('SIGINT');
      await exited;
    },
  };
  backends.add(backend);
  return backend;
}

const allSessions = state => state.workspaces.flatMap(w => w.sessions.flatMap(s => [s, ...s.children]));
const hasItem = (items, id) => items.some(item => item.id.endsWith(':' + id));

// play adds the scene's workspaces and plays every session's turns in order,
// a turn's children after it. It returns each scene session's ID.
async function play(scene, base, backend) {
  const {live, post} = backend;
  const ids = new Map();

  // turnPlayed reports whether the selected session has received the turn's
  // last item and reached the turn's end.
  const turnPlayed = (id, turn) => {
    if (live.closed) throw new Error('the backend closed its data socket');
    const shown = allSessions(live.state).find(s => s.id === id);
    if (!shown) return false;
    const failure = [...live.transcript.values()].find(item => item.kind === 'error');
    if (failure) throw new Error(`${shown.title}: ${failure.text}`);
    const last = turn.items.at(-1).id;
    if (!hasItem([...live.transcript.values()], last)) return false;
    if (turn.end === 'ask') return hasItem(live.state.requests.filter(r => r.session === id).map(r => r.item), last);
    return shown.activity === (turn.end === 'idle' ? 'idle' : 'busy');
  };

  const playTurns = async (id, s) => {
    ids.set(s, id);
    for (const [i, turn] of s.turns.entries()) {
      if (i > 0) await post('session', {action: 'input', id}, {text: turn.prompt});
      backend.select(id);
      await until(() => turnPlayed(id, turn));
      for (const c of turn.children ?? []) {
        const child = await until(() => allSessions(live.state).find(x => x.parent === id && x.title === c.title));
        await playTurns(child.id, c);
      }
    }
  };

  for (const w of scene.workspaces) {
    const repo = path.join(base, w.name);
    fs.cpSync(path.join(fixtures, w.repo), repo, {recursive: true});
    const git = (...args) => run('git', ['-c', 'user.name=Hypercube', '-c', 'user.email=hypercube@example.com', '-c', 'commit.gpgsign=false', ...args], {cwd: repo});
    await git('init', '-q', '-b', 'main');
    await git('add', '-A');
    await git('commit', '-q', '-m', 'Initial commit');
    if (w.changes) await git('apply', path.join(fixtures, w.changes));
    const workspace = await (await post('workspace', {action: 'new', path: repo})).text();
    for (const s of w.sessions) {
      const created = await (await post('session', {action: 'new', workspace, profile: s.profile}, {text: s.turns[0].prompt})).json();
      await playTurns(created.session, s);
    }
  }
  for (const [s, id] of ids) if (!s.unseen) await post('session', {action: 'seen', id});
  return ids;
}

// Requests the page has in flight, and the paths of those it completed since
// it last loaded. The data socket stays open and is not counted.
const requests = {inflight: new Set(), completed: []};

async function capture(scene, ids, backend, window, name) {
  const contents = window.contents;
  const win = BrowserWindow.fromWebContents(contents);
  const webRequest = session.defaultSession.webRequest;
  webRequest.onBeforeRequest((details, callback) => {
    if (details.resourceType !== 'webSocket') requests.inflight.add(details.id);
    callback({});
  });
  const finished = details => {
    if (!requests.inflight.delete(details.id)) return;
    const url = new URL(details.url);
    requests.completed.push(url.pathname + '?' + (url.searchParams.get('path') ?? ''));
    notify();
  };
  webRequest.onCompleted(finished);
  webRequest.onErrorOccurred(finished);

  const {width, height} = scene.size;
  win.setSize(width, height);
  win.center();
  const [shownWidth, shownHeight] = win.getSize();
  if (shownWidth !== width || shownHeight !== height) throw new Error(`the window is ${shownWidth}×${shownHeight}, not ${width}×${height}; use a larger display`);
  if (screen.getDisplayMatching(win.getBounds()).scaleFactor !== 2) throw new Error('the window must be on a Retina display, so images are twice the scene size');

  // The views are written from a page that does not save its own over them.
  const [w, s] = scene.select;
  const top = scene.workspaces[w].sessions[s];
  const selected = ids.get(top);
  const sessionViews = {};
  for (const [scened, id] of ids) if (scened.view) sessionViews[id] = scened.view;
  const views = {selected: null, workbench: {...defaultWorkbench, ...scene.workbench}, sessionViews};
  await contents.loadURL(backend.origin + '/version');
  await contents.executeJavaScript(`localStorage.setItem('hypercube.views', ${JSON.stringify(JSON.stringify(views))})`);

  // settled waits until the page shows the selected session with its open
  // tabs' content, the window has the theme, and nothing is loading.
  const tabs = [];
  const panes = node => node ? (node.tabs ? [node] : node.children.flatMap(panes)) : [];
  for (const pane of panes(top.view?.tabs.root)) {
    const [kind, file] = [pane.active.slice(0, pane.active.indexOf(':')), pane.active.slice(pane.active.indexOf(':') + 1)];
    if (kind === 'file' || kind === 'diff') tabs.push(`/${kind}?${file}`);
  }
  const settled = async theme => {
    await contents.insertCSS(still);
    await contents.executeJavaScript('document.fonts.ready.then(() => true)');
    await pageCondition(contents, `document.querySelector('.transcript-list')?.textContent.includes(${JSON.stringify(top.turns[0].prompt)})`);
    await until(() => nativeTheme.themeSource === theme && tabs.every(tab => requests.completed.includes(tab)) && requests.inflight.size === 0);
    await contents.executeJavaScript('new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve(true))))');
    // An error card is the one card that names no session.
    const error = await contents.executeJavaScript(`[...document.querySelectorAll('.notification-card')]
      .find(card => !card.querySelector('.notification-card-session'))?.textContent ?? ''`);
    if (error) throw new Error(`the page shows an error: ${error}`);
  };
  const shot = async theme => {
    await settled(theme);
    win.focus();
    const windowNumber = win.getMediaSourceId().split(':')[1];
    console.log(`${name}: capturing ${theme}`);
    await run('screencapture', ['-x', '-o', `-l${windowNumber}`, path.join(images, `${name}-${theme}.png`)]);
  };

  requests.completed = [];
  await contents.loadURL(`${backend.origin}/#${encodeURIComponent(selected)}`);
  await shot('light');

  const config = await (await fetch(backend.origin + '/config')).json();
  await backend.post('config', {}, {...config, theme: 'dark'});
  requests.completed = [];
  await contents.loadURL(`${backend.origin}/#${encodeURIComponent(selected)}`);
  await shot('dark');
}

// pageCondition waits until a page expression is true, checking it again as
// the page's document changes.
function pageCondition(contents, expression) {
  return contents.executeJavaScript(`new Promise(resolve => {
    const observer = new MutationObserver(() => check());
    const check = () => { if (${expression}) { observer.disconnect(); resolve(true); } };
    observer.observe(document, {subtree: true, childList: true, characterData: true, attributes: true});
    check();
  })`);
}
