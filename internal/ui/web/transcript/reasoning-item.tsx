// A run of reasoning draws as one row. A short run shows in full. A longer
// one shows a Thinking heading, or Thought once every item in the run is done,
// over its last lines, each cut to one line on screen so the row's height
// stays the same while the agent reasons. Opening it shows the whole trace,
// which grows as the agent reasons; the last lines slide closed as the trace
// slides open.
import {Collapsible, CollapsibleContent} from '../components/ui/collapsible';
import {Disclosure, useExpanded} from './disclosure.tsx';
import {Markdown, MarkdownText} from './markdown.tsx';
import type {ReasoningRow} from './transcript-tree.ts';

// shownLines is how many lines of reasoning show without opening it. A run
// shows in full when it has no more lines than that and none is longer than
// lineLength characters.
export const shownLines = 3;
export const lineLength = 100;

export function ReasoningItem({row}: {row: ReasoningRow}) {
  const [open] = useExpanded(row.item.id);
  const texts = row.items.map(item => item.text?.trim() ?? '');
  const lines = texts.flatMap(text => text.split('\n')).map(line => line.trim()).filter(Boolean);
  if (lines.length <= shownLines && lines.every(line => line.length <= lineLength)) {
    return <div className="text-muted-foreground">{lines.map((line, i) => <p key={i} className="break-words"><MarkdownText text={line} /></p>)}</div>;
  }
  const thinking = row.items.some(item => !item.done);
  const first = lines.length - shownLines;
  return <div className="text-muted-foreground">
    <Disclosure id={row.item.id} summary={thinking ? 'Thinking…' : 'Thought'}>
      <div className="mt-2 flex flex-col gap-3">
        {row.items.map((item, i) => <Markdown key={item.id} text={texts[i]} style="chat" muted />)}
      </div>
    </Disclosure>
    <Collapsible open={!open}>
      <CollapsibleContent>
        {lines.slice(Math.max(0, first)).map((line, i) => <p key={first + i} className="truncate"><MarkdownText text={line} /></p>)}
      </CollapsibleContent>
    </Collapsible>
  </div>;
}
