// language returns the VS Code language ID of a file name, or '' for plain text.

const names: Record<string, string> = {
  makefile: 'makefile', gnumakefile: 'makefile', dockerfile: 'dockerfile', containerfile: 'dockerfile',
  '.bashrc': 'shellscript', '.zshrc': 'shellscript', '.profile': 'shellscript', 'tsconfig.json': 'jsonc', 'jsconfig.json': 'jsonc',
};

const extensions: Record<string, string> = {
  go: 'go', js: 'javascript', mjs: 'javascript', cjs: 'javascript', jsx: 'javascriptreact',
  ts: 'typescript', mts: 'typescript', cts: 'typescript', tsx: 'typescriptreact',
  json: 'json', jsonc: 'jsonc', jsonl: 'jsonl', yml: 'yaml', yaml: 'yaml', toml: 'toml',
  sh: 'shellscript', bash: 'shellscript', zsh: 'shellscript', py: 'python', pyi: 'python',
  html: 'html', htm: 'html', css: 'css', scss: 'scss', md: 'markdown', markdown: 'markdown',
  mk: 'makefile', dockerfile: 'dockerfile', rs: 'rust', c: 'c', i: 'c',
  cpp: 'cpp', cc: 'cpp', cxx: 'cpp', hpp: 'cpp', hh: 'cpp', hxx: 'cpp', h: 'cpp',
  java: 'java', swift: 'swift', sql: 'sql', xml: 'xml', svg: 'xml', plist: 'xml',
  diff: 'diff', patch: 'diff', ini: 'ini', ml: 'ocaml', mli: 'ocaml',
};

export const hasPreview = (name: string) => ['markdown', 'html'].includes(language(name)) || name.toLowerCase().endsWith('.svg');

export function language(name: string): string {
  const lower = name.toLowerCase();
  if (names[lower]) return names[lower];
  if (/^(dockerfile|containerfile)\./.test(lower)) return 'dockerfile';
  const dot = lower.lastIndexOf('.');
  return dot > 0 ? extensions[lower.slice(dot + 1)] ?? '' : '';
}
