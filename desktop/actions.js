// The actions the page asks the app to run in the user's graphical session: the
// folder chooser, and opening a folder or file in another application. The page
// is trusted by window.js; the request's fields are still checked here.
const {spawn} = require('node:child_process');
const path = require('node:path');

// openWith opens target in a macOS application with `open -a`, which exits
// once the application has it, and resolves to open's error message, empty on
// success.
function openWith(application, target) {
  return new Promise(resolve => {
    const child = spawn('open', ['-a', application, target], {stdio: ['ignore', 'ignore', 'pipe']});
    let stderr = '';
    child.stderr.on('data', chunk => { stderr += chunk; });
    child.once('error', error => resolve(error.message));
    child.once('close', code => resolve(code === 0 ? '' : stderr.trim()));
  });
}

// startCode starts VS Code's code command on target, detached, and resolves to
// an error message when it cannot start, empty otherwise.
function startCode(target) {
  return new Promise(resolve => {
    const child = spawn('code', [target], {detached: true, stdio: 'ignore'});
    child.once('error', error => resolve(error.code === 'ENOENT' ? "VS Code's code command was not found." : error.message));
    child.once('spawn', () => {
      child.unref();
      resolve('');
    });
  });
}

// createActions returns the function that runs a request from the page, with
// the window the folder chooser belongs to. It resolves to the chosen folder,
// empty when the user cancels, for choose-folder, and otherwise to an error
// message, empty on success.
function createActions({dialog, shell, platform}) {
  const open = async ({app, path: target}) => {
    if (typeof target !== 'string' || !path.isAbsolute(target)) return 'The path is not absolute.';
    switch (app) {
      case 'folder':
        return shell.openPath(target);
      case 'reveal':
        shell.showItemInFolder(target);
        return '';
      case 'vscode':
        return platform === 'darwin' ? openWith('Visual Studio Code', target) : startCode(target);
      case 'terminal':
        return platform === 'darwin' ? openWith('Terminal', target) : 'Terminal is not available on Linux.';
      default:
        return 'Unknown application.';
    }
  };
  return async (request, win) => {
    switch (request?.type) {
      case 'choose-folder': {
        const {canceled, filePaths} = await dialog.showOpenDialog(win, {
          title: 'Add Workspace',
          buttonLabel: 'Add',
          properties: ['openDirectory', 'createDirectory'],
        });
        return canceled ? '' : filePaths[0];
      }
      case 'open':
        return open(request);
      default:
        return 'Unknown action.';
    }
  };
}

module.exports = {createActions};
