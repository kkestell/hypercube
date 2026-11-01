// Renders an agent's transcript as DOM. Text from models and tools reaches the
// page only as text nodes and as attributes of elements this file creates, so
// it never becomes HTML. Markdown is parsed by marked into tokens, which are
// built into elements here; raw HTML in Markdown shows as text, and only links
// with http, https, or mailto addresses keep their address.

import {lexer} from 'marked';

// safeHref returns the address of a link the page may follow, or null.
export function safeHref(href) {
  try {
    const url = new URL(href);
    return ['http:', 'https:', 'mailto:'].includes(url.protocol) ? url.href : null;
  } catch {
    return null;
  }
}

const create = (doc, tag, className, ...children) => {
  const node = doc.createElement(tag);
  if (className) node.className = className;
  node.append(...children);
  return node;
};

function link(doc, href, children) {
  const safe = safeHref(href);
  if (!safe) return create(doc, 'span', '', ...children);
  const a = create(doc, 'a', '', ...children);
  a.setAttribute('href', safe);
  a.setAttribute('target', '_blank');
  a.setAttribute('rel', 'noopener noreferrer');
  return a;
}

// render builds the DOM nodes for marked tokens.
function render(doc, tokens = []) {
  return tokens.flatMap(t => {
    const kids = () => render(doc, t.tokens);
    switch (t.type) {
      case 'space':
      case 'def':
        return [];
      case 'paragraph': return [create(doc, 'p', '', ...kids())];
      case 'heading': return [create(doc, 'h' + t.depth, '', ...kids())];
      case 'code': return [create(doc, 'pre', '', create(doc, 'code', '', t.text))];
      case 'blockquote': return [create(doc, 'blockquote', '', ...kids())];
      case 'hr': return [create(doc, 'hr', '')];
      case 'list': {
        const list = create(doc, t.ordered ? 'ol' : 'ul', '', ...t.items.map(item => create(doc, 'li', '', ...render(doc, item.tokens))));
        if (t.ordered && t.start !== '' && t.start !== 1) list.setAttribute('start', t.start);
        return [list];
      }
      case 'checkbox': {
        const box = create(doc, 'input', '');
        box.type = 'checkbox';
        box.checked = t.checked;
        box.disabled = true;
        return [box, ' '];
      }
      case 'table': {
        const cell = (tag, c) => {
          const node = create(doc, tag, '', ...render(doc, c.tokens));
          if (c.align) node.setAttribute('style', 'text-align: ' + c.align);
          return node;
        };
        return [create(doc, 'table', '',
          create(doc, 'thead', '', create(doc, 'tr', '', ...t.header.map(c => cell('th', c)))),
          create(doc, 'tbody', '', ...t.rows.map(r => create(doc, 'tr', '', ...r.map(c => cell('td', c))))))];
      }
      case 'strong': return [create(doc, 'strong', '', ...kids())];
      case 'em': return [create(doc, 'em', '', ...kids())];
      case 'del': return [create(doc, 'del', '', ...kids())];
      case 'codespan': return [create(doc, 'code', '', t.text)];
      case 'br': return [create(doc, 'br', '')];
      case 'link': return [link(doc, t.href, kids())];
      case 'image': return [link(doc, t.href, [t.text || t.href])];
      case 'text': return t.tokens ? kids() : [doc.createTextNode(t.text)];
      case 'escape': return [doc.createTextNode(t.text)];
      default: return [doc.createTextNode(t.raw ?? '')];
    }
  });
}

// renderMarkdown returns the DOM nodes for Markdown text.
export function renderMarkdown(doc, text) {
  return render(doc, lexer(text));
}

// Tool input fields, in the order that best says what a call does.
const summaryKeys = ['command', 'cmd', 'file_path', 'path', 'pattern', 'query', 'url', 'description', 'prompt'];

