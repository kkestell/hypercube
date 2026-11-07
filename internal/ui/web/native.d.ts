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
    // platform is the app's process.platform: 'darwin' on macOS, 'linux' on
    // Linux.
    platform: string;
    // chooseFolder shows the folder chooser and returns the folder, empty when
    // the user cancels.
    chooseFolder(): Promise<string>;
    // open opens an absolute path in app: 'vscode' and 'terminal' open a
    // folder in those applications, 'folder' opens a folder in the file
    // manager, and 'reveal' shows a file or folder in its parent folder.
    // Terminal is macOS only. It returns an error message, empty on success.
    open(app: 'vscode' | 'folder' | 'reveal' | 'terminal', path: string): Promise<string>;
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
    log: string;
    // bundle is the app that would run the service.
    bundle: string;
    // action is how the user gets a backend on this address: 'headless' runs
    // it in a terminal, for a port the service does not use; 'restart' starts
    // the installed service; 'move' asks to move a downloaded macOS app out of
    // its temporary location before setup; 'install' asks to install a Linux
    // bundle that is not the installed one; 'setup' offers to install the
    // service.
    action: 'headless' | 'restart' | 'move' | 'install' | 'setup';
    // command is what the user runs for 'headless', 'restart', and 'install';
    // empty otherwise.
    command: string;
  }

  interface WindowEventMap {
    'hypercube-notification': CustomEvent<{session: string}>;
    'hypercube-shortcut': CustomEvent<{id: string}>;
    'hypercube-title-bar': CustomEvent<TitleBar>;
  }
}
