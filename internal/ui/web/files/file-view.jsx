// Source line selection uses the same theme tokens as the standard controls.
import {Fragment, useLayoutEffect, useRef, useState} from 'react';
import {useScrollMemory} from '../editor/use-scroll-memory.js';
import {useStore} from '../store.js';
import {Find} from '../shared/find.jsx';
import {Markdown} from '../transcript/markdown.jsx';
import {revealed} from './files.js';
import {hasPreview, language} from './language.js';
import {markdownImageURL} from './markdown-image.js';

function Source({file, lang, wrap}) {
  const [highlighted, setHighlighted] = useState(null);
  useLayoutEffect(() => {
    let current = true;
    import('./highlight.js').then(({highlight}) => { if (current) setHighlighted({text: file.text, lines: highlight(file.text, lang)}); });
    return () => { current = false; };
  }, [file.text, lang]);
  const html = highlighted?.text === file.text ? highlighted.lines : null;
  return <div className={`file-view-code grid min-w-full grid-cols-[auto_minmax(0,1fr)] font-mono text-sm leading-5 ${wrap ? 'w-full' : 'w-max'}`}>
    {file.text.split('\n').map((line, i) => {
      const selected = file.lines?.some(({start, end}) => i + 1 >= start && i + 1 <= end);
      return <Fragment key={i}>
        <span data-find-ignore aria-hidden="true" className={`file-view-gutter sticky left-0 select-none px-4 text-right text-muted-foreground ${selected ? 'bg-accent' : 'bg-background'}`}>{i + 1}</span>
        <code className={`block ${wrap ? 'whitespace-pre-wrap break-all' : 'whitespace-pre'} pr-4 ${selected ? 'file-view-highlight bg-accent' : ''}`}
          {...(html ? {dangerouslySetInnerHTML: {__html: html[i] || ' '}} : {children: line || ' '})} />
      </Fragment>;
    })}
  </div>;
}

export function FileView({id, path, active = true}) {
  const [state, store] = useStore();
  const view = useRef(null), list = useRef(null);
  useScrollMemory(view, id);
  const file = state.session.files.open[path];
  const sessionId = state.session.id;
  useLayoutEffect(() => {
    if (!file?.reveal || file.text === null) return;
    const highlight = view.current.querySelector('.file-view-highlight');
    if (!highlight) return;
    highlight.scrollIntoView({block: 'center'});
    store.update(state => revealed(state, sessionId, path));
  }, [file?.text, file?.reveal, file?.source]);
  return <div className="relative flex min-h-0 flex-1 flex-col"><Find list={list} view={view} active={active} label="Find in file" className="absolute right-2 top-2 z-10 flex items-center gap-2 rounded-md border bg-background p-2 shadow-md" /><div ref={view} tabIndex={0} className="file-view min-h-0 flex-1 overflow-auto"><div ref={list}>
    {file && (file.message != null ? <div className="file-view-message p-6 text-center text-muted-foreground">{file.message}</div>
      : file.text == null ? null
      : hasPreview(path) && !file.source ? <div className="p-6"><Markdown text={file.text} style="preview" imageSource={href => markdownImageURL(href, sessionId, path)} /></div>
      : <Source file={file} lang={language(path.slice(path.lastIndexOf('/') + 1))} wrap={state.workbench?.wrap} />)}
  </div></div></div>;
}
