// Renders an agent's transcript as DOM. Text from models and tools reaches the
// page only as text nodes and as attributes of elements this file creates, so
// it never becomes HTML. Markdown is parsed into plain objects first; only
// links with http, https, or mailto addresses keep their address.

const fence = /^ {0,3}(`{3,}|~{3,})\s*([^\s`]*)/;
const heading = /^ {0,3}(#{1,6})\s+(.*?)(?:\s+#+)?\s*$/;
const quote = /^ {0,3}>\s?/;
const marker = /^( {0,3})([-*+]|\d{1,9}[.)])\s+(.*)$/;

// blocks parses Markdown into paragraphs, headings, code, quotes, and lists.
// An unfinished code block, as in streamed text, runs to the end.
export function blocks(text) {
  const lines = text.replace(/\r\n?/g, '\n').split('\n');
  const out = [];
  const starts = line => fence.test(line) || heading.test(line) || quote.test(line) || marker.test(line);
  for (let i = 0; i < lines.length;) {
    const line = lines[i];
    let m = fence.exec(line);
    if (m) {
      const code = [];
      for (i++; i < lines.length; i++) {
        const close = /^ {0,3}(`{3,}|~{3,})\s*$/.exec(lines[i]);
        if (close && close[1][0] === m[1][0] && close[1].length >= m[1].length) { i++; break; }
        code.push(lines[i]);
      }
      out.push({type: 'code', lang: m[2], text: code.join('\n')});
    } else if (!line.trim()) {
      i++;
    } else if ((m = heading.exec(line))) {
      out.push({type: 'heading', level: m[1].length, children: inline(m[2])});
      i++;
    } else if (quote.test(line)) {
      const inner = [];
      for (; i < lines.length && quote.test(lines[i]); i++) inner.push(lines[i].replace(quote, ''));
      out.push({type: 'quote', children: blocks(inner.join('\n'))});
    } else if ((m = marker.exec(line))) {
      const ordered = /\d/.test(m[2]);
      const list = {type: ordered ? 'ol' : 'ul', start: ordered ? parseInt(m[2], 10) : 1, items: []};
      const indent = m[1].length;
      // After an item, the next marker of the same list continues it, even
      // past blank lines.
      const following = () => {
        let j = i;
        while (j < lines.length && !lines[j].trim()) j++;
        const next = j < lines.length && marker.exec(lines[j]);
        if (!next || next[1].length > indent + 1 || /\d/.test(next[2]) !== ordered) return null;
        i = j;
        return next;
      };
      for (; m; m = following()) {
        // The item's text continues on lines indented past its marker, and on
        // lines that begin no other block.
        const width = m[1].length + m[2].length + 1;
        const item = [m[3]];
        for (i++; i < lines.length; i++) {
          const next = lines[i];
          if (!next.trim()) {
            const after = lines.slice(i + 1).find(l => l.trim());
            if (after === undefined || !/^ {2,}\S/.test(after) || marker.test(after) && after.search(/\S/) <= indent + 1) break;
            item.push('');
          } else if (/^ {2,}\S/.test(next) && !(marker.test(next) && next.search(/\S/) <= indent + 1)) {
            item.push(next.slice(Math.min(width, next.search(/\S/))));
          } else if (!starts(next)) {
            item.push(next);
          } else {
            break;
          }
        }
        list.items.push(blocks(item.join('\n')));
      }
      out.push(list);
    } else {
      const para = [line];
      for (i++; i < lines.length && lines[i].trim() && !starts(lines[i]); i++) para.push(lines[i]);
      out.push({type: 'paragraph', children: inline(para.join('\n'))});
    }
  }
  return out;
}

// safeHref returns the address of a link the page may follow, or null.
export function safeHref(href) {
  try {
    const url = new URL(href);
    return ['http:', 'https:', 'mailto:'].includes(url.protocol) ? url.href : null;
  } catch {
    return null;
  }
}

// inline parses code spans, emphasis, links, and line breaks.
export function inline(text) {
  const out = [];
  let buffer = '';
  const flush = () => { if (buffer) out.push({type: 'text', text: buffer}); buffer = ''; };
  for (let i = 0; i < text.length;) {
    const c = text[i], rest = text.slice(i);
    let m;
    if (c === '\\' && /^\\[\\`*_{}[\]()#+\-.!>~|<]/.test(rest)) {
      buffer += text[i + 1];
      i += 2;
    } else if (c === '`') {
      const run = /^`+/.exec(rest)[0];
      const end = text.indexOf(run, i + run.length);
      if (end < 0) { buffer += run; i += run.length; continue; }
      flush();
      out.push({type: 'code', text: text.slice(i + run.length, end).replace(/^ (.*\S.*) $/s, '$1')});
      i = end + run.length;
    } else if (c === '[' && (m = /^\[([^\]]*)\]\(\s*([^)\s]+)(?:\s+"[^"]*")?\s*\)/.exec(rest))) {
      flush();
      out.push({type: 'link', href: m[2], children: inline(m[1])});
      i += m[0].length;
    } else if ((c === '*' || c === '_') && (m = emphasis(text, i))) {
      const {length, end, type} = m;
      flush();
      out.push({type, children: inline(text.slice(i + length, end))});
      i = end + length;
    } else if ((m = /^https?:\/\/[^\s<>]*[^\s<>.,;:!?'")\]]/.exec(rest)) && !/[\w]/.test(text[i - 1] ?? '')) {
      flush();
      out.push({type: 'link', href: m[0], children: [{type: 'text', text: m[0]}]});
      i += m[0].length;
    } else if (c === '\n') {
      flush();
      out.push({type: 'break'});
      i++;
    } else {
      buffer += c;
      i++;
    }
  }
  flush();
  return out;
}

// emphasis finds the closing mark of the emphasis starting at i. A mark that
// touches a space inside, or sits within a word as an underscore, is plain.
function emphasis(text, i) {
  const c = text[i];
  const length = text[i + 1] === c ? 2 : 1;
  const mark = c.repeat(length);
  if (c === '_' && /\w/.test(text[i - 1] ?? '')) return null;
  const start = i + length;
  if (!text[start] || /\s/.test(text[start]) || text[start] === c) return null;
  for (let end = text.indexOf(mark, start); end >= 0; end = text.indexOf(mark, end + 1)) {
    if (end === start || /\s/.test(text[end - 1])) continue;
    if (c === '_' && /\w/.test(text[end + length] ?? '')) continue;
    if (length === 1 && text[end + 1] === c) { end++; continue; }
    return {length, end, type: length === 2 ? 'strong' : 'em'};
  }
  return null;
}

const create = (doc, tag, className, ...children) => {
  const node = doc.createElement(tag);
  if (className) node.className = className;
  node.append(...children);
  return node;
};

function renderInline(doc, nodes) {
  return nodes.map(n => {
    switch (n.type) {
      case 'text': return doc.createTextNode(n.text);
      case 'break': return create(doc, 'br');
      case 'code': return create(doc, 'code', '', n.text);
      case 'strong': return create(doc, 'strong', '', ...renderInline(doc, n.children));
      case 'em': return create(doc, 'em', '', ...renderInline(doc, n.children));
      case 'link': {
        const href = safeHref(n.href);
        if (!href) return create(doc, 'span', '', ...renderInline(doc, n.children));
        const a = create(doc, 'a', '', ...renderInline(doc, n.children));
        a.setAttribute('href', href);
        a.setAttribute('target', '_blank');
        a.setAttribute('rel', 'noopener noreferrer');
        return a;
      }
    }
  });
}

function renderBlocks(doc, list) {
  return list.map(b => {
    switch (b.type) {
      case 'paragraph': return create(doc, 'p', '', ...renderInline(doc, b.children));
      case 'heading': return create(doc, 'h' + b.level, '', ...renderInline(doc, b.children));
      case 'code': return create(doc, 'pre', '', create(doc, 'code', '', b.text));
      case 'quote': return create(doc, 'blockquote', '', ...renderBlocks(doc, b.children));
      default: {
        const list = create(doc, b.type, '', ...b.items.map(item => create(doc, 'li', '', ...renderBlocks(doc, item))));
        if (b.type === 'ol' && b.start !== 1) list.setAttribute('start', b.start);
        return list;
      }
    }
  });
}

// renderMarkdown returns the DOM nodes for Markdown text.
export function renderMarkdown(doc, text) {
  return renderBlocks(doc, blocks(text));
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

const row = {
  user: 'flex justify-end',
  text: 'md text-neutral-200',
  reasoning: 'text-neutral-500',
  tool: 'group min-w-0 border border-[#2b2b2b] bg-[#1e1e1e]',
  error: 'border border-red-500/50 bg-red-500/10 px-2 py-1.5 whitespace-pre-wrap break-words text-red-300',
  attention: 'border border-amber-400 bg-amber-400/10 px-2 py-1.5',
};

const summaryClass = 'flex cursor-pointer list-none items-center gap-1.5 px-2 py-1 text-neutral-400 hover:text-neutral-200 [&::-webkit-details-marker]:hidden';
const chevron = 'icon icon-chevron-right size-4 transition-transform group-open:rotate-90';
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
      const kind = kinds.has(item.kind) ? item.kind : 'text';
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
        r.body = create(doc, 'div', 'max-w-[85%] whitespace-pre-wrap break-words rounded bg-[#37373d] px-3 py-1.5 text-neutral-100');
        r.element = create(doc, 'div', row.user, r.body);
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
        r.body.textContent = item.text ?? '';
        break;
      case 'reasoning':
        r.body.textContent = item.text ?? '';
        break;
      case 'tool': {
        r.head.replaceChildren(create(doc, 'span', 'font-medium text-neutral-200', item.tool ?? 'tool'), ' ', toolSummary(item.input));
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
