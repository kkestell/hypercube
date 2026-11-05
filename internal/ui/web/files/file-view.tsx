// Source line selection uses the same theme tokens as the standard controls.
import {Fragment, useLayoutEffect, useRef, useState, type ReactNode} from 'react';
import {useScrollMemory} from '../editor/use-scroll-memory.ts';
import {useStore} from '../store.ts';
import {Find} from '../shared/find.tsx';
import {tabBackground} from '../shared/tab-background.ts';
import {Markdown} from '../transcript/markdown.tsx';
import {revealed} from './files.ts';
import {BrowserPreview} from './browser-preview.tsx';
import {language} from './language.ts';
import {markdownImageURL} from './markdown-image.ts';
import type {LineRange} from './types.ts';

interface SourceProps {
  text: string;
  lines: LineRange[];
  lang: string;
  wrap: boolean;
}

function Source({text, lines, lang, wrap}: SourceProps) {
  const [highlighted, setHighlighted] = useState<{text: string; lines: string[]} | null>(null);
  useLayoutEffect(() => {
    let current = true;
    import('./highlight.ts').then(({highlight}) => { if (current) setHighlighted({text, lines: highlight(text, lang)}); });
    return () => { current = false; };
  }, [text, lang]);
  const html = highlighted?.text === text ? highlighted.lines : null;
  return <div className={`file-view-code grid min-w-full grid-cols-[auto_minmax(0,1fr)] font-mono text-sm leading-5 ${wrap ? 'w-full' : 'w-max'}`}>
    {text.split('\n').map((line, i) => {
      const selected = lines.some(({start, end = start}) => i + 1 >= start && i + 1 <= end);
      return <Fragment key={i}>
        <span data-find-ignore aria-hidden="true" className={`file-view-gutter sticky left-0 select-none px-4 text-right text-muted-foreground ${selected ? 'bg-accent' : tabBackground}`}>{i + 1}</span>
        <code className={`block ${wrap ? 'whitespace-pre-wrap break-all' : 'whitespace-pre'} pr-4 ${selected ? 'file-view-highlight bg-accent' : ''}`}
          {...(html ? {dangerouslySetInnerHTML: {__html: html[i] || ' '}} : {children: line || ' '})} />
      </Fragment>;
    })}
  </div>;
}

interface FileViewProps {
  id: string;
  path: string;
  active?: boolean;
}

export function FileView({id, path, active = true}: FileViewProps) {
  const [state, store] = useStore();
  const view = useRef<HTMLDivElement>(null), list = useRef<HTMLDivElement>(null);
  useScrollMemory(view, id);
  const session = state.session;
  const file = session?.files.open[path];
  const sessionId = session?.id;
  useLayoutEffect(() => {
    if (!sessionId || !file?.reveal || file.text === null) return;
    const highlight = view.current?.querySelector('.file-view-highlight');
    if (!highlight) return;
    highlight.scrollIntoView({block: 'center'});
    store.update(state => revealed(state, sessionId, path));
  }, [file?.text, file?.reveal, file?.source]);
  if (!sessionId) return null;
  const preview = file?.preview && (!file.source || file.text === null) ? file.preview : undefined;
  let content: ReactNode = null;
  if (preview) content = <BrowserPreview key={preview.url} preview={preview} path={path} />;
  else if (file?.message) content = <div className="file-view-message p-6 text-center text-muted-foreground">{file.message}</div>;
  else if (file?.text !== null && file?.text !== undefined) {
    content = language(path) === 'markdown' && !file.source
      ? <div className={`mx-auto p-6 ${state.workbench.fixedWidth ? 'max-w-[51.7rem]' : ''}`}>
        <Markdown text={file.text} style="preview" imageSource={href => markdownImageURL(href, sessionId, path)} />
      </div>
      : <Source text={file.text} lines={file.lines} lang={language(path.slice(path.lastIndexOf('/') + 1))} wrap={state.workbench.wrap} />;
  }
  return <div className="relative flex min-h-0 flex-1 flex-col">
    {!preview && <Find list={list} view={view} active={active} label="Find in file" className="absolute right-2 top-2 z-10 flex items-center gap-2 rounded-md border bg-background p-2 shadow-md" />}
    <div ref={view} tabIndex={0} className={`file-view min-h-0 flex-1 overflow-auto ${preview ? 'flex flex-col' : ''}`}>
      {preview ? content : <div ref={list}>{content}</div>}
    </div>
  </div>;
}
