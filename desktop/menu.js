// The application menu. The page supplies the App, Session, View, and Help
// items, each with its key bindings, and the menus own those keys wherever
// focus is in the window. An item's first binding shows in the menu; the others
// are hidden items. Zoom items are the app's; the others go to the page. macOS
// has the standard application menu; Linux has File, Edit, View, Session, and
// Help, with Settings and Quit in File and About in Help.
const {app, Menu} = require('electron');
const {accelerator} = require('./accelerator.js');

// setMenu installs the menu for platform, 'darwin' or 'linux', with the page's
// items. run is called with each item's ID when the user chooses it or presses
// one of its keys.
function setMenu(items, run, platform) {
  const section = name => items.filter(item => item.section === name).flatMap(item => item.bindings.map((binding, i) =>
    ({label: item.title, accelerator: accelerator(binding), visible: i === 0, click: () => run(item.id)})));
  const view = [
    ...section('View'),
    {type: 'separator'},
    {role: 'togglefullscreen'},
    {role: 'toggleDevTools'},
  ];
  const about = {label: `About ${app.name}`, click: () => run('about')};
  Menu.setApplicationMenu(Menu.buildFromTemplate(platform === 'darwin' ? [
    {role: 'appMenu', submenu: [
      about,
      {type: 'separator'},
      ...section('App'),
      {type: 'separator'},
      {role: 'services'},
      {type: 'separator'},
      {role: 'hide'},
      {role: 'hideOthers'},
      {role: 'unhide'},
      {type: 'separator'},
      {role: 'quit'},
    ]},
    {role: 'editMenu'},
    {label: 'Session', submenu: section('Session')},
    {label: 'View', submenu: view},
    {role: 'window', submenu: [
      {role: 'minimize'},
      {role: 'zoom'},
      {role: 'close'},
      {type: 'separator'},
      {role: 'front'},
    ]},
    {role: 'help', submenu: section('Help')},
  ] : [
    {label: 'File', submenu: [
      ...section('App'),
      {type: 'separator'},
      {role: 'quit'},
    ]},
    {role: 'editMenu'},
    {label: 'View', submenu: view},
    {label: 'Session', submenu: section('Session')},
    {role: 'help', submenu: [
      ...section('Help'),
      {type: 'separator'},
      about,
    ]},
  ]));
}

module.exports = {setMenu};
