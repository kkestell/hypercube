// An open file, as VS Code's editor shows it: a centered message for a file it
// does not display, the Markdown preview, or highlighted source with line
// numbers. A deleted file keeps its contents, and a refresh keeps the scroll.
import {useRef} from 'preact/hooks';
import {useScrollMemory} from '../editor/use-scroll-memory.js';
import {useStore} from '../store.js';
import {Markdown} from '../transcript/markdown.jsx';
import {HighlightedCode} from './highlighted-code.jsx';
import {language} from './language.js';
import './file-view.css';

function Contents({file, lang}) {
  if (file.message !== null) return <div class="file-view-message">{file.message}</div>;
  if (file.text === null) return null;
  if (lang === 'markdown') return <Markdown text={file.text} style="preview" />;
  const lines = file.text.split('\n').length;
  return (
    <>
      <div class="file-view-gutter" style={{'--digits': Math.max(5, String(lines).length)}}>
        {Array.from({length: lines}, (_, i) => i + 1).join('\n')}
      </div>
      <pre class="file-view-code"><HighlightedCode text={file.text} lang={lang} /></pre>
    </>
  );
}

// id is the file's tab, under which its scroll is kept.
export function FileView({id, path}) {
  const [state] = useStore();
  const view = useRef(null);
  useScrollMemory(view, id);
  const file = state.session.files.open[path];
  return (
    <div ref={view} class="file-view">
      {file && <Contents file={file} lang={language(path.slice(path.lastIndexOf('/') + 1))} />}
    </div>
  );
}
