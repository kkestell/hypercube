import {highlight} from './highlight.ts';

self.onmessage = (event: MessageEvent<{texts: string[]; lang: string}>) => {
  const {texts, lang} = event.data;
  texts.forEach((text, document) => {
    const lines = highlight(text, lang);
    for (let start = 0; start < lines.length; start += 160) {
      self.postMessage({kind: 'chunk', document, start, lines: lines.slice(start, start + 160)});
    }
  });
  self.postMessage({kind: 'done'});
};
