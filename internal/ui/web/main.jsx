// Starts the page: the store, the data socket, file polling, and App.
import './base.css';
import './elements.js';
import {render} from 'preact';
import {App} from './app/app.jsx';
import {setVisible} from './app/workbench.js';
import {startDataSocket} from './data-socket.js';
import {startFilePolling} from './files/file-polling.js';
import {createStore} from './store.js';

const store = createStore({
  workspaces: [],
  requests: [],
  selected: decodeURIComponent(location.hash.slice(1)),
  pending: null,
  unsent: null,
  visible: document.visibilityState === 'visible',
  drafts: {},
  dismissed: {},
  errors: [],
  confirmation: null,
  workbench: {sidebar: true, rightPanel: true, panel: false, rightTab: 'workers'},
  session: null,
});

document.addEventListener('visibilitychange', () => store.update(state => setVisible(state, document.visibilityState === 'visible')));
startDataSocket(store);
startFilePolling(store);
render(<App store={store} />, document.getElementById('app'));
