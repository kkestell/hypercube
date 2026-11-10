// Makes the documentation screenshots: for each scene file, a light and a dark
// PNG of the real desktop window showing the real backend, in docs/images.
//
//   electron scripts/screenshots/main.js --dir=<folder> <scene files>
//
// The folder holds hypercube, its MCP bridge and holder, and the scripted
// harness plugin linked under each harness name. It receives the run's
// Electron profile and each scene's state, configuration, and repositories;
// `make screenshots` creates and deletes it. A scene's sessions play through
// the backend's own actions: the plugin answers each prompt with the turn
// the scene assigns to it.
const {app, BrowserWindow, ipcMain, nativeImage, nativeTheme, screen, session} = require('electron');
const {execFile, spawn} = require('node:child_process');
const fs = require('node:fs');
const http = require('node:http');
const net = require('node:net');
const path = require('node:path');
const {promisify} = require('node:util');
const {createMainWindow} = require('../../desktop/window.js');

const root = path.resolve(__dirname, '../..');
const fixtures = path.join(root, 'docs/screenshots/repos');
const images = path.join(root, 'docs/images');
const pagesDir = path.join(root, 'internal/ui/dist');
const run = promisify(execFile);

const dirFlag = process.argv.find(arg => arg.startsWith('--dir='));
const dir = dirFlag && path.resolve(dirFlag.slice('--dir='.length));
const sceneFiles = process.argv.slice(2).filter(arg => arg.endsWith('.json'));

// The page's own default workbench, which a scene's workbench overrides.
const defaultWorkbench = {
  sidebar: true, rightPanel: true, rightTab: 'files', wrap: false, fixedWidth: true,
  sizes: {sidebar: 290, rightPanel: 320, panel: 240, editor: 560}, sidebarTab: 'sessions',
  collapsedWorkspaces: {}, collapsedSessions: {}, collapsedPullRequestGroups: {}, sidebarScroll: {sessions: 0, archived: 0},
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
    await startGitHub();
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
  if (scene.waiting) return shootWaiting(scene, base, name);
  fs.mkdirSync(path.join(base, 'config/hypercube'), {recursive: true});
  // Terminal panels run zsh with a prompt naming only the directory, and keep
  // its history in the scene's folder.
  fs.mkdirSync(path.join(base, 'zsh'));
  fs.writeFileSync(path.join(base, 'zsh/.zshrc'), "PROMPT='%1~ %# '\nRPROMPT=''\n");
  // The published example configuration supplies the model titles; Pi has no
  // entry there, so the scene config adds one for the Pi sessions.
  const example = JSON.parse(fs.readFileSync(path.join(root, 'docs/examples/config.json'), 'utf8'));
  example.models.push({harness: 'pi', model: 'openai/gpt-5.2', title: 'GPT-5.2', efforts: ['low', 'medium', 'high'], default_effort: 'high'});
  fs.writeFileSync(path.join(base, 'config/hypercube/config.json'),
    JSON.stringify({...example, theme: 'light'}, null, 2));
  github.scene = scene;
  github.branches = new Map();
  writeGH(base, scene.github);
  console.log(`${name}: playing`);
  let backend = await startBackend(base, path.resolve(file));
  let window;
  try {
    // Parked sessions play first; quitting the backend then ends their
    // agents, and the rest play on a new backend.
    const ids = new Map();
    const workspaces = await addWorkspaces(scene, base, backend);
    await play(scene, workspaces, backend, ids, s => s.parked);
    if (ids.size > 0) {
      await backend.stop();
      backend = await startBackend(base, path.resolve(file));
    }
    await play(scene, workspaces, backend, ids, s => !s.parked);
    for (const [s, id] of ids) if (!s.unseen) await backend.post('session', {action: 'seen', id});
    if (scene.github) await showGitHub(scene, base, workspaces, backend, ids);
    window = createMainWindow({platform: process.platform, origin: backend.origin, pagesDir, userData: base});
    await capture(scene, ids, workspaces, backend, window, name);
  } finally {
    if (window) BrowserWindow.fromWebContents(window.contents).destroy();
    await backend.stop();
  }
}

