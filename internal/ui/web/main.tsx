// Starts the page: the store, the data socket, file polling, the browsers'
// reports, the window's notification clicks, badge and title bar, and App.
import {createRoot} from 'react-dom/client';
import {App} from './app/app.tsx';
import {startAppearance} from './app/appearance.ts';
import {startBadge} from './app/badge.ts';
import {startTitleBar} from './app/title-bar.ts';
import {setVisible, startWrapStorage} from './app/workbench.ts';
import {startBrowsers} from './browser/browser.ts';
import {startDraftStorage} from './agent/draft-storage.ts';
import {startDataSocket} from './data-socket.ts';
import {startFilePolling} from './files/file-polling.ts';
import {startNativeNotifications} from './notifications/notifications.ts';
import {initialState} from './state.ts';
import {createStore} from './store.ts';

const store = createStore(initialState(decodeURIComponent(location.hash.slice(1)), document.visibilityState === 'visible'));

startAppearance();
startDraftStorage(store);
startWrapStorage(store);
document.addEventListener('visibilitychange', () => store.update(state => setVisible(state, document.visibilityState === 'visible')));
startDataSocket(store);
startFilePolling(store);
startBrowsers(store);
startNativeNotifications(store);
startBadge(store);
startTitleBar(store);
// index.html holds the #app element.
createRoot(document.getElementById('app')!).render(<App store={store} />);
