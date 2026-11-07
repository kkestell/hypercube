// The background service that keeps the Hypercube backend running, and the
// bundle layout and service manager it depends on: launchd on macOS, a systemd
// user service on Linux. The waiting page offers what status returns, and
// install runs the installer its button starts.
const {execFile} = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const service = 'com.github.kkestell.hypercube';
const defaultPort = 4280;

// run resolves with the outcome of a command, which fails with its error.
const run = (file, args, options) => new Promise(resolve => execFile(file, args, options, (error, stdout, stderr) => resolve({error, stdout, stderr})));

// createSetup returns the setup for platform, 'darwin' or 'linux', of the app
// whose executable is execPath:
//   service, defaultPort  the service's name and the port it serves
//   bundle        the app's directory
//   status(port)  what the waiting page offers: {action, command}
//   install(shell) runs the bundle's installer for the default port and the
//                 user's login shell, and returns its error, empty on success
function createSetup(platform, execPath = process.execPath) {
  const mac = platform === 'darwin';
  const bundle = mac ? path.resolve(execPath, '../../..') : path.dirname(execPath);
  const installer = path.join(bundle, mac ? 'Contents/Resources/install-service' : 'resources/install-service');

  // installed says the service manager has the service for the user.
  const installed = async () => mac
    ? (await run('launchctl', ['print', `gui/${process.getuid()}/${service}`])).error === null
    : (await run('systemctl', ['--user', 'show', '-p', 'LoadState', '--value', service])).stdout.trim() === 'loaded';
  const restart = mac ? `launchctl kickstart gui/${process.getuid()}/${service}` : `systemctl --user restart ${service}`;

  // The Linux service runs the installed bundle's hypercube, which only the
  // installer puts there.
  const installedBundle = () => {
    try {
      return fs.realpathSync(path.join(process.env.XDG_DATA_HOME || path.join(os.homedir(), '.local/share'), 'hypercube'));
    } catch {
      return '';
    }
  };

  // status is the action that gives the user a backend on port, and the
  // command it shows. A port other than the default is not the service's, and
  // the service is never replaced once installed. A downloaded macOS app runs
  // from a temporary location until it is moved, and the service must not name
  // that.
  async function status(port) {
    if (port !== defaultPort) return {action: 'headless', command: `hypercube ui ${port}`};
    if (await installed()) return {action: 'restart', command: restart};
    if (mac && bundle.includes('/AppTranslocation/')) return {action: 'move', command: ''};
    if (!mac && fs.realpathSync(bundle) !== installedBundle()) return {action: 'install', command: path.join(bundle, 'resources/install')};
    return {action: 'setup', command: ''};
  }

  async function install(shell) {
    const env = {...process.env, APP: bundle, SERVICE: service, PORT: String(defaultPort), SHELL: shell};
    const {error, stderr} = await run(installer, [], {env});
    return error ? stderr.trim() || error.message : '';
  }

  return {service, defaultPort, bundle, status, install};
}

module.exports = {createSetup};
