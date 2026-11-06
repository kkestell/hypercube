// The activity of a finished turn, laid out as the ChatGPT app lays it out: a
// "Worked for" toggle with a chevron after it, a rule beneath, and the
// activity, when open, below the rule. Closed, the turn's response follows the
// rule.
import {Button} from '../components/ui/button';
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from '../components/ui/collapsible';
import {Chevron} from '../shared/icon.tsx';
import {formatDuration} from './duration.ts';
import {TranscriptRow, type RowControls} from './transcript-item.tsx';
import type {ActivityRow} from './transcript-tree.ts';

export function ActivityItem({row, session, expanded, setOpen}: RowControls & {row: ActivityRow}) {
  const open = !!expanded[row.key];
  const label = row.start && row.until >= row.start ? 'Worked for ' + formatDuration(row.until - row.start) : 'Worked';
  return (
    <Collapsible className="activity-item" open={open} onOpenChange={open => setOpen(row.key, open)}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" size="sm" className="h-auto gap-2 px-0 py-1 font-normal has-[>svg]:px-0 text-muted-foreground hover:bg-transparent hover:text-muted-foreground dark:hover:bg-transparent">
          {label}<Chevron open={open} />
        </Button>
      </CollapsibleTrigger>
      <div className="mt-1 border-t" />
      <CollapsibleContent className="activity-item-rows mt-4 flex flex-col gap-3">
        {open && row.rows.map(inner => <TranscriptRow key={inner.item.id} row={inner} session={session} expanded={expanded} setOpen={setOpen} />)}
      </CollapsibleContent>
    </Collapsible>
  );
}
