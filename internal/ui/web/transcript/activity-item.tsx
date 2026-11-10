// The activity of a finished turn, laid out as the ChatGPT app lays it out: a
// "Worked for" toggle with a chevron after it, a rule beneath, and the
// activity, when open, below the rule. Closed, the turn's response follows the
// rule.
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {slide} from '../shared/motion.ts';
import {Chevron} from '../shared/icon.tsx';
import {useExpanded} from './disclosure.tsx';
import {formatDuration} from './duration.ts';
import {TranscriptRow} from './transcript-item.tsx';
import type {ActivityRow} from './transcript-tree.ts';

export function ActivityItem({row}: {row: ActivityRow}) {
  const [open, setOpen] = useExpanded(row.key);
  const label = row.start && row.until >= row.start ? 'Worked for ' + formatDuration(row.until - row.start) : 'Worked';
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger className="flex items-center gap-2 rounded-md py-1 text-muted-foreground">
        {label}<Chevron open={open} />
      </CollapsibleTrigger>
      <div className="mt-1 border-t" />
      <CollapsibleContent className={`flex flex-col gap-3 pt-4 ${slide}`}>
        {row.rows.map(inner => <TranscriptRow key={inner.item.id} row={inner} />)}
      </CollapsibleContent>
    </Collapsible>
  );
}