// shootWaiting captures the waiting page that offers to set up the background
// service, answering its status request as a freshly installed macOS app's
// setup would.
async function shootWaiting(scene, base, name) {
  fs.mkdirSync(base);
  ipcMain.handle('setup-status', () => ({
    address: 'http://localhost:4280/', log: '~/.local/state/hypercube/ui.log', bundle: '~/Applications/Hypercube.app', action: 'setup', command: '',
  }));
  nativeTheme.themeSource = 'light';
  const window = createMainWindow({platform: process.platform, origin: 'http://localhost:4280', pagesDir, userData: base});
  const contents = window.contents;
  try {
    const win = place(scene, window);
    await window.showWaiting();
    const settled = async theme => {
      nativeTheme.themeSource = theme;
      await until(() => nativeTheme.themeSource === theme);
      await contents.insertCSS(still);
      await contents.executeJavaScript('document.fonts.ready.then(() => true)');
      await pageCondition(contents, `document.querySelector('main button') && document.documentElement.classList.contains('dark') === ${theme === 'dark'}`);
      await frames(contents);
    };
    await shot(scene, win, contents, name, 'light', settled);
    await shot(scene, win, contents, name, 'dark', settled);
  } finally {
    BrowserWindow.fromWebContents(contents).destroy();
    ipcMain.removeHandler('setup-status');
  }
}

