// What calls for the user's attention outside the window: the number of
// sessions waiting, as the Dock badge and menu bar icon on macOS and the
// launcher badge on Linux, and a notification for each alert, silent unless the
// alert asks for a sound.
const {app, Menu, Notification, Tray, nativeImage} = require('electron');
const path = require('node:path');

// createTray makes the macOS menu bar icon, whose menu shows the window by
// calling show.
function createTray(resourcesDir, show) {
  const tray = new Tray(nativeImage.createFromPath(path.join(resourcesDir, 'trayTemplate.png')));
  tray.setToolTip('Hypercube');
  tray.setContextMenu(Menu.buildFromTemplate([
    {label: 'Show Hypercube', click: show},
    {type: 'separator'},
    {role: 'quit', label: 'Quit Hypercube'},
  ]));
  return tray;
}

// createAttention makes the attention for platform, 'darwin' or 'linux', with
// the menu bar icon on macOS. clicked runs with the session of a notification
// the user clicks.
function createAttention({platform, resourcesDir, show, clicked}) {
  const mac = platform === 'darwin';
  const tray = mac ? createTray(resourcesDir, show) : null;
  // A notification must be referenced until the user answers it.
  const open = new Set();

  return {
    setBadge(count) {
      app.setBadgeCount(count);
      tray?.setTitle(count ? String(count) : '');
      tray?.setToolTip(count ? `Hypercube, ${count} waiting on you` : 'Hypercube');
    },
    // Linux notifications drop the subtitle, so it leads the body there.
    notify({session, title, subtitle, text, sound}) {
      const notification = new Notification(mac
        ? {title, subtitle, body: text, silent: !sound}
        : {title, body: [subtitle, text].filter(Boolean).join('\n'), silent: !sound});
      open.add(notification);
      notification.on('click', () => clicked(session));
      notification.on('close', () => open.delete(notification));
      notification.show();
    },
  };
}

module.exports = {createAttention};
