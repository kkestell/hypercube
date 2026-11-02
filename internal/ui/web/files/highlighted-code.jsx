// Code highlighted by Shiki, which loads as its own chunk the first time any
// code is drawn. The plain text shows until it loads.
import {useLayoutEffect, useRef} from 'preact/hooks';

export function HighlightedCode({text, lang}) {
  const ref = useRef(null);
  useLayoutEffect(() => {
    const code = ref.current;
    let current = true;
    code.textContent = text;
    import('./highlight.js').then(({highlight}) => {
      if (current) code.innerHTML = highlight(text, lang).join('\n');
    });
    return () => {
      current = false;
    };
  }, [text, lang]);
  return <code ref={ref} />;
}
