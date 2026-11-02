// The read-only file browser: a tree of the session's working directory, as in
// VS Code's Explorer, with VS Code's Seti file icons.

// Paths are relative to the session's working directory, which is ''.
export const files = {
  async list(session, dir) {
    const response = await fetch('/files?' + new URLSearchParams({session, path: dir}), {cache: 'no-store'});
    if (!response.ok) {
      const error = new Error(await response.text());
      error.status = response.status;
      throw error;
    }
    return response.json();
  },
  async read(session, path, etag) {
    const headers = etag ? {'If-None-Match': etag} : {};
    const response = await fetch('/file?' + new URLSearchParams({session, path}), {headers, cache: 'no-store'});
    if (response.status === 304) return {unchanged: true};
    if (response.status === 404) return {deleted: true};
    if (response.status === 413) return {message: 'The file is not displayed in the text editor because it is very large.'};
    if (response.status === 415) return {message: 'The file is not displayed in the text editor because it is either binary or uses an unsupported text encoding.'};
    if (!response.ok) return {message: await response.text()};
    return {text: await response.text(), etag: response.headers.get('ETag')};
  },
};

// language returns the VS Code language ID of a file name, or '' for plain text.
const languageNames = {makefile: 'makefile', gnumakefile: 'makefile', dockerfile: 'dockerfile', containerfile: 'dockerfile',
  '.bashrc': 'shellscript', '.zshrc': 'shellscript', '.profile': 'shellscript', 'tsconfig.json': 'jsonc', 'jsconfig.json': 'jsonc'};
const languageExtensions = {
  go: 'go', js: 'javascript', mjs: 'javascript', cjs: 'javascript', jsx: 'javascriptreact',
  ts: 'typescript', mts: 'typescript', cts: 'typescript', tsx: 'typescriptreact',
  json: 'json', jsonc: 'jsonc', jsonl: 'jsonl', yml: 'yaml', yaml: 'yaml', toml: 'toml',
  sh: 'shellscript', bash: 'shellscript', zsh: 'shellscript', py: 'python', pyi: 'python',
  html: 'html', htm: 'html', css: 'css', scss: 'scss', md: 'markdown', markdown: 'markdown',
  mk: 'makefile', dockerfile: 'dockerfile', rs: 'rust', c: 'c', i: 'c',
  cpp: 'cpp', cc: 'cpp', cxx: 'cpp', hpp: 'cpp', hh: 'cpp', hxx: 'cpp', h: 'cpp',
  java: 'java', swift: 'swift', sql: 'sql', xml: 'xml', svg: 'xml', plist: 'xml',
  diff: 'diff', patch: 'diff', ini: 'ini',
};
export function language(name) {
  const lower = name.toLowerCase();
  if (languageNames[lower]) return languageNames[lower];
  if (/^(dockerfile|containerfile)\./.test(lower)) return 'dockerfile';
  const dot = lower.lastIndexOf('.');
  return dot > 0 ? languageExtensions[lower.slice(dot + 1)] ?? '' : '';
}

// The Seti icon theme maps a file name, then its extensions from longest to
// shortest, then its language to a glyph of seti.woff and a color.
let theme;
export const seti = () => theme ??= fetch('vs-seti-icon-theme.json').then(res => res.json());

export function fileIcon(theme, name) {
  const lower = name.toLowerCase();
  let id = theme.fileNames[lower];
  for (let i = lower.indexOf('.'); !id && i >= 0; i = lower.indexOf('.', i + 1)) id = theme.fileExtensions[lower.slice(i + 1)];
  id ??= theme.languageIds[language(name)] ?? theme.file;
  const {fontCharacter, fontColor} = theme.iconDefinitions[id];
  return `<span class="seti-icon" style="color:${fontColor}">&#x${fontCharacter.slice(1)};</span>`;
}

