// Starts the page: the store, the data socket, file polling, the browsers'
// reports, the window's notification clicks and title bar, and App.
import './base.css';
import './elements.js';
import {render} from 'preact';
import {App} from './app/app.jsx';
import {startBadge} from './app/badge.js';
import {startTitleBar} from './app/title-bar.js';
import {setVisible} from './app/workbench.js';
import {startBrowsers} from './browser/browser.js';
import {startDataSocket} from './data-socket.js';
import {startFilePolling} from './files/file-polling.js';
import {startNativeNotifications} from './notifications/notifications.js';
import {createStore} from './store.js';

const store = createStore({
  workspaces: [],
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
  workbench: {sidebar: true, rightPanel: true, panel: false, rightTab: 'files'},
  session: null,
  browsers: {},
  fileOpens: {},
  overlays: {},
  titleBar: null,
});

document.addEventListener('visibilitychange', () => store.update(state => setVisible(state, document.visibilityState === 'visible')));
startDataSocket(store);
startFilePolling(store);
startBrowsers(store);
startNativeNotifications(store);
startBadge(store);
startTitleBar(store);
render(<App store={store} />, document.getElementById('app'));
