// Builds the browser view into the directory given as the only argument: the
// bundles of web/, the page, the theme stylesheet, and the license of every
// package the bundle includes. Its output depends only on the sources and the
// locked dependencies, so make check can compare it with the committed dist/.
// Run `bun install --frozen-lockfile` before it: the bundler resolves packages
// from the node_modules that exists when this script starts.
import {cpSync, existsSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync} from 'node:fs';
import {basename, dirname, join, resolve} from 'node:path';

const root = import.meta.dir;
const outdir = process.argv[2] && resolve(process.argv[2]);
if (!outdir) {
  console.error('usage: bun build.js <outdir>');
  process.exit(2);
}

// Another bun version can bundle differently, which would make dist/ look stale.
const pinned = JSON.parse(readFileSync(join(root, 'package.json'), 'utf8')).packageManager.replace(/^bun@/, '');
if (Bun.version !== pinned) {
  console.error(`internal/ui needs bun ${pinned}; this is bun ${Bun.version}`);
  process.exit(1);
}

rmSync(outdir, {recursive: true, force: true});
mkdirSync(outdir, {recursive: true});

// Bun.build resolves paths against the working directory, and the metafile
// names its inputs relative to it.
process.chdir(root);
const inputs = new Set();
// A CSS entry in the split JavaScript build makes Bun emit an empty JS chunk.
for (const [entrypoint, splitting] of [['web/main.jsx', true], ['web/codicon.css', false]]) {
  const result = await Bun.build({
    entrypoints: [entrypoint],
    outdir,
    target: 'browser',
    minify: true,
    splitting,
    sourcemap: 'none',
    metafile: true,
    naming: {entry: '[name].[ext]', chunk: '[name]-[hash].[ext]', asset: '[name]-[hash].[ext]'},
  });
  if (!result.success) {
    for (const log of result.logs) console.error(log);
    process.exit(1);
  }
  for (const input of Object.keys(result.metafile.inputs)) inputs.add(input);
}

const {theme} = await import('@vscode-elements/webview-playground/dist/themes/dark-v2.js');
writeFileSync(join(outdir, 'theme.css'), `:root {\n${theme.map(([name, value]) => `  ${name}: ${value};\n`).join('')}}\n`);
for (const file of ['index.html', 'favicon.svg']) cpSync(join('web', file), join(outdir, file));

// Every bundled package ships its license, and so does the vendored Seti font.
const licenses = new Map([['@vscode-elements/webview-playground', 'node_modules/@vscode-elements/webview-playground']]);
for (const input of inputs) {
  const match = input.match(/^(.*node_modules\/)((?:@[^/]+\/)?[^/]+)/);
  if (match) licenses.set(match[2], match[1] + match[2]);
  else if (input.startsWith('web/') && existsSync(join(dirname(input), 'LICENSE'))) licenses.set(basename(dirname(input)), dirname(input));
}
for (const [name, dir] of [...licenses].sort(([a], [b]) => a.localeCompare(b))) {
  const files = readdirSync(dir).filter(file => /^licen[cs]e/i.test(file));
  if (!files.length) {
    console.error(`${dir} has no license file`);
    process.exit(1);
  }
  mkdirSync(join(outdir, 'licenses', name), {recursive: true});
  for (const file of files) cpSync(join(dir, file), join(outdir, 'licenses', name, file));
}
