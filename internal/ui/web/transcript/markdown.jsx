// Markdown as Preact elements. Text from models reaches the page only as text
// and as attributes of the elements built here, so it never becomes HTML: raw
// HTML in Markdown shows as text, and links keep only http, https, and mailto
// addresses. The chat style is for transcripts; the preview style follows VS
// Code's Markdown preview and highlights its code blocks.
import {lexer} from 'marked';
import {CopyButton} from './copy-button.jsx';
import {HighlightedCode} from '../files/highlighted-code.jsx';
import './markdown.css';

// safeHref returns the address of a link the page may follow, or null.
export function safeHref(href) {
  try {
    const url = new URL(href);
    return ['http:', 'https:', 'mailto:'].includes(url.protocol) ? url.href : null;
  } catch {
    return null;
  }
}

function Link({href, children}) {
  const safe = safeHref(href);
  if (!safe) return <span>{children}</span>;
  return <a href={safe} target="_blank" rel="noopener noreferrer">{children}</a>;
}

function tokens(list = [], options) {
  return list.map(token => element(token, options));
}

// joined puts a separator between rendered tokens without dropping empty ones.
function joined(nodes, separator) {
  return nodes.flatMap((node, i) => i ? [separator, node] : [node]);
}

function element(t, options) {
  const children = () => tokens(t.tokens, options);
  switch (t.type) {
    case 'space':
    case 'def':
      return null;
    case 'paragraph': return options.flatten ? children() : <p>{children()}</p>;
    case 'heading': {
      if (options.flatten) return children();
      const Heading = 'h' + t.depth;
      return <Heading>{children()}</Heading>;
    }
    case 'code':
      return options.flatten ? t.text : <div class="markdown-code-block"><pre>{options.preview ? <HighlightedCode text={t.text} lang={t.lang ?? ''} /> : <code>{t.text}</code>}</pre>{!options.preview && <CopyButton text={t.text} label="Copy code" />}</div>;
    case 'blockquote': return options.flatten ? flatTokens(t.tokens) : <blockquote>{children()}</blockquote>;
    case 'hr': return options.flatten ? null : <hr />;
    case 'list': {
      if (options.flatten) return joined(t.items.map(item => flatTokens(item.tokens)), '\n');
      const items = t.items.map(item => <li>{tokens(item.tokens, options)}</li>);
      return t.ordered ? <ol start={t.start === '' || t.start === 1 ? undefined : t.start}>{items}</ol> : <ul>{items}</ul>;
    }
    case 'checkbox': return options.flatten ? null : <><input type="checkbox" checked={t.checked} disabled />{' '}</>;
    case 'table': {
      if (options.flatten) {
        const rows = [t.header, ...t.rows].map(cells => joined(cells.map(cell => tokens(cell.tokens, options)), ' '));
        return joined(rows, '\n');
      }
      const cell = (Tag, c) => <Tag style={c.align ? {textAlign: c.align} : undefined}>{tokens(c.tokens, options)}</Tag>;
      return (
        <table>
          <thead><tr>{t.header.map(c => cell('th', c))}</tr></thead>
          <tbody>{t.rows.map(row => <tr>{row.map(c => cell('td', c))}</tr>)}</tbody>
        </table>
      );
    }
    case 'strong': return <strong>{children()}</strong>;
    case 'em': return <em>{children()}</em>;
    case 'del': return <del>{children()}</del>;
    case 'codespan': return <code>{t.text}</code>;
    case 'br': return <br />;
    case 'link': return <Link href={t.href}>{children()}</Link>;
    case 'image': {
      if (options.flatten) return t.text || null;
      const src = options.imageSource?.(t.href);
      return src ? <img src={src} alt={t.text} title={t.title || undefined} /> : <Link href={t.href}>{t.text || t.href}</Link>;
    }
    case 'text': return t.tokens ? children() : t.text;
    case 'escape': return t.text;
    default: return t.raw ?? '';
  }
}

// flatTokens renders block tokens as plain text, keeping inline marks.
function flatTokens(list = []) {
  return joined(list.map(token => element(token, {flatten: true})).filter(node => node != null), '\n');
}

// style is 'chat' or 'preview'.
export function Markdown({text, style, imageSource}) {
  return <div class={`markdown markdown-${style}`}>{tokens(lexer(text), {preview: style === 'preview', imageSource})}</div>;
}

// MarkdownText is Markdown as inline text for a compact preview: headings,
// lists, quotes, code blocks, and images flatten to their text, while inline
// marks, inline code, and links remain.
export function MarkdownText({text}) {
  return <span class="markdown markdown-inline">{flatTokens(lexer(text))}</span>;
}
