// Source line selection uses the same theme tokens as the standard controls.
import {Fragment, useEffect, useLayoutEffect, useRef, type ReactNode} from 'react';
import {useScrollMemory} from '../shared/use-scroll-memory.ts';
import {openFileTab} from '../editor/editor.ts';
import {useStore} from '../store.ts';
import {Find} from '../shared/find.tsx';
import {Message} from '../shared/message.tsx';
import {Markdown, MarkdownLink} from '../transcript/markdown.tsx';
import {fileReady, readOpenFile, revealed} from './files.ts';
import {BrowserPreview} from './browser-preview.tsx';
import {language} from './language.ts';
import {markdownImageURL} from './markdown-image.ts';
import {markdownFilePath} from './markdown-path.ts';
import type {LineRange} from './types.ts';
import {useHighlight} from './use-highlight.ts';

interface SourceProps {
  text: string;
  lines: LineRange[];
  lang: string;
  wrap: boolean;
}

function Source({text, lines, lang, wrap}: SourceProps) {
  const html = useHighlight([text], lang)?.[0];
  return <div className={`file-view-code grid min-w-full grid-cols-[auto_minmax(0,1fr)] font-mono text-xs leading-5 ${wrap ? 'w-full' : 'w-max'}`}>
    {text.split('\n').map((line, i) => {
      const selected = lines.some(({start, end = start}) => i + 1 >= start && i + 1 <= end);
      return <Fragment key={i}>
        <span data-find-ignore aria-hidden="true" className={`sticky left-0 select-none px-4 text-right text-muted-foreground ${selected ? 'bg-accent' : 'bg-tab'}`}>{i + 1}</span>
        <code data-selected={selected || undefined} className={`block ${wrap ? 'whitespace-pre-wrap break-all' : 'whitespace-pre'} pr-4 ${selected ? 'bg-accent' : ''}`}
          {...(html?.[i] !== undefined ? {dangerouslySetInnerHTML: {__html: html[i] || ' '}} : {children: line || ' '})} />
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
  const editor = state.editor;
  const file = editor?.files.open[path];
  const ready = fileReady(file);
  useScrollMemory(view, id, ready);
  const viewKey = editor?.id;
  useEffect(() => {
    if (viewKey && file && !ready) readOpenFile(store, path);
  }, [viewKey, path, !!file, ready]);
  useLayoutEffect(() => {
    if (!viewKey || !file?.reveal || file.text === null) return;
    const highlight = view.current?.querySelector('[data-selected]');
    if (!highlight) return;
    highlight.scrollIntoView({block: 'center'});
    store.update(state => revealed(state, viewKey, path));
  }, [file?.text, file?.reveal, file?.source]);
  if (!viewKey) return null;
  const preview = file?.preview && (!file.source || file.text === null) ? file.preview : undefined;
  let content: ReactNode = null;
  if (preview) content = <BrowserPreview key={preview.url} preview={preview} path={path} />;
  else if (file?.message) content = <Message fill>{file.message}</Message>;
  else if (file?.text !== null && file?.text !== undefined) {
    content = language(path) === 'markdown' && !file.source
      ? <div className={`mx-auto p-6 ${state.workbench.fixedWidth ? 'max-w-3xl' : ''}`}>
        <Markdown text={file.text} style="preview" imageSource={href => markdownImageURL(href, editor!.target, path)} renderLink={(href, children) => {
          const target = markdownFilePath(href, path);
          return target === null ? <MarkdownLink href={href}>{children}</MarkdownLink> : <a href={href} onClick={event => {
            event.preventDefault();
            openFileTab(store, target);
          }}>{children}</a>;
        }} />
      </div>
      : <Source text={file.text} lines={file.lines} lang={language(path.slice(path.lastIndexOf('/') + 1))} wrap={state.workbench.wrap} />;
  }
  return <div className="relative flex min-h-0 flex-1 flex-col">
    {!preview && <Find list={list} view={view} active={active} label="Find in file" />}
    <div ref={view} role="region" aria-label="File contents" tabIndex={0} className={`file-view min-h-0 flex-1 overflow-auto ${preview ? 'flex flex-col' : ''}`}>
      {preview ? content : <div ref={list}>{content}</div>}
    </div>
  </div>;
}