// toolSummary is the one line that tells a tool call apart: the input's most
// telling text, shortened.
export function toolSummary(input) {
  let text = '';
  if (typeof input === 'string') {
    text = input;
  } else if (input && typeof input === 'object') {
    const key = summaryKeys.find(k => typeof input[k] === 'string') ?? Object.keys(input).find(k => typeof input[k] === 'string');
    text = key ? input[key] : '';
  }
  text = text.replace(/\s+/g, ' ').trim();
  return text.length > 120 ? text.slice(0, 119) + '…' : text;
}

// Hypercube's own tools, as what each call does and the input that tells
// calls apart. send_message shows as a message instead.
const hypercubeTools = {
  create_agent: input => ['Create agent', input.name],
  remove_agent: input => ['Remove agent', input.name],
  list_agents: () => ['List agents', ''],
  set_status: input => ['Set status', input.text],
};

// toolTitle is a tool call's name and summary.
export function toolTitle(item) {
  const input = item.input && typeof item.input === 'object' ? item.input : {};
  const named = item.server === 'hypercube' && hypercubeTools[item.tool];
  if (named) {
    const [name, detail] = named(input);
    return [name, toolSummary(typeof detail === 'string' ? detail : '')];
  }
  const name = item.server ? item.server + ' - ' + item.tool : item.tool ?? 'tool';
  return [name, toolSummary(item.input)];
}

// isMessage reports whether an item is a call that sends a message to an agent.
const isMessage = item => item.kind === 'tool' && item.server === 'hypercube' && item.tool === 'send_message';

// A message between agents longer than this many lines, or about as many lines
// of text, shows its first lines until it is expanded.
const clampLines = 6;
export const isLong = text => text.split('\n').length > clampLines || text.length > clampLines * 100;

// A tool's input and output show this many characters.
const shownLimit = 20000;
const clip = text => text.length > shownLimit ? text.slice(0, shownLimit) + '\n… (' + (text.length - shownLimit) + ' more characters)' : text;

function inputText(input) {
  if (input === undefined || input === null) return '';
  return clip(typeof input === 'string' ? input : JSON.stringify(input, null, 2));
}

// submitsOnKey reports whether a key press in the composer sends the message:
// Enter alone, outside of text composition.
export function submitsOnKey(e) {
  return e.key === 'Enter' && !e.shiftKey && !e.altKey && !e.ctrlKey && !e.metaKey && !e.isComposing;
}

const kinds = new Set(['user', 'reasoning', 'tool', 'error', 'attention']);

const kindOf = item => isMessage(item) ? 'message' : kinds.has(item.kind) ? item.kind : 'text';

const row = {
  user: 'flex justify-end',
  message: 'flex justify-start',
  text: 'md text-neutral-200',
  reasoning: 'text-neutral-500',
  tool: 'group min-w-0 border border-[#2b2b2b] bg-[#1e1e1e]',
  error: 'border border-red-500/50 bg-red-500/10 px-2 py-1.5 whitespace-pre-wrap break-words text-red-300',
  attention: 'border border-amber-400 bg-amber-400/10 px-2 py-1.5',
};

const summaryClass = 'flex cursor-pointer list-none items-center gap-1.5 px-2 py-1 text-neutral-400 hover:text-neutral-200 [&::-webkit-details-marker]:hidden';
const chevron = 'icon icon-chevron-right size-4 transition-transform group-open:rotate-90';
const bubble = 'flex max-w-[85%] min-w-0 flex-col gap-0.5 rounded bg-[#37373d] px-3 py-1.5 text-neutral-100';
const messageText = 'whitespace-pre-wrap break-words';
const button = 'rounded border border-[#454545] bg-[#2d2d30] px-2 py-1 text-neutral-200 hover:bg-[#37373d] disabled:opacity-50';

// Transcript is one agent's items in a scrolling element. Items arrive whole,
// and a later item with the same ID replaces the earlier one in place. An item
// with a parent that is shown appears inside the parent's details.
export class Transcript {
  // actions.answer(item, choice, text) sends the user's answer to an attention
  // request and returns whether it was accepted.
  constructor(doc, actions) {
    this.doc = doc;
    this.actions = actions;
    this.list = create(doc, 'div', 'flex flex-col gap-3 p-3');
    this.root = create(doc, 'div', 'min-h-0 flex-1 overflow-y-auto text-[13px]', this.list);
    this.rows = new Map();
  }

