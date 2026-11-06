// Starts the page: the store, appearance, data socket, config, file polling, desktop app events,
// badge, title bar, and App.
import {createRoot} from 'react-dom/client';
import {App} from './app/app.tsx';
import {fetchConfig} from './api.ts';
import {startAppearance} from './app/appearance.ts';
import {receivedConfig} from './app/config.ts';
import {startBadge} from './app/badge.ts';
import {startTitleBar} from './app/title-bar.ts';
import {startWindowEvents} from './app/window.ts';
import {setVisible} from './app/workbench.ts';
import {startViewStorage} from './app/view-storage.ts';
import {startDraftStorage} from './agent/draft-storage.ts';
import {startDataSocket} from './data-socket.ts';
import {startFilePolling} from './files/file-polling.ts';
import {startNativeNotifications} from './notifications/notifications.ts';
import {initialState} from './state.ts';
import {createStore} from './store.ts';
import {selectSession} from './sessions/selection.ts';

const selected = decodeURIComponent(location.hash.slice(1));
const store = createStore(initialState(selected, document.visibilityState === 'visible'));

startAppearance(store);
startViewStorage(store);
startDraftStorage(store);
if (selected) store.update(state => selectSession(state, selected));
document.addEventListener('visibilitychange', () => store.update(state => setVisible(state, document.visibilityState === 'visible')));
startWindowEvents();
startDataSocket(store);
fetchConfig(store).then(config => { if (config) store.update(state => receivedConfig(state, config)); });
startFilePolling(store);
startNativeNotifications(store);
startBadge(store);
startTitleBar(store);
// index.html holds the #app element.
createRoot(document.getElementById('app')!).render(<App store={store} />);
