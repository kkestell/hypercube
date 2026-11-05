// The main window's bridge to the app, for the Hypercube page and the waiting
// page. The app accepts a page's messages only from the backend's page and the
// waiting page's setup requests only from the waiting page.
const {contextBridge, ipcRenderer} = require('electron');

contextBridge.exposeInMainWorld('hypercube', {
  postMessage: message => ipcRenderer.send('page', message),
  onEvent: callback => ipcRenderer.on('event', (_, name, detail) => callback(name, detail)),
});
contextBridge.exposeInMainWorld('hypercubeSetup', {
  status: () => ipcRenderer.invoke('setup-status'),
  install: () => ipcRenderer.invoke('setup-install'),
});
