// Builds the browser view into the directory given as the only argument: the
// bundles of web/, the page, the Tailwind stylesheet, and the license of every
// package the bundle includes. Its output depends only on the sources and the
// locked dependencies, so make check can compare it with the committed dist/.
// Run `bun install --frozen-lockfile` before it: the bundler resolves packages
// from the node_modules that exists when this script starts.
import {cpSync, existsSync, mkdirSync, readdirSync, readFileSync, rmSync} from 'node:fs';
import {join, resolve} from 'node:path';

const root = import.meta.dir;
const outdir = process.argv[2] && resolve(process.argv[2]);
if (!outdir) {
  console.error('usage: bun build.ts <outdir>');
  process.exit(2);
}

// Another bun version can bundle differently, which would make dist/ look stale.
const {packageManager}: {packageManager: string} = JSON.parse(readFileSync(join(root, 'package.json'), 'utf8'));
const pinned = packageManager.replace(/^bun@/, '');
if (Bun.version !== pinned) {
  console.error(`internal/ui needs bun ${pinned}; this is bun ${Bun.version}`);
  process.exit(1);
}

rmSync(outdir, {recursive: true, force: true});
mkdirSync(outdir, {recursive: true});

// Bun.build resolves paths against the working directory, and the metafile
// names its inputs relative to it.
process.chdir(root);
const result = await Bun.build({
  entrypoints: ['web/main.tsx'],
  outdir,
  target: 'browser',
  minify: true,
  define: {'process.env.NODE_ENV': '"production"'},
  jsx: {development: false},
  splitting: true,
  sourcemap: 'none',
  metafile: true,
  naming: {entry: '[name].[ext]', chunk: '[name]-[hash].[ext]', asset: '[name]-[hash].[ext]'},
});
if (!result.success || !result.metafile) {
  for (const log of result.logs) console.error(log);
  process.exit(1);
}

const css = Bun.spawnSync([process.execPath, 'node_modules/@tailwindcss/cli/dist/index.mjs',
  '-i', 'web/base.css', '-o', join(outdir, 'styles.css'), '--minify'], {stdout: 'inherit', stderr: 'inherit'});
if (css.exitCode !== 0) process.exit(css.exitCode);
for (const file of ['index.html', 'favicon.svg']) cpSync(join('web', file), join(outdir, file));

// Every bundled package ships its license, including the upstream UI sources.
const licenses = new Map([
  ['shadcn-ui', 'web/components/ui'],
  ['after-dark', 'web/files/after-dark'],
  ['tailwindcss', 'node_modules/tailwindcss'],
  ['@tailwindcss/typography', 'node_modules/@tailwindcss/typography'],
  ['tw-animate-css', 'node_modules/tw-animate-css'],
]);
for (const input of Object.keys(result.metafile.inputs)) {
  const match = input.match(/^(.*node_modules\/)((?:@[^/]+\/)?[^/]+)/);
  if (match) licenses.set(match[2], match[1] + match[2]);
}
for (const [name, packageDir] of [...licenses].sort(([a], [b]) => a.localeCompare(b))) {
  // A few Radix dependencies omit their license in the npm archive.
  const dir = existsSync(join('licenses', name)) ? join('licenses', name) : packageDir;
  const files = readdirSync(dir).filter(file => /^licen[cs]e/i.test(file));
  if (!files.length) {
    console.error(`${dir} has no license file`);
    process.exit(1);
  }
  mkdirSync(join(outdir, 'licenses', name), {recursive: true});
  for (const file of files) cpSync(join(dir, file), join(outdir, 'licenses', name, file));
}