  // reset replaces every item and shows the end.
  reset(items) {
    this.list.replaceChildren();
    this.rows.clear();
    this.apply(items);
    this.root.scrollTop = this.root.scrollHeight;
  }

  // update adds or replaces items. A view at the end stays at the end.
  update(items) {
    const {scrollTop, clientHeight, scrollHeight} = this.root;
    const atEnd = scrollHeight - scrollTop - clientHeight < 40;
    this.apply(items);
    if (atEnd) this.root.scrollTop = this.root.scrollHeight;
  }

  apply(items) {
    for (const item of items) {
      const json = JSON.stringify(item);
      const kind = kindOf(item);
      let r = this.rows.get(item.id);
      if (r?.json === json) continue;
      if (!r || r.kind !== kind) {
        const old = r;
        r = this.build(kind);
        this.rows.set(item.id, r);
        if (old) {
          old.element.replaceWith(r.element);
        } else {
          const parent = this.rows.get(item.parent);
          if (parent?.kids) parent.kids.hidden = false;
          (parent?.kids ?? this.list).append(r.element);
        }
      }
      r.json = json;
      this.fill(r, item);
      if (kind === 'attention' && !item.done) this.reveal(item.parent);
    }
  }

  // reveal opens the details around an item that needs the user.
  reveal(id) {
    for (let r = this.rows.get(id); r; r = this.rows.get(r.item.parent)) r.element.open = true;
  }

  build(kind) {
    const doc = this.doc;
    const r = {kind, element: null, body: null, kids: null};
    switch (kind) {
      case 'user':
      case 'message':
        r.head = create(doc, 'div', 'flex items-center gap-1.5 text-xs text-neutral-400');
        r.body = create(doc, 'div', messageText);
        r.more = create(doc, 'button', 'self-start text-xs text-neutral-400 hover:text-neutral-200 hover:underline');
        r.more.type = 'button';
        r.more.onclick = () => {
          r.expanded = !r.expanded;
          this.fill(r, r.item);
        };
        r.error = create(doc, 'div', 'whitespace-pre-wrap break-words text-red-300');
        r.element = create(doc, 'div', row[kind], create(doc, 'div', bubble, r.head, r.body, r.more, r.error));
        break;
      case 'reasoning':
        r.body = create(doc, 'div', 'mt-1 whitespace-pre-wrap break-words');
        r.element = create(doc, 'details', 'group ' + row.reasoning,
          create(doc, 'summary', 'flex cursor-pointer list-none items-center gap-1 italic hover:text-neutral-300 [&::-webkit-details-marker]:hidden',
            create(doc, 'span', chevron), 'Thinking'), r.body);
        break;
      case 'tool':
        r.head = create(doc, 'span', 'min-w-0 flex-1 truncate');
        r.status = create(doc, 'span', 'flex shrink-0');
        r.input = create(doc, 'pre', 'max-h-64 overflow-auto whitespace-pre-wrap break-words border-t border-[#2b2b2b] px-2 py-1 font-mono text-xs text-neutral-400');
        r.output = create(doc, 'pre', 'max-h-64 overflow-auto whitespace-pre-wrap break-words border-t border-[#2b2b2b] px-2 py-1 font-mono text-xs text-neutral-300');
        r.kids = create(doc, 'div', 'flex flex-col gap-3 border-t border-[#2b2b2b] p-2');
        r.element = create(doc, 'details', row.tool,
          create(doc, 'summary', summaryClass, create(doc, 'span', chevron), r.head, r.status), r.input, r.output, r.kids);
        break;
      case 'error':
        r.body = r.element = create(doc, 'div', row.error);
        break;
      case 'attention':
        r.body = create(doc, 'div', 'flex flex-col gap-2');
        r.element = create(doc, 'div', row.attention, r.body);
        break;
      default:
        r.body = r.element = create(doc, 'div', row.text);
    }
    return r;
  }

