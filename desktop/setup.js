// The login service that keeps the Hypercube backend running: whether it is
// loaded, and the installer the waiting page's button runs.
const {execFile} = require('node:child_process');
const path = require('node:path');

const service = 'com.github.kkestell.hypercube';
const defaultPort = 4280;

// run resolves with the outcome of a command, which fails with its error.
const run = (file, args, options) => new Promise(resolve => execFile(file, args, options, (error, stdout, stderr) => resolve({error, stderr})));

// loaded says launchd has the login service for the user.
async function loaded(uid) {
  return (await run('launchctl', ['print', `gui/${uid}/${service}`])).error === null;
}

// install runs the bundle's installer for the bundle, the default port, and the
// user's login shell. It returns the installer's error, empty on success.
async function install(bundle, shell) {
  const env = {...process.env, APP: bundle, SERVICE: service, PORT: String(defaultPort), SHELL: shell};
  const {error, stderr} = await run(path.join(bundle, 'Contents/Resources/install-service'), [], {env});
  return error ? stderr.trim() || error.message : '';
}

module.exports = {service, defaultPort, loaded, install};
