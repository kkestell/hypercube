// What calls for the user's attention outside the window: the menu bar icon
// and Dock badge with the number of sessions waiting, and a notification for
// each alert, silent unless the alert asks for a sound.
const {app, Menu, Notification, Tray, nativeImage} = require('electron');
const path = require('node:path');

// createAttention makes the menu bar icon, whose menu shows the window by
// calling show. clicked runs with the session of a notification the user
// clicks.
function createAttention({resourcesDir, show, clicked}) {
  const tray = new Tray(nativeImage.createFromPath(path.join(resourcesDir, 'trayTemplate.png')));
  tray.setToolTip('Hypercube');
  tray.setContextMenu(Menu.buildFromTemplate([
    {label: 'Show Hypercube', click: show},
    {type: 'separator'},
    {role: 'quit', label: 'Quit Hypercube'},
  ]));
  // A notification must be referenced until the user answers it.
  const open = new Set();

  return {
    setBadge(count) {
      const label = count ? String(count) : '';
      app.dock.setBadge(label);
      tray.setTitle(label);
      tray.setToolTip(count ? `Hypercube, ${count} waiting on you` : 'Hypercube');
    },
    notify({session, title, subtitle, text, sound}) {
      const notification = new Notification({title, subtitle, body: text, silent: !sound});
      open.add(notification);
      notification.on('click', () => clicked(session));
      notification.on('close', () => open.delete(notification));
      notification.show();
    },
  };
}

module.exports = {createAttention};
