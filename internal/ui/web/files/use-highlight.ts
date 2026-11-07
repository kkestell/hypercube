// Source and diff panes each own the worker for their current content.
// Terminating it drops an obsolete synchronous highlight before starting
// another one; plain text remains visible until the result arrives.
import {useEffect, useState} from 'react';

type Chunk = {kind: 'chunk'; document: number; start: number; lines: string[]};

export function useHighlight(texts: string[], lang: string, active: boolean): string[][] | null {
  const first = texts[0], second = texts[1];
  const [result, setResult] = useState<{first: string; second?: string; lang: string; lines: string[][]} | null>(null);
  useEffect(() => {
    if (!active) return;
    let worker: Worker;
    try {
      worker = new Worker(new URL('./highlight-worker.js', import.meta.url), {type: 'module'});
    } catch {
      return;
    }
    const pending: Chunk[] = [];
    let frame = 0;
    const showNext = () => {
      frame = 0;
      const chunk = pending.shift();
      if (!chunk) return;
      setResult(previous => {
        const lines = previous?.first === first && previous.second === second && previous.lang === lang
          ? previous.lines.map(document => [...document])
          : texts.map(() => []);
        lines[chunk.document].splice(chunk.start, chunk.lines.length, ...chunk.lines);
        return {first, second, lang, lines};
      });
      if (pending.length) frame = requestAnimationFrame(showNext);
    };
    worker.onmessage = (event: MessageEvent<Chunk | {kind: 'done'}>) => {
      if (event.data.kind === 'done') {
        worker.terminate();
        return;
      }
      pending.push(event.data);
      if (!frame) frame = requestAnimationFrame(showNext);
    };
    worker.onerror = () => worker.terminate();
    worker.postMessage({texts, lang});
    return () => {
      worker.terminate();
      cancelAnimationFrame(frame);
      pending.length = 0;
    };
  }, [first, second, lang, active]);
  return result?.first === first && result.second === second && result.lang === lang ? result.lines : null;
}
