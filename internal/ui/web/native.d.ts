// What the Hypercube desktop app adds to the page: a bridge that takes the
// page's messages and delivers the window's events. A page outside the app has
// none.

import type {TitleBar} from './app/types.ts';

declare global {
  // hypercube is the main window's bridge. The app calls onEvent with the name
  // and detail of each event it reports: hypercube-notification,
  // hypercube-shortcut, and hypercube-title-bar.
  var hypercube: {
    postMessage(message: object): void;
    onEvent(callback: (name: string, detail: unknown) => void): void;
  } | undefined;

  // hypercubeSetup is the waiting page's bridge to the app's background
  // service setup.
  var hypercubeSetup: {
    status(): Promise<SetupStatus>;
    // install runs the installer and returns its error, empty on success.
    install(): Promise<string>;
  } | undefined;

  interface SetupStatus {
    address: string;
    port: number;
    log: string;
    // bundle is the app that would run at login.
    bundle: string;
    service: string;
    uid: number;
    // action is how the user gets a backend on this address: 'headless' runs
    // it in a terminal, for a port the login service does not use; 'kickstart'
    // starts the loaded login service; 'move' asks to move a downloaded app
    // out of its temporary location before setup; 'setup' offers to install
    // the login service.
    action: 'headless' | 'kickstart' | 'move' | 'setup';
  }

  interface WindowEventMap {
    'hypercube-notification': CustomEvent<{session: string}>;
    'hypercube-shortcut': CustomEvent<{id: string}>;
    'hypercube-title-bar': CustomEvent<TitleBar>;
  }
}
