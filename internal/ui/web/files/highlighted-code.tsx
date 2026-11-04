// Shiki loads on first use; plain text shows until its highlighted copy arrives.
import {useEffect, useState} from 'react';

export function HighlightedCode({text, lang}: {text: string; lang: string}) {
  const [highlighted, setHighlighted] = useState<{text: string; lang: string; html: string} | null>(null);
  useEffect(() => {
    let current = true;
    import('./highlight.ts').then(({highlight}) => {
      if (current) setHighlighted({text, lang, html: highlight(text, lang).join('\n')});
    });
    return () => { current = false; };
  }, [text, lang]);
  return highlighted?.text === text && highlighted.lang === lang
    ? <code dangerouslySetInnerHTML={{__html: highlighted.html}} /> : <code>{text}</code>;
}
