// The read-only file browser: a tree of the session's working directory, as in
// VS Code's Explorer, with VS Code's Seti file icons.

// ---- Stub file API. The real API replaces this object. -------------------
// list(dir) resolves to the entries of one directory, never .git, as
// {name, dir}; read(path) resolves to a file's text. Paths are relative to the
// working directory, which is ''.
const sample = {
  'go.mod': 'module example.com/hypercube\n\ngo 1.25\n\nrequire github.com/coder/websocket v1.8.13\n',
  'Makefile': 'BIN ?= bin/hypercube\n\n.PHONY: build check\n\nbuild:\n\tgo build -o $(BIN) ./cmd/hypercube\n\ncheck: build\n\tgo vet ./...\n\tgo test ./...\n',
  '.gitignore': 'bin/\nnode_modules/\n',
  'README.md': `# Hypercube

Hypercube runs teams of local coding agents.

## Getting started

1. Install the app.
2. Add a workspace.
3. Start a session and send the lead a prompt.

- Agents run as native harnesses.
- They message each other through an MCP server.
  - Nested items work too.

> Agents are trusted local processes.

## Harnesses

| Harness | Plugin | Status |
| :--- | :--- | ---: |
| Claude Code | \`plugins/claude\` | Ready |
| Codex | \`plugins/codex\` | Ready |
| OpenCode | \`plugins/opencode\` | Ready |

### Configuration

Edit \`~/.config/hypercube/config.json\`:

\`\`\`json
{"port": 8080, "leads": [{"name": "claude", "harness": "claude"}]}
\`\`\`

Start the app:

\`\`\`sh
make build && bin/hypercube serve --port 8080
\`\`\`

---

See [the docs](https://example.com/docs) for **more** _detail_.
`,
  'cmd/hypercube/main.go': `// Command hypercube runs the app.
package main

import (
	"fmt"
	"os"
)

// Version is set at build time.
var Version = "dev"

type server struct {
	port  int
	debug bool
}

func (s *server) run() error {
	if s.port == 0 {
		return fmt.Errorf("no port")
	}
	fmt.Printf("listening on :%d\\n", s.port)
	return nil
}

func main() {
	s := &server{port: 8080}
	if err := s.run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
`,
  'internal/ui/static/app.js': `// The browser view.
import {TabLayout} from './tabs.js';

const layout = new TabLayout();
let count = 0;

export async function load(url) {
  const res = await fetch(url);
  if (!res.ok) throw new Error(\`load failed: \${res.status}\`);
  return res.json();
}

document.addEventListener('click', e => {
  const id = e.target.closest('[data-tab]')?.dataset.tab;
  if (id) layout.open(id);
  count++;
});
`,
  'internal/ui/package.json': '{\n  "name": "hypercube-ui",\n  "private": true,\n  "dependencies": {\n    "marked": "18.1.2",\n    "shiki": "4.5.0"\n  }\n}\n',
  '.github/workflows/check.yml': 'name: check\non:\n  push:\n    branches: [main]\njobs:\n  check:\n    runs-on: macos-latest\n    steps:\n      - uses: actions/checkout@v4\n      - run: make check # every check\n',
  'scripts/dev': '#!/bin/sh\n# Runs an isolated development session.\nset -eu\n\nport=${PORT:-8090}\nif [ "$1" = up ]; then\n  echo "starting on $port"\n  exec bin/hypercube serve --port "$port"\nfi\n',
  'docs/configuration.md': '# Configuration\n\nHypercube reads `~/.config/hypercube/config.json`.\n',
  'internal/store/schema.go': schema(),
};

// schema is a long Go file of a few thousand lines.
function schema() {
  let text = '// Package store holds the generated schema.\npackage store\n';
  for (let i = 1; i <= 400; i++) {
    text += `
// Table${i} is generated table number ${i}.
type Table${i} struct {
	ID   int64  \`json:"id"\`
	Name string \`json:"name"\`
}
`;
  }
  return text;
}

export const files = {
  async list(dir) {
    const prefix = dir ? dir + '/' : '';
    const entries = new Map();
    for (const path of Object.keys(sample)) {
      if (!path.startsWith(prefix)) continue;
      const [name, ...rest] = path.slice(prefix.length).split('/');
      entries.set(name, {name, dir: rest.length > 0});
    }
    return [...entries.values()];
  },
  async read(path) {
    if (!(path in sample)) throw new Error('No such file: ' + path);
    return sample[path];
  },
};
// ---- End of stub file API. ------------------------------------------------

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
// each folder's entries once, when the folder first expands.
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
  async reset(shown) {
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
    const entries = await this.list(dir);
    if (generation !== this.generation) return;
    entries.sort((a, b) => b.dir - a.dir || collator.compare(a.name, b.name));
    this.children.set(dir, entries);
    this.render();
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
