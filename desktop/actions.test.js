const {expect, test} = require('bun:test');
const {createActions} = require('./actions.js');

// forbidden fails the test if the action reaches the system.
const forbidden = () => { throw new Error('reached the system'); };
const system = {dialog: {showOpenDialog: forbidden}, shell: {openPath: forbidden, showItemInFolder: forbidden}};

test.each([
  [{type: 'open', app: 'folder', path: 'relative/dir'}],
  [{type: 'open', app: 'folder', path: ['/tmp']}],
  [{type: 'open', app: 'unknown', path: '/tmp'}],
  [{type: 'unknown'}],
  [undefined],
])('%j is refused without running anything', async request => {
  const act = createActions({...system, platform: 'darwin'});
  expect(await act(request, {})).not.toBe('');
});

test('Terminal is refused on Linux', async () => {
  const act = createActions({...system, platform: 'linux'});
  expect(await act({type: 'open', app: 'terminal', path: '/tmp'}, {})).toContain('not available');
});

test('the folder chooser is a sheet on the window and returns the folder or an empty string', async () => {
  const windows = [];
  const results = [{canceled: false, filePaths: ['/chosen']}, {canceled: true, filePaths: []}];
  const dialog = {showOpenDialog: async (win, options) => { windows.push([win, options.properties]); return results.shift(); }};
  const act = createActions({...system, dialog, platform: 'linux'});
  expect(await act({type: 'choose-folder'}, 'window')).toBe('/chosen');
  expect(await act({type: 'choose-folder'}, 'window')).toBe('');
  expect(windows).toEqual([['window', ['openDirectory', 'createDirectory']], ['window', ['openDirectory', 'createDirectory']]]);
});

test('Reveal and Folder use the shell', async () => {
  const calls = [];
  const shell = {openPath: async path => { calls.push(['openPath', path]); return ''; }, showItemInFolder: path => calls.push(['reveal', path])};
  const act = createActions({...system, shell, platform: 'linux'});
  expect(await act({type: 'open', app: 'folder', path: '/a b'}, {})).toBe('');
  expect(await act({type: 'open', app: 'reveal', path: '/a b/c.go'}, {})).toBe('');
  expect(calls).toEqual([['openPath', '/a b'], ['reveal', '/a b/c.go']]);
});
