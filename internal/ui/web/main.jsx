// Starts the page: the store, the data socket, file polling, the browsers'
// reports, the window's notification clicks, badge and title bar, and App.
import {createRoot} from 'react-dom/client';
import {App} from './app/app.jsx';
import {startAppearance} from './app/appearance.js';
import {startBadge} from './app/badge.js';
import {startTitleBar} from './app/title-bar.js';
import {setVisible} from './app/workbench.js';
import {startBrowsers} from './browser/browser.js';
import {startDraftStorage} from './agent/draft-storage.js';
import {startDataSocket} from './data-socket.js';
import {startFilePolling} from './files/file-polling.js';
import {startNativeNotifications} from './notifications/notifications.js';
import {createStore} from './store.js';

const store = createStore({
  workspaces: [],
  loaded: false,
  limits: [],
  requests: [],
  selected: decodeURIComponent(location.hash.slice(1)),
  pending: null,
  unsent: null,
  visible: document.visibilityState === 'visible',
  drafts: {},
  dismissed: {},
  errors: [],
  confirmation: null,
  workbench: {sidebar: true, rightPanel: true, panel: false, rightTab: 'files', wrap: localStorage.getItem('hypercube.wrap') === 'true'},
  session: null,
  browsers: {},
  fileOpens: {},
  overlays: {},
  titleBar: null,
});

startAppearance();
startDraftStorage(store);
let savedWrap = store.get().workbench.wrap;
store.subscribe(state => {
  if (state.workbench.wrap === savedWrap) return;
  savedWrap = state.workbench.wrap;
  localStorage.setItem('hypercube.wrap', String(savedWrap));
});
document.addEventListener('visibilitychange', () => store.update(state => setVisible(state, document.visibilityState === 'visible')));
startDataSocket(store);
startFilePolling(store);
startBrowsers(store);
startNativeNotifications(store);
startBadge(store);
startTitleBar(store);
createRoot(document.getElementById('app')).render(<App store={store} />);