// validate checks what the plugin and this driver rely on: every turn has
// items and an end, children have no children, item IDs are unique within a
// session, no prompt contains another, since the plugin finds a turn by
// the prompt its input contains, a parked session ends idle, has no view,
// and is not selected, and a terminal belongs to a selected session.
function validate(scene) {
  if (scene.waiting) return;
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
  for (const w of scene.workspaces) for (const s of w.sessions) {
    check(s, false);
    if (s.parked && (s.view || s.turns.at(-1).end !== 'idle')) throw new Error(`parked session "${s.turns[0].prompt}" needs an idle end and no view`);
  }
  const [w, s] = scene.select;
  if (s === undefined && scene.terminal) throw new Error('a terminal needs a selected session');
  if (w === undefined && !scene.github) throw new Error('the Pull Requests view needs github');
  if (!scene.github && scene.workspaces.some(w => w.pullRequests)) throw new Error('pull requests need github');
  if (s !== undefined && scene.workspaces[w].sessions[s].parked) throw new Error('the selected session cannot be parked');
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
    HOME: process.env.HOME, USER: process.env.USER, PATH: path.join(base, 'gh') + path.delimiter + process.env.PATH, LANG: process.env.LANG || 'en_US.UTF-8',
    TERM: process.env.TERM || 'xterm-256color', XDG_STATE_HOME: path.join(base, 'state'), XDG_CONFIG_HOME: path.join(base, 'config'),
    SHELL: '/bin/zsh', ZDOTDIR: path.join(base, 'zsh'),
    HYPERCUBE_SCENE: sceneFile, HYPERCUBE_GITHUB_API: github.api,
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

// addWorkspaces adds the scene's workspaces and returns their IDs in scene
// order.
async function addWorkspaces(scene, base, backend) {
  const workspaces = [];
  for (const w of scene.workspaces) {
    const repo = path.join(base, w.name);
    fs.cpSync(path.join(fixtures, w.repo), repo, {recursive: true});
    const git = (...args) => run('git', ['-c', 'user.name=Hypercube', '-c', 'user.email=hypercube@example.com', '-c', 'commit.gpgsign=false', ...args], {cwd: repo});
    await git('init', '-q', '-b', 'main');
    await git('add', '-A');
    await git('commit', '-q', '-m', 'Initial commit');
    if (scene.github) await addRemote(repo, w.repo, git);
    if (w.changes) await git('apply', path.join(fixtures, w.changes));
    workspaces.push(await (await backend.post('workspace', {action: 'new', path: repo})).text());
  }
  return workspaces;
}

// GitHub as the scenes see it: a gh signed in to one account in scenes that
// show GitHub and to none in the rest, and a GraphQL server answering for each
// fixture repository in docs/screenshots/repos/github.json and for the scene's
// own pull requests.
const githubFixtures = JSON.parse(fs.readFileSync(path.join(fixtures, 'github.json'), 'utf8'));
const github = {api: '', scene: null, branches: new Map()};

// writeGH writes the scene's stand-in gh, which the backend finds first on its
// PATH, signed in when the scene shows GitHub.
function writeGH(base, signedIn) {
  const hosts = signedIn ? `{"login":"${githubFixtures.account}","state":"success","active":true,"host":"github.com"}` : '';
  fs.mkdirSync(path.join(base, 'gh'), {recursive: true});
  fs.writeFileSync(path.join(base, 'gh/gh'), `#!/bin/sh
case "$1 $2" in
"auth status") echo '{"hosts":{"github.com":[${hosts}]}}' ;;
"auth token") echo token ;;
*) exit 1 ;;
esac
`, {mode: 0o755});
}

// startGitHub starts the GraphQL server.
async function startGitHub() {
  const server = http.createServer((request, response) => {
    let body = '';
    request.on('data', chunk => { body += chunk; });
    request.on('end', () => {
      response.setHeader('Content-Type', 'application/json');
      response.end(JSON.stringify(answer(JSON.parse(body).query)));
    });
  }).listen(0, '127.0.0.1');
  await new Promise(resolve => server.once('listening', resolve));
  server.unref();
  github.api = `http://127.0.0.1:${server.address().port}/graphql`;
}

// pullRequests lists the scene's pull requests with their repositories: each
// workspace's fixture's, then the workspace's own, then those in repositories
// no workspace holds.
function pullRequests(scene) {
  return [...scene.workspaces.flatMap(w => {
    const {repo, pullRequests: shared = []} = githubFixtures.repos[w.repo];
    return [...shared, ...(w.pullRequests ?? [])].map(pr => ({...pr, repo}));
  }), ...githubFixtures.elsewhere];
}

// answer answers the watcher's queries: the account can push to every
// repository, reads the scene's pull requests, and sees a failed required
// check on those marked requiredFailed.
function answer(query) {
  const {account} = githubFixtures;
  const prs = pullRequests(github.scene);
  const url = pr => `https://github.com/${pr.repo}/pull/${pr.number}`;
  if (query.includes('isRequired')) {
    const data = {};
    let alias, repo;
    for (const m of query.matchAll(/(r\d+):repository\(owner:"([^"]*)",name:"([^"]*)"\)|(p\d+):pullRequest\(number:(\d+)\)/g)) {
      if (m[1]) { [alias, repo] = [m[1], `${m[2]}/${m[3]}`]; data[alias] = {}; continue; }
      const failed = prs.some(pr => pr.repo === repo && String(pr.number) === m[5] && pr.requiredFailed);
      data[alias][m[4]] = {statusCheckRollup: {contexts: {nodes: failed ? [{conclusion: 'FAILURE', isRequired: true}] : []}}};
    }
    return {data};
  }
  if (!query.includes('involved:search')) return {data: {repository: {viewerPermission: 'WRITE'}}};
  const person = login => ({__typename: 'User', login});
  const node = pr => {
    const author = pr.author ?? account;
    const oid = '0'.repeat(40);
    // A reply waiting on the account: a thread it started, or any thread on
    // its own pull request, where someone else spoke last.
    const thread = {isResolved: false, first: {nodes: [{author: person(account), createdAt: '2026-11-02T15:00:00Z'}]},
      last: {nodes: [{author: person(pr.replier ?? 'tomas-hallberg-dev'), createdAt: '2026-11-02T16:00:00Z'}]}};
    return {number: pr.number, title: pr.title, url: url(pr), isDraft: !!pr.draft, isCrossRepository: false,
      headRefName: github.branches.get(pr.session) ?? pr.branch, headRefOid: oid, repository: {nameWithOwner: pr.repo},
      author: {login: author}, reviewDecision: pr.review ?? '', mergeStateStatus: pr.merge ?? 'BLOCKED',
      statusCheckRollup: pr.checks ? {state: pr.checks} : null, reviewRequests: {totalCount: pr.pendingReviews ?? 0},
      latestOpinionatedReviews: {nodes: pr.reviewed ? [{author: {login: account}, state: pr.reviewed, commit: {oid}}] : []},
      reviews: {nodes: []}, reviewThreads: {nodes: Array(pr.unanswered ?? 0).fill(thread)}, comments: {nodes: []}, commits: {nodes: []}};
  };
  return {data: {
    involved: {nodes: prs.map(node)},
    requested: {nodes: prs.filter(pr => pr.requested).map(node)},
    direct: {nodes: prs.filter(pr => pr.requested === 'user').map(pr => ({url: url(pr)}))},
  }};
}

// addRemote gives a workspace its fixture's github.com origin, which Git
// fetches from a local bare repository through url.insteadOf, and puts the
// fixture's behind count of commits there.
async function addRemote(repo, fixture, git) {
  const {repo: name, behind = 0} = githubFixtures.repos[fixture];
  const remote = `https://github.com/${name}.git`;
  const bare = repo + '.git';
  await run('git', ['clone', '-q', '--bare', repo, bare]);
  await git('remote', 'add', 'origin', remote);
  await git('config', `url.${bare}.insteadOf`, remote);
  let head = (await git('rev-parse', 'HEAD')).stdout.trim();
  for (let i = 1; i <= behind; i++) {
    head = (await git('commit-tree', 'HEAD^{tree}', '-p', head, '-m', `Upstream change ${i}`)).stdout.trim();
  }
  if (behind) await git('push', '-q', 'origin', `${head}:refs/heads/main`);
  await git('fetch', '-q', 'origin');
  await git('branch', '-q', '--set-upstream-to=origin/main');
}

// showGitHub names each session's pull request branch after the branch the
// session runs on, has the backend read GitHub again, and waits until every
// workspace shows its pull requests and every linked session its own.
async function showGitHub(scene, base, workspaces, backend, ids) {
  const {live, post} = backend;
  const linked = pullRequests(scene).filter(pr => pr.session);
  for (const pr of linked) {
    const scened = [...ids.keys()].find(s => s.turns[0].prompt === pr.session);
    const shown = scened && allSessions(live.state).find(s => s.id === ids.get(scened));
    if (!shown) throw new Error(`pull request #${pr.number} names no session "${pr.session}"`);
    const branch = (await run('git', ['branch', '--show-current'], {cwd: shown.dir})).stdout.trim();
    if (!branch || branch === 'main') throw new Error(`pull request #${pr.number} needs a worktree session`);
    github.branches.set(pr.session, branch);
  }
  for (const w of scene.workspaces) await post('workspace', {action: 'new', path: path.join(base, w.name)});
  const count = pullRequests(scene).length;
  await until(() => {
    const shownWorkspaces = live.state.workspaces.filter(w => workspaces.includes(w.id));
    return live.state.accounts[0]?.pullRequests.length === count
      && shownWorkspaces.every(w => w.github?.updated > 0 && !w.github.error)
      && linked.every(pr => allSessions(live.state).find(s => s.id === ids.get([...ids.keys()].find(x => x.turns[0].prompt === pr.session)))?.pullRequest?.branch === github.branches.get(pr.session));
  });
}

// play plays the turns of the top-level sessions that chosen selects, in
// scene order, a turn's children after it. It adds each scene session's ID to
// ids.
async function play(scene, workspaces, backend, ids, chosen) {
  const {live, post} = backend;

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

  for (const [i, w] of scene.workspaces.entries()) {
    for (const s of w.sessions.filter(chosen)) {
      const created = await (await post('session', {action: 'new', workspace: workspaces[i], ...s.settings, ...(s.worktree ? {worktree: '1'} : {})}, {text: s.turns[0].prompt})).json();
      await playTurns(created.session, s);
    }
  }
}

// Requests the page has in flight, and the paths of those it completed since
// it last loaded. The data socket stays open and is not counted.
const requests = {inflight: new Set(), completed: []};

// A session's editor view with no tabs.
const emptyView = {panel: false, tabs: {root: null, preview: null, dragging: null, nextId: 1}, activePane: null, expanded: {}, scroll: {}, folders: {}, files: {}};

// place sizes the window for the scene on a Retina display and shows it in
// front, with keys typed meanwhile kept from the page, and returns it.
function place(scene, window) {
  const win = BrowserWindow.fromWebContents(window.contents);
  // The window takes focus to draw its buttons active; keys typed meanwhile
  // must not reach the page.
  window.contents.on('before-input-event', event => event.preventDefault());
  const {width, height} = scene.size;
  win.setSize(width, height);
  win.center();
  const [shownWidth, shownHeight] = win.getSize();
  if (shownWidth !== width || shownHeight !== height) throw new Error(`the window is ${shownWidth}×${shownHeight}, not ${width}×${height}; use a larger display`);
  if (screen.getDisplayMatching(win.getBounds()).scaleFactor !== 2) throw new Error('the window must be on a Retina display, so images are twice the scene size');
  // A covered window stops painting, so it stays in front while the page
  // renders and is captured.
  window.show();
  return win;
}

const frames = contents => contents.executeJavaScript('new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve(true))))');

// shot waits until settled says the page shows the scene in theme, then
// captures the window to docs/images.
async function shot(scene, win, contents, name, theme, settled) {
  const {width, height} = scene.size;
  await settled(theme);
  const windowNumber = win.getMediaSourceId().split(':')[1];
  console.log(`${name}: capturing ${theme}`);
  const file = path.join(images, `${name}-${theme}.png`);
  // macOS can briefly capture a wider native frame after sizing the window.
  let image, wrongSize = false;
  do {
    if (!win.isFocused()) {
      const focused = new Promise(resolve => win.once('focus', resolve));
      app.focus({steal: true});
      win.focus();
      await focused;
    }
    await run('screencapture', ['-x', '-o', `-l${windowNumber}`, file]);
    image = nativeImage.createFromPath(file);
    const size = image.getSize();
    if (size.width !== 2 * width || size.height !== 2 * height) {
      if (wrongSize) throw new Error(`${name}-${theme}.png is ${size.width}×${size.height}, not twice ${width}×${height}`);
      wrongSize = true;
      continue;
    }
    if (win.isFocused()) break;
  } while (true);
  // The screen holds the window's last painted frame, which lags the page
  // when the window was covered; the image must show what the page drew.
  if (difference(image, await contents.capturePage()) > 8) {
    throw new Error(`${name}-${theme}.png does not show the page; keep the window in front while it is captured`);
  }
  await addShadow(contents, file);
}

async function capture(scene, ids, workspaces, backend, window, name) {
  const contents = window.contents;
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
  const win = place(scene, window);

  // The views are written from a page that does not save its own over them.
  // A selection of a workspace alone shows its dashboard, and no selection the
  // Pull Requests view.
  const [w, s] = scene.select;
  const top = s === undefined ? null : scene.workspaces[w].sessions[s];
  const selected = top ? ids.get(top) : w === undefined ? null : 'workspace:' + workspaces[w];
  const sessionViews = {};
  for (const [scened, id] of ids) if (scened.view) {
    backend.select(id);
    await until(() => hasItem([...backend.live.transcript.values()], scened.turns.at(-1).items.at(-1).id));
    const items = [...backend.live.transcript.values()];
    const expanded = {};
    for (const key of Object.keys(scened.view.expanded ?? {})) {
      const item = key === 'activity' ? items.find(item => item.kind === 'state' && item.state === 'working') : items.find(item => item.id.endsWith(':' + key));
      if (!item) throw new Error(`cannot expand ${key}`);
      expanded[key === 'activity' ? 'activity:' + item.id : item.id] = true;
    }
    sessionViews[id] = {panel: false, ...scened.view, expanded};
  }
  if (scene.terminal) sessionViews[selected] = {...(sessionViews[selected] ?? emptyView), panel: true};
  const merged = {...defaultWorkbench, ...scene.workbench};
  // The conversation takes two fifths of the space beside the sidebar and right
  // panel; editor panes take the rest.
  const available = scene.size.width - (merged.sidebar ? merged.sizes.sidebar : 0) - (merged.rightPanel ? merged.sizes.rightPanel : 0);
  merged.sizes = {...defaultWorkbench.sizes, editor: Math.round(available * 3 / 5), ...scene.workbench?.sizes};
  const views = {selected: null, workbench: merged, sessionViews, workspaceViews: {}};
  await contents.loadURL(backend.origin + '/version');
  await contents.executeJavaScript(`localStorage.setItem('hypercube.views', ${JSON.stringify(JSON.stringify(views))})`);

  // settled waits until the page shows the selected session with its open
  // tabs' content, or the selected workspace's dashboard, and whatever the
  // scene opens, the window has the theme, and nothing is loading.
  const workbench = views.workbench;
  const tabs = selected && workbench.rightPanel && workbench.rightTab === 'files' ? ['/files/tree?', '/changes?'] : [];
  const panes = node => node ? (node.tabs ? [node] : node.children.flatMap(panes)) : [];
  for (const pane of panes(top?.view?.tabs.root)) {
    const [kind, file] = [pane.active.slice(0, pane.active.indexOf(':')), pane.active.slice(pane.active.indexOf(':') + 1)];
    if (kind === 'file' || kind === 'diff') tabs.push(`/${kind}?${file}`);
  }
  // An error card is the one card that names no session. A view the page
  // rejects shows one and never makes the scene's requests, so the checks
  // look for it before waiting on them.
  const errorCard = `[...document.querySelectorAll('.notification-card')]
    .find(card => !card.querySelector('.notification-card-session'))?.textContent`;
  const checkError = async () => {
    const error = await contents.executeJavaScript(`${errorCard} ?? ''`);
    if (error) throw new Error(`the page shows an error: ${error}`);
  };
  const shown = top ? `document.querySelector('.transcript-list')?.textContent.includes(${JSON.stringify(top.turns[0].prompt)})`
    : selected ? `document.querySelector('[aria-label="Recent sessions"]')?.children.length === ${Math.min(10, scene.workspaces[w]?.sessions.length)}`
    : `document.querySelector('[aria-label="Pull requests across accounts"] ul')`;
  // The terminal's rows, and whether one is a prompt: the scene's zsh prompt
  // ends with % after the directory.
  const rows = `[...document.querySelectorAll('.terminal-panel .xterm-rows > div')].map(row => row.textContent.trimEnd())`;
  const prompt = `row => /^\\S+ %$/.test(row)`;
  let typed = false;
  const settled = async theme => {
    await contents.insertCSS(still);
    await contents.executeJavaScript('document.fonts.ready.then(() => true)');
    await pageCondition(contents, `${errorCard} || ${shown}`);
    await checkError();
    if (scene.comparison === 'branch') {
      await contents.executeJavaScript(`[...document.querySelectorAll('[role="tab"]')].find(tab => tab.textContent === 'Committed').dispatchEvent(new MouseEvent('mousedown', {bubbles: true, button: 0}))`);
      await pageCondition(contents, `document.querySelector('.git-panel')?.textContent.match(/ ahead of |Compared with/)`);
    }
    if (scene.openModelPicker) {
      await contents.executeJavaScript(`document.querySelector('button[aria-label^="Model:"]').click()`);
      await pageCondition(contents, `document.querySelector('[data-slot=popover-content] [role=option]')`);
    }
    if (scene.openSettings) {
      await contents.executeJavaScript(`document.querySelector('button[aria-label="Settings"]').click()`);
      await pageCondition(contents, `document.querySelector('nav[aria-label="Settings sections"]')`);
      await contents.executeJavaScript(`[...document.querySelectorAll('nav[aria-label="Settings sections"] button')].find(button => button.textContent === ${JSON.stringify(scene.openSettings)}).click()`);
      await pageCondition(contents, `document.querySelector('nav[aria-label="Settings sections"] [aria-current=page]')?.textContent === ${JSON.stringify(scene.openSettings)}
        && !document.querySelector('[role=dialog]').textContent.includes('Loading settings')`);
      // The dialog focuses its first section, whose ring would mark it chosen.
      await contents.executeJavaScript(`document.activeElement.blur()`);
    }
    if (scene.openSearch) {
      await contents.executeJavaScript(`document.querySelector('button[aria-label="Search sessions"]').click()`);
      await pageCondition(contents, `document.querySelector('[cmdk-input]')`);
      // React takes typing from the input's native value setter.
      await contents.executeJavaScript(`(() => {
        const input = document.querySelector('[cmdk-input]');
        Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(input, ${JSON.stringify(scene.openSearch)});
        input.dispatchEvent(new Event('input', {bubbles: true}));
      })()`);
      await pageCondition(contents, `document.querySelector('[cmdk-item]')`);
    }
    if (scene.terminal) {
      // The shell keeps its output when the page reloads, so the command runs
      // once; xterm.js takes typed text from its textarea's input events.
      await pageCondition(contents, `${rows}.some(${prompt})`);
      if (!typed) {
        typed = true;
        await contents.executeJavaScript(`document.querySelector('.terminal-panel .xterm-helper-textarea')
          .dispatchEvent(new InputEvent('input', {data: ${JSON.stringify(scene.terminal + '\r')}, inputType: 'insertText'}))`);
      }
      await pageCondition(contents, `(rows => {
        const ran = rows.findIndex(row => row.endsWith(${JSON.stringify(scene.terminal)}));
        return ran >= 0 && rows.slice(ran + 1).some(${prompt});
      })(${rows})`);
    }
    await until(() => nativeTheme.themeSource === theme && tabs.every(tab => requests.completed.includes(tab)) && requests.inflight.size === 0);
    await frames(contents);
    // File and diff highlighters run in workers after their text loads. Network
    // completion alone can leave the plain-text fallback in the capture.
    await pageCondition(contents, `[...document.querySelectorAll('.file-view-code, .diff-view-lines')]
      .every(view => view.querySelector('code span[style*="--shiki"]'))`);
    await frames(contents);
    await checkError();
  };

  requests.completed = [];
  await contents.loadURL(`${backend.origin}/${selected ? '#' + encodeURIComponent(selected) : ''}`);
  await shot(scene, win, contents, name, 'light', settled);

  const config = await (await fetch(backend.origin + '/config')).json();
  await backend.post('config', {}, {...config, theme: 'dark'});
  // Loading the same address would only follow its hash, so reload.
  requests.completed = [];
  const reloaded = new Promise(resolve => contents.once('did-finish-load', resolve));
  contents.reload();
  await reloaded;
  await shot(scene, win, contents, name, 'dark', settled);
}

// addShadow rewrites a capture on a transparent canvas with a drop shadow
// that follows the window's rounded corners.
async function addShadow(contents, file) {
  const png = fs.readFileSync(file).toString('base64');
  const shadowed = await contents.executeJavaScript(`new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () => {
      const pad = 96, drop = 24;
      const canvas = document.createElement('canvas');
      canvas.width = image.width + 2 * pad;
      canvas.height = image.height + 2 * pad;
      const context = canvas.getContext('2d');
      context.shadowColor = 'rgba(0, 0, 0, 0.35)';
      context.shadowBlur = 64;
      context.shadowOffsetY = drop;
      context.drawImage(image, pad, pad);
      resolve(canvas.toDataURL('image/png').split(',')[1]);
    };
    image.onerror = () => reject(new Error('could not load the capture'));
    image.src = 'data:image/png;base64,${png}';
  })`);
  fs.writeFileSync(file, Buffer.from(shadowed, 'base64'));
}

// difference is the mean difference of two images' color channels, from 0 to
// 255, compared at a small size so color management and the window's buttons
// barely count.
function difference(a, b) {
  const small = image => image.resize({width: 72, height: 45}).toBitmap();
  const [x, y] = [small(a), small(b)];
  let sum = 0, count = 0;
  for (let i = 0; i < x.length; i += 4) {
    for (let c = 0; c < 3; c++) sum += Math.abs(x[i + c] - y[i + c]);
    count += 3;
  }
  return sum / count;
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
