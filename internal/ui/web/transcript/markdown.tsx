// Markdown as React elements. Text from models reaches the page only as text
// and as attributes of the elements built here, so it never becomes HTML: raw
// HTML in Markdown shows as text, and links keep only http, https, and mailto
// addresses. File previews also highlight their code blocks.
import {createElement, Fragment, type ReactNode} from 'react';
import {lexer, type Token, type Tokens} from 'marked';
import {decodeHTMLStrict} from 'entities/decode';
import {CopyButton} from './copy-button.tsx';
import {HighlightedCode} from '../files/highlighted-code.tsx';

// safeHref returns the address of a link the page may follow, or null.
export function safeHref(href: string): string | null {
  try {
    const url = new URL(href);
    return ['http:', 'https:', 'mailto:'].includes(url.protocol) ? url.href : null;
  } catch {
    return null;
  }
}

function Link({href, children}: {href: string; children: ReactNode}) {
  const safe = safeHref(href);
  if (!safe) return <span>{children}</span>;
  return <a href={safe} target="_blank" rel="noopener noreferrer">{children}</a>;
}

// Options are how a token renders: flatten draws plain text, preview omits
// the copy buttons, and imageSource gives the address an image may load from.
interface Options {
  flatten?: boolean;
  preview?: boolean;
  imageSource?: (href: string) => string | null | undefined;
}

function tokens(list: Token[] = [], options: Options): ReactNode[] {
  return list.map((token, i) => <Fragment key={i}>{element(token, options)}</Fragment>);
}

// joined puts a separator between rendered tokens without dropping empty ones.
function joined(nodes: ReactNode[], separator: ReactNode): ReactNode[] {
  return nodes.flatMap((node, i) => i ? [separator, node] : [node]);
}

function element(t: Token, options: Options): ReactNode {
  const children = (inner: Token[] | undefined) => tokens(inner, options);
  switch (t.type) {
    case 'space':
    case 'def':
      return null;
    case 'paragraph': return options.flatten ? children(t.tokens) : <p>{children(t.tokens)}</p>;
    case 'heading': return options.flatten ? children(t.tokens) : createElement('h' + t.depth, null, children(t.tokens));
    case 'code':
      return options.flatten ? t.text : <div className="markdown-code-block group relative [--copy-bg:var(--tw-prose-pre-bg)]"><pre><HighlightedCode text={t.text} lang={t.lang ?? ''} /></pre>{!options.preview && <CopyButton text={t.text} label="Copy code" />}</div>;
    case 'blockquote': return options.flatten ? flatTokens(t.tokens) : <blockquote>{children(t.tokens)}</blockquote>;
    case 'hr': return options.flatten ? null : <hr />;
    case 'list': {
      if (options.flatten) return joined(t.items.map((item: Tokens.ListItem) => flatTokens(item.tokens)), '\n');
      const items = t.items.map((item: Tokens.ListItem, i: number) => <li key={i}>{tokens(item.tokens, options)}</li>);
      return t.ordered ? <ol start={t.start === '' || t.start === 1 ? undefined : t.start}>{items}</ol> : <ul>{items}</ul>;
    }
    case 'checkbox': return options.flatten ? null : <><input type="checkbox" checked={t.checked} disabled />{' '}</>;
    case 'table': {
      if (options.flatten) {
        const rows = [t.header, ...t.rows].map((cells: Tokens.TableCell[]) => joined(cells.map((cell: Tokens.TableCell) => tokens(cell.tokens, options)), ' '));
        return joined(rows, '\n');
      }
      const cell = (Tag: 'th' | 'td', c: Tokens.TableCell, i: number) => <Tag key={i} style={c.align ? {textAlign: c.align} : undefined}>{tokens(c.tokens, options)}</Tag>;
      return (
        <table>
          <thead><tr>{t.header.map((c: Tokens.TableCell, i: number) => cell('th', c, i))}</tr></thead>
          <tbody>{t.rows.map((row: Tokens.TableCell[], i: number) => <tr key={i}>{row.map((c, i) => cell('td', c, i))}</tr>)}</tbody>
        </table>
      );
    }
    case 'strong': return <strong>{children(t.tokens)}</strong>;
    case 'em': return <em>{children(t.tokens)}</em>;
    case 'del': return <del>{children(t.tokens)}</del>;
    case 'codespan': return <code>{t.text}</code>;
    case 'br': return <br />;
    case 'link': return <Link href={decodeHTMLStrict(t.href)}>{children(t.tokens)}</Link>;
    case 'image': {
      const text = decodeHTMLStrict(t.text), href = decodeHTMLStrict(t.href);
      if (options.flatten) return text || null;
      const src = options.imageSource?.(href);
      return src ? <img src={src} alt={text} title={t.title ? decodeHTMLStrict(t.title) : undefined} /> : <Link href={href}>{text || href}</Link>;
    }
    // Decode only parsed text and attributes: code and raw HTML stay literal.
    case 'text': return t.tokens ? children(t.tokens) : decodeHTMLStrict(t.text);
    case 'escape': return t.text;
    default: return t.raw ?? '';
  }
}

// flatTokens renders block tokens as plain text, keeping inline marks.
function flatTokens(list: Token[] = []): ReactNode[] {
  return joined(list.map(token => element(token, {flatten: true})).filter(node => node != null).map((node, i) => <Fragment key={i}>{node}</Fragment>), '\n');
}

// style is 'chat' or 'preview'.
interface MarkdownProps extends Pick<Options, 'imageSource'> {
  text: string;
  style: 'chat' | 'preview';
}

export function Markdown({text, style, imageSource}: MarkdownProps) {
  return (
    <div className={`markdown markdown-${style} prose prose-sm prose-neutral prose-h1:text-xl prose-h2:text-lg prose-h3:text-base prose-h4:text-sm dark:prose-invert max-w-none break-words prose-code:before:content-none prose-code:after:content-none`}>
      {tokens(lexer(text), {preview: style === 'preview', imageSource})}
    </div>
  );
}

// MarkdownText is Markdown as inline text for a compact preview: headings,
// lists, quotes, code blocks, and images flatten to their text, while inline
// marks, inline code, and links remain.
export function MarkdownText({text}: {text: string}) {
  return <span className="markdown markdown-inline">{flatTokens(lexer(text))}</span>;
}
