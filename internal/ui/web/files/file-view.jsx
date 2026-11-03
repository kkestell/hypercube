// An open file, as VS Code's editor shows it: a centered message for a file it
// does not display, the Markdown preview, or highlighted source with line
// numbers. A deleted file keeps its contents, and a refresh keeps the scroll.
import {useLayoutEffect, useRef} from 'preact/hooks';
import {useScrollMemory} from '../editor/use-scroll-memory.js';
import {useStore} from '../store.js';
import {Markdown} from '../transcript/markdown.jsx';
import {HighlightedCode} from './highlighted-code.jsx';
import {revealed} from './files.js';
import {hasPreview, language} from './language.js';
import {markdownImageURL} from './markdown-image.js';
import './file-view.css';

function Contents({file, lang, session, path}) {
  if (file.message !== null) return <div class="file-view-message">{file.message}</div>;
  if (file.text === null) return null;
  if (hasPreview(path) && !file.source) return <Markdown text={file.text} style="preview" imageSource={href => markdownImageURL(href, session, path)} />;
  const lines = file.text.split('\n').length;
  return (
    <>
      {(file.lines ?? []).map(({start, end}) => <div class="file-view-highlight" style={{'--start': start, '--count': end - start + 1}} />)}
      <div class="file-view-gutter" style={{'--digits': Math.max(5, String(lines).length)}}>
        {Array.from({length: lines}, (_, i) => i + 1).join('\n')}
      </div>
      <pre class="file-view-code"><HighlightedCode text={file.text} lang={lang} /></pre>
    </>
  );
}

// id is the file's tab, under which its scroll is kept.
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
  return (
    <div ref={view} class="file-view">
      {file && <Contents file={file} lang={language(path.slice(path.lastIndexOf('/') + 1))} session={state.session.id} path={path} />}
    </div>
  );
}
