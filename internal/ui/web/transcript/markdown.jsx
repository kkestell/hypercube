// Markdown as Preact elements. Text from models reaches the page only as text
// and as attributes of the elements built here, so it never becomes HTML: raw
// HTML in Markdown shows as text, and links keep only http, https, and mailto
// addresses. The chat style is for transcripts; the preview style follows VS
// Code's Markdown preview and highlights its code blocks.
import {lexer} from 'marked';
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

function tokens(list = [], preview) {
  return list.map(token => element(token, preview));
}

function element(t, preview) {
  const children = () => tokens(t.tokens, preview);
  switch (t.type) {
    case 'space':
    case 'def':
      return null;
    case 'paragraph': return <p>{children()}</p>;
    case 'heading': {
      const Heading = 'h' + t.depth;
      return <Heading>{children()}</Heading>;
    }
    case 'code':
      return <pre>{preview ? <HighlightedCode text={t.text} lang={t.lang ?? ''} /> : <code>{t.text}</code>}</pre>;
    case 'blockquote': return <blockquote>{children()}</blockquote>;
    case 'hr': return <hr />;
    case 'list': {
      const items = t.items.map(item => <li>{tokens(item.tokens, preview)}</li>);
      return t.ordered ? <ol start={t.start === '' || t.start === 1 ? undefined : t.start}>{items}</ol> : <ul>{items}</ul>;
    }
    case 'checkbox': return <><input type="checkbox" checked={t.checked} disabled />{' '}</>;
    case 'table': {
      const cell = (Tag, c) => <Tag style={c.align ? {textAlign: c.align} : undefined}>{tokens(c.tokens, preview)}</Tag>;
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
    case 'image': return <Link href={t.href}>{t.text || t.href}</Link>;
    case 'text': return t.tokens ? children() : t.text;
    case 'escape': return t.text;
    default: return t.raw ?? '';
  }
}

// style is 'chat' or 'preview'.
export function Markdown({text, style}) {
  return <div class={`markdown markdown-${style}`}>{tokens(lexer(text), style === 'preview')}</div>;
}
