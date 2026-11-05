// The agents' browsers: one window for each session, with a toolbar above the
// page the agent drives. The toolbar is a Hypercube page with its own bridge;
// the site's page has no bridge and shares one persistent profile with every
// other browser, so logins last.
const {BaseWindow, WebContentsView, ipcMain, session} = require('electron');
const path = require('node:path');
const vm = require('node:vm');
const dock = require('./dock.js');
const {external} = require('./window.js');
const {normalizeAddress} = require('./address.js');

const toolbarHeight = 44;
const partition = 'persist:browser';

const browsers = new Map(); // session ID -> Browser

class Browser {
  constructor(id, pagesDir) {
    this.id = id;
    const win = this.win = new BaseWindow({width: 1000, height: 720, minWidth: 500, minHeight: 300, show: false, title: `Browser — ${id}`});
    this.toolbar = new WebContentsView({webPreferences: {preload: path.join(__dirname, 'toolbar-preload.js')}});
    this.site = new WebContentsView({webPreferences: {partition}});
    win.contentView.addChildView(this.toolbar);
    win.contentView.addChildView(this.site);
    const layout = () => {
      const {width, height} = win.getContentBounds();
      this.toolbar.setBounds({x: 0, y: 0, width, height: toolbarHeight});
      this.site.setBounds({x: 0, y: toolbarHeight, width, height: height - toolbarHeight});
    };
    layout();
    win.on('resize', layout);
    this.toolbar.webContents.loadFile(path.join(pagesDir, 'toolbar.html'));

    const contents = this.site.webContents;
    // A script can run in a browser that has not navigated, once its blank
    // page has loaded.
    this.blank = contents.loadURL('about:blank');
    // A link that opens a window opens in the same page.
    contents.setWindowOpenHandler(({url}) => {
      contents.loadURL(url);
      return {action: 'deny'};
    });
    const update = () => {
      win.setTitle(`${contents.getTitle() || 'Browser'} — ${id}`);
      this.toolbar.webContents.send('state', {url: contents.getURL(), canGoBack: contents.navigationHistory.canGoBack(), canGoForward: contents.navigationHistory.canGoForward()});
    };
    for (const event of ['did-navigate', 'did-navigate-in-page', 'page-title-updated', 'did-finish-load']) contents.on(event, update);
    this.toolbar.webContents.on('did-finish-load', update);

    // closed fails the calls waiting on the browser when its window ends.
    this.closed = new Promise((_, reject) => { this.fail = reject; });
    this.closed.catch(() => {});
    win.on('closed', () => {
      this.fail(new Error('The browser was closed'));
      browsers.delete(id);
      this.toolbar.webContents.close();
      contents.close();
      dock.hideIfNoWindows();
    });
  }

  // until is what promise resolves to, unless the browser closes first.
  until(promise) {
    return Promise.race([promise, this.closed]);
  }

  async navigate(address) {
    const url = normalizeAddress(address);
    if (!url) throw new Error(`${address} is not a URL`);
    dock.show();
    this.win.showInactive();
    await this.until(this.blank);
    await this.until(this.site.webContents.loadURL(url));
    return {url: this.site.webContents.getURL(), title: this.site.webContents.getTitle()};
  }

  async eval(script) {
    const wrapped = `(async () => { const value = await (async () => {\n${script}\n})(); return JSON.stringify(value) ?? 'undefined'; })()`;
    // Electron reports a script that does not compile without saying why.
    new vm.Script(wrapped);
    await this.until(this.blank);
    return {result: await this.until(this.site.webContents.executeJavaScript(wrapped))};
  }
}

const browserOf = webContents => [...browsers.values()].find(b => b.toolbar.webContents === webContents);

// The toolbar's controls act on its browser's site.
function listen() {
  const on = (channel, act) => ipcMain.on(channel, (event, ...args) => {
    const browser = browserOf(event.sender);
    if (browser) act(browser, browser.site.webContents, ...args);
  });
  on('browser-navigate', (browser, site, address) => {
    const url = normalizeAddress(address);
    if (url) site.loadURL(url);
    site.focus();
  });
  on('browser-focus', (browser, site) => site.focus());
  on('browser-back', (browser, site) => site.navigationHistory.goBack());
  on('browser-forward', (browser, site) => site.navigationHistory.goForward());
  on('browser-reload', (browser, site) => site.reload());
  on('browser-external', (browser, site) => external(site.getURL()));
}

// createBrowsers makes the browsers' shared session deny the pages' permission
// requests, and answers the toolbars. pagesDir holds the toolbar page.
function createBrowsers(pagesDir) {
  const profile = session.fromPartition(partition);
  profile.setPermissionRequestHandler((_, __, allow) => allow(false));
  profile.setPermissionCheckHandler(() => false);
  listen();

  // request carries out a backend request, opening the session's browser if it
  // has none.
  const open = id => {
    if (!browsers.has(id)) browsers.set(id, new Browser(id, pagesDir));
    return browsers.get(id);
  };
  return async function request({type, session: id, url, script}) {
    switch (type) {
      case 'navigate':
        return open(id).navigate(url);
      case 'eval':
        return open(id).eval(script);
      case 'close':
        browsers.get(id)?.win.close();
        return {};
      default:
        throw new Error(`unknown request ${type}`);
    }
  };
}

module.exports = {createBrowsers};
