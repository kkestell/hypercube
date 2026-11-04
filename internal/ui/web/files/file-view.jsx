// Source line selection uses the same theme tokens as the standard controls.
import {Fragment, useLayoutEffect, useRef, useState} from 'react';
import {useScrollMemory} from '../editor/use-scroll-memory.js';
import {useStore} from '../store.js';
import {Markdown} from '../transcript/markdown.jsx';
import {revealed} from './files.js';
import {hasPreview, language} from './language.js';
import {markdownImageURL} from './markdown-image.js';

function Source({file, lang}) {
  const [highlighted, setHighlighted] = useState(null);
  useLayoutEffect(() => {
    let current = true;
    import('./highlight.js').then(({highlight}) => { if (current) setHighlighted({text: file.text, lines: highlight(file.text, lang)}); });
    return () => { current = false; };
  }, [file.text, lang]);
  const html = highlighted?.text === file.text ? highlighted.lines : null;
  return <div className="file-view-code grid min-w-full w-max grid-cols-[auto_1fr] font-mono text-sm leading-6">
    {file.text.split('\n').map((line, i) => {
      const selected = file.lines?.some(({start, end}) => i + 1 >= start && i + 1 <= end);
      return <Fragment key={i}>
        <span aria-hidden="true" className={`file-view-gutter sticky left-0 select-none px-4 text-right text-muted-foreground ${selected ? 'bg-accent' : 'bg-background'}`}>{i + 1}</span>
        <code className={`whitespace-pre pr-4 ${selected ? 'file-view-highlight bg-accent' : ''}`}
          {...(html ? {dangerouslySetInnerHTML: {__html: html[i] || ' '}} : {children: line || ' '})} />
      </Fragment>;
    })}
  </div>;
}

export function FileView({id, path}) {
  const [state, store] = useStore();
  const view = useRef(null);
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
  return <div ref={view} className="file-view min-h-0 flex-1 overflow-auto">
    {file && (file.message != null ? <div className="file-view-message p-6 text-center text-muted-foreground">{file.message}</div>
      : file.text == null ? null
      : hasPreview(path) && !file.source ? <div className="p-6"><Markdown text={file.text} style="preview" imageSource={href => markdownImageURL(href, sessionId, path)} /></div>
      : <Source file={file} lang={language(path.slice(path.lastIndexOf('/') + 1))} />)}
  </div>;
}