const escape = text => text.replace(/[&<>"']/g, c => `&#${c.charCodeAt(0)};`);
const collator = new Intl.Collator(undefined, {numeric: true, sensitivity: 'base'});
const join = (dir, name) => dir ? dir + '/' + name : name;
const parent = path => path.slice(0, Math.max(0, path.lastIndexOf('/')));

// FileTree lists folders first, then files, each by name. Folders expand on
// click and keep the expansion of their subfolders while collapsed. It loads
// each folder's entries on first expand and refreshes visible folders on polling.
export class FileTree {
  constructor(root, {list, open, keepOpen}) {
    this.list = list;
    this.open = open;
    this.keepOpen = keepOpen;
    this.element = document.createElement('div');
    this.element.className = 'file-tree';
    this.element.setAttribute('role', 'tree');
    this.element.setAttribute('aria-label', 'Files');
    this.element.tabIndex = 0;
    root.append(this.element);
    this.element.onclick = e => {
      const row = e.target.closest('[data-path]');
      if (!row) return;
      // The second click of a double-click keeps a file open.
      if (e.detail === 2 && !this.rows.find(r => r.path === row.dataset.path).dir) this.keepOpen(row.dataset.path);
      else this.activate(row.dataset.path);
    };
    this.element.onkeydown = e => this.key(e);
    this.reset(false);
  }

  // reset forgets the tree and, when shown, lists the working directory.
  async reset(shown, list = this.list) {
    this.list = list;
    const generation = this.generation = (this.generation ?? 0) + 1;
    this.children = new Map();
    this.expanded = new Set();
    this.selected = null;
    this.rows = [];
    this.render();
    if (!shown) return;
    this.theme = await seti();
    if (generation === this.generation) await this.load('');
  }

  async load(dir) {
    const generation = this.generation;
    try {
      const entries = await this.list(dir);
      if (generation !== this.generation) return;
      entries.sort((a, b) => b.dir - a.dir || collator.compare(a.name, b.name));
      this.children.set(dir, entries);
      this.render();
    } catch {
      // An unreadable folder has no children on first expand.
    }
  }

  async refresh() {
    const generation = this.generation;
    let changed = false;
    const dirs = ['', ...this.rows.filter(row => row.dir && this.expanded.has(row.path)).map(row => row.path)];
    for (const dir of dirs) {
      try {
        const entries = await this.list(dir);
        if (generation !== this.generation) return;
        entries.sort((a, b) => b.dir - a.dir || collator.compare(a.name, b.name));
        if (JSON.stringify(entries) !== JSON.stringify(this.children.get(dir))) {
          this.children.set(dir, entries);
          changed = true;
        }
      } catch (error) {
        if (generation !== this.generation) return;
        if (error.status === 404) {
          changed = this.children.delete(dir) || changed;
          changed = this.expanded.delete(dir) || changed;
        }
      }
    }
    if (generation === this.generation && changed) this.render();
  }

  render() {
    this.rows = [];
    const walk = (dir, depth) => {
      for (const e of this.children.get(dir) ?? []) {
        const path = join(dir, e.name);
        this.rows.push({...e, path, depth});
        if (e.dir && this.expanded.has(path)) walk(path, depth + 1);
      }
    };
    walk('', 0);
    this.element.innerHTML = this.rows.map((r, i) => `<div role="treeitem" id="file-row-${i}" data-path="${escape(r.path)}" aria-level="${r.depth + 1}"
      ${r.dir ? `aria-expanded="${this.expanded.has(r.path)}"` : ''} aria-selected="${r.path === this.selected}" class="file-row" style="padding-left:${8 + r.depth * 8}px">
      ${r.dir ? '<span class="file-twistie"><span class="icon icon-chevron-right size-4"></span></span>' : fileIcon(this.theme, r.name)}
      <span class="file-name">${escape(r.name)}</span></div>`).join('');
    const index = this.rows.findIndex(r => r.path === this.selected);
    if (index < 0) this.element.removeAttribute('aria-activedescendant');
    else this.element.setAttribute('aria-activedescendant', 'file-row-' + index);
  }

  select(path) {
    this.selected = path;
    this.render();
    this.element.querySelector('[aria-selected="true"]')?.scrollIntoView({block: 'nearest'});
  }

  toggle(path, open = !this.expanded.has(path)) {
    if (open) this.expanded.add(path); else this.expanded.delete(path);
    this.render();
    if (open && !this.children.has(path)) this.load(path);
  }

  // activate selects a row; a folder toggles and a file opens.
  activate(path) {
    const row = this.rows.find(r => r.path === path);
    this.select(path);
    if (row.dir) this.toggle(path);
    else this.open(path);
  }

  // The keys are VS Code's list keys.
  key(e) {
    const index = this.rows.findIndex(r => r.path === this.selected), row = this.rows[index];
    const move = i => this.rows[i] && this.select(this.rows[i].path);
    if (e.key === 'ArrowDown') move(index < 0 ? 0 : index + 1);
    else if (e.key === 'ArrowUp') move(index < 0 ? 0 : index - 1);
    else if (e.key === 'Home') move(0);
    else if (e.key === 'End') move(this.rows.length - 1);
    else if (e.key === 'ArrowRight' && row?.dir) {
      if (!this.expanded.has(row.path)) this.toggle(row.path, true);
      else if (this.rows[index + 1]?.depth > row.depth) move(index + 1);
    } else if (e.key === 'ArrowLeft' && row) {
      if (row.dir && this.expanded.has(row.path)) this.toggle(row.path, false);
      else if (row.depth > 0) this.select(parent(row.path));
    } else if ((e.key === 'Enter' || e.key === ' ') && row) this.activate(row.path);
    else return;
    e.preventDefault();
  }
}