  fill(r, item) {
    const doc = this.doc;
    r.item = item;
    switch (r.kind) {
      case 'user':
        this.message(r, item.from ? 'From ' + item.from : '', item.text ?? '', !!item.from);
        r.error.hidden = true;
        break;
      case 'message': {
        const input = item.input && typeof item.input === 'object' ? item.input : {};
        this.message(r, 'To ' + (input.to ?? ''), typeof input.text === 'string' ? input.text : '', true);
        if (!item.done) r.head.append(spinner(doc));
        r.error.textContent = item.failed ? item.output ?? '' : '';
        r.error.hidden = !r.error.textContent;
        break;
      }
      case 'reasoning':
        r.body.textContent = item.text ?? '';
        break;
      case 'tool': {
        const [name, summary] = toolTitle(item);
        r.head.replaceChildren(create(doc, 'span', 'font-medium text-neutral-200', name), ' ', summary);
        r.status.replaceChildren(...(!item.done ? [spinner(doc)] : item.failed ? [dot(doc, 'Failed', 'bg-red-400')] : []));
        const input = inputText(item.input), output = clip(item.output ?? '');
        r.input.textContent = input;
        r.input.hidden = !input;
        r.output.textContent = output;
        r.output.hidden = !output;
        break;
      }
      case 'error':
        r.body.textContent = item.text ?? '';
        break;
      case 'attention':
        r.body.replaceChildren(...this.form(item));
        break;
      default:
        r.body.replaceChildren(...renderMarkdown(doc, item.text ?? ''));
    }
    if (r.kids) r.kids.hidden = r.kids.children.length === 0;
  }

  // message fills a message's heading and text. A long message between agents
  // shows its first lines and a control that expands and collapses it.
  message(r, heading, text, clamps) {
    r.head.replaceChildren(heading);
    r.head.hidden = !heading;
    const long = clamps && isLong(text);
    r.body.textContent = text;
    r.body.className = messageText + (long && !r.expanded ? ' line-clamp-6' : '');
    r.more.textContent = long ? (r.expanded ? 'Show less' : 'Show more') : '';
    r.more.hidden = !long;
  }

  // form is an attention request: its prompt, then its choices and typed
  // reply while it is open, or the answer once it is finished.
  form(item) {
    const doc = this.doc;
    const parts = [create(doc, 'div', 'whitespace-pre-wrap break-words text-neutral-100', item.text ?? '')];
    if (item.done) {
      if (item.answer) parts.push(create(doc, 'div', 'text-neutral-400', item.answer));
      return parts;
    }
    const controls = [];
    const field = item.free ? create(doc, 'input', 'min-w-0 flex-1 border border-[#454545] bg-[#3c3c3c] px-2 py-1 text-neutral-100 outline-none focus:border-sky-500') : null;
    const buttons = [];
    const send = async (choice, text) => {
      for (const b of buttons) b.disabled = true;
      if (field) field.disabled = true;
      const accepted = await this.actions.answer(item, choice, text);
      if (!accepted) {
        for (const b of buttons) b.disabled = false;
        if (field) field.disabled = false;
      }
    };
    for (const choice of item.choices ?? []) {
      const b = create(doc, 'button', button, choice);
      b.type = 'button';
      b.onclick = () => send(choice, field?.value ?? '');
      buttons.push(b);
      controls.push(b);
    }
    if (field) {
      field.type = 'text';
      const b = create(doc, 'button', button, 'Send');
      b.type = 'button';
      b.onclick = () => { if (field.value.trim()) send('', field.value); };
      field.onkeydown = e => { if (submitsOnKey(e)) b.onclick(); };
      buttons.push(b);
      controls.unshift(field);
      controls.push(b);
    }
    parts.push(create(doc, 'div', 'flex flex-wrap items-center gap-2', ...controls));
    return parts;
  }
}

const spinner = doc => {
  const s = create(doc, 'span', 'icon icon-loading size-3.5 text-neutral-400');
  s.setAttribute('role', 'img');
  s.title = 'Running';
  return s;
};

const dot = (doc, title, color) => {
  const s = create(doc, 'span', `block size-2 rounded-full ${color}`);
  s.title = title;
  return s;
};
