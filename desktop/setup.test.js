const {afterEach, expect, test} = require('bun:test');
const {execFileSync} = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {install} = require('./setup.js');

const dirs = [];
afterEach(() => dirs.splice(0).forEach(dir => fs.rmSync(dir, {recursive: true, force: true})));
const temp = () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'hypercube-desktop-'));
  dirs.push(dir);
  return dir;
};

// bundleWith is an app bundle whose installer is script.
function bundleWith(script) {
  const bundle = path.join(temp(), 'Hypercube.app');
  const installer = path.join(bundle, 'Contents/Resources/install-service');
  fs.mkdirSync(path.dirname(installer), {recursive: true});
  fs.writeFileSync(installer, script, {mode: 0o755});
  return bundle;
}

test('the installer runs for the bundle, default port, and login shell', async () => {
  const out = path.join(temp(), 'env');
  const bundle = bundleWith(`#!/bin/sh\nenv >${out}\n`);
  expect(await install(bundle, '/bin/tcsh')).toBe('');
  const env = fs.readFileSync(out, 'utf8').split('\n');
  expect(env).toContain(`APP=${bundle}`);
  expect(env).toContain('SERVICE=com.github.kkestell.hypercube');
  expect(env).toContain('PORT=4280');
  expect(env).toContain('SHELL=/bin/tcsh');
});

test('a failed installer reports its error', async () => {
  const bundle = bundleWith('#!/bin/sh\necho "port 4280 is in use" >&2\nexit 1\n');
  expect(await install(bundle, '/bin/zsh')).toBe('port 4280 is in use');
});

test('the login service runs the backend from the app through the login shell', () => {
  // A launchctl and lsof that report nothing and an HOME of the test's own
  // keep the installer from touching the real service.
  const home = temp();
  const bin = temp();
  const calls = path.join(bin, 'calls');
  for (const name of ['launchctl', 'lsof']) fs.writeFileSync(path.join(bin, name), `#!/bin/sh\necho ${name} "$@" >>${calls}\n`, {mode: 0o755});
  execFileSync(path.join(__dirname, '../scripts/install-service'), {
    env: {PATH: `${bin}:/usr/bin:/bin`, HOME: home, APP: '/Apps/Hypercube.app', SERVICE: 'test.service', PORT: '4282', SHELL: '/bin/tcsh'},
  });
  const plist = path.join(home, 'Library/LaunchAgents/test.service.plist');
  const read = key => execFileSync('plutil', ['-extract', key, 'raw', '-o', '-', plist]).toString().trim();
  expect(read('Program')).toBe('/bin/tcsh');
  expect(read('ProgramArguments.0')).toBe('-tcsh');
  expect(read('ProgramArguments.1')).toBe('-i');
  expect(read('ProgramArguments.2')).toBe('-c');
  expect(read('ProgramArguments.3')).toBe('exec "$HYPERCUBE_APP_EXECUTABLE" ui "$HYPERCUBE_PORT"');
  expect(read('EnvironmentVariables.HYPERCUBE_APP_EXECUTABLE')).toBe('/Apps/Hypercube.app/Contents/MacOS/hypercube');
  expect(read('EnvironmentVariables.HYPERCUBE_PORT')).toBe('4282');
  expect(fs.readFileSync(calls, 'utf8')).toContain(`launchctl bootstrap gui/${process.getuid()} ${plist}`);
});
