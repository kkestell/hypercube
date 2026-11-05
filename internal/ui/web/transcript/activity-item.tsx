// The activity of a finished turn: one closed disclosure that says how long
// the agent worked and holds everything the turn showed but its response.
import {Disclosure} from './disclosure.tsx';
import {formatDuration} from './duration.ts';
import {TranscriptRow, type RowControls} from './transcript-item.tsx';
import type {ActivityRow} from './transcript-tree.ts';

export function ActivityItem({row, session, expanded, setOpen}: RowControls & {row: ActivityRow}) {
  const open = !!expanded[row.key];
  const summary = row.start && row.until >= row.start ? 'Worked for ' + formatDuration(row.until - row.start) : 'Worked';
  return (
    <Disclosure className="activity-item" open={open} onToggle={open => setOpen(row.key, open)} summary={summary}>
      {open && (
        <div className="activity-item-rows ml-2 flex flex-col gap-3 border-l pl-6">
          {row.rows.map(inner => <TranscriptRow key={inner.item.id} row={inner} session={session} expanded={expanded} setOpen={setOpen} />)}
        </div>
      )}
    </Disclosure>
  );
}
