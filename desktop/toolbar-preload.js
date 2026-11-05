// The browser toolbar's bridge to the browser window.
const {contextBridge, ipcRenderer} = require('electron');

contextBridge.exposeInMainWorld('hypercubeBrowser', {
  onState: callback => { ipcRenderer.on('state', (_, state) => callback(state)); },
  navigate: address => ipcRenderer.send('browser-navigate', address),
  focusSite: () => ipcRenderer.send('browser-focus'),
  back: () => ipcRenderer.send('browser-back'),
  forward: () => ipcRenderer.send('browser-forward'),
  reload: () => ipcRenderer.send('browser-reload'),
  openExternal: () => ipcRenderer.send('browser-external'),
});
