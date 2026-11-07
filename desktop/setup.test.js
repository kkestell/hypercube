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

// Redirect the installer's outputs and stub service commands. HOME stays intact,
// and the script can never reach the user's launchctl or lsof.
function isolatedInstaller() {
  const bin = temp();
  const calls = path.join(bin, 'calls');
  fs.writeFileSync(calls, '');
  for (const name of ['launchctl', 'lsof']) fs.writeFileSync(path.join(bin, name), `#!/bin/sh\necho ${name} "$@" >>"$TEST_CALLS"\n`, {mode: 0o755});
  const script = fs.readFileSync(path.join(__dirname, '../scripts/install-service'), 'utf8')
    .replace(/^PLIST=.*$/m, 'PLIST=$TEST_PLIST')
    .replace(/^LOG=.*$/m, 'LOG=$TEST_LOG');
  const installer = path.join(bin, 'install-service');
  fs.writeFileSync(installer, script, {mode: 0o755});
  const plist = path.join(temp(), 'service.plist');
  const log = path.join(temp(), 'Logs & <output>', 'ui.log');
  const env = {...process.env, PATH: `${bin}:/usr/bin:/bin`, APP: '/Apps/Research & <Development>/Hypercube.app',
    SERVICE: 'test.service&local', PORT: '4282', SHELL: '/shells/with & <names>/tcsh',
    TEST_CALLS: calls, TEST_PLIST: plist, TEST_LOG: log};
  return {installer, bin, env, plist, log, calls};
}

test('the login service preserves XML characters and runs the backend through the login shell', () => {
  const {installer, env, plist, log, calls} = isolatedInstaller();
  execFileSync(installer, {env});
  const read = key => execFileSync('plutil', ['-extract', key, 'raw', '-o', '-', plist]).toString().trim();
  expect(read('Label')).toBe(env.SERVICE);
  expect(read('Program')).toBe(env.SHELL);
  expect(read('ProgramArguments.0')).toBe('-tcsh');
  expect(read('ProgramArguments.1')).toBe('-i');
  expect(read('ProgramArguments.2')).toBe('-c');
  expect(read('ProgramArguments.3')).toBe('exec "$HYPERCUBE_APP_EXECUTABLE" ui "$HYPERCUBE_PORT"');
  expect(read('EnvironmentVariables.HYPERCUBE_APP_EXECUTABLE')).toBe(`${env.APP}/Contents/MacOS/hypercube`);
  expect(read('EnvironmentVariables.HYPERCUBE_PORT')).toBe('4282');
  expect(read('StandardOutPath')).toBe(log);
  expect(read('StandardErrorPath')).toBe(log);
  expect(fs.readFileSync(calls, 'utf8')).toContain(`launchctl bootstrap gui/${process.getuid()} ${plist}`);
});

test('a failed plist validation leaves the loaded service alone', () => {
  const {installer, bin, env, calls} = isolatedInstaller();
  fs.writeFileSync(path.join(bin, 'plutil'), '#!/bin/sh\nexit 1\n', {mode: 0o755});
  expect(() => execFileSync(installer, {env})).toThrow();
  expect(fs.readFileSync(calls, 'utf8')).toBe('');
});
