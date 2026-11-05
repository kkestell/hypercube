// The Dock icon follows the windows: it shows with the first and goes with
// the last, leaving the menu bar icon.
const {app, BaseWindow} = require('electron');

const show = () => app.dock.show();

function hideIfNoWindows() {
  if (!BaseWindow.getAllWindows().some(w => w.isVisible() || w.isMinimized())) app.dock.hide();
}

module.exports = {show, hideIfNoWindows};
