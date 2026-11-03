// The activity of a finished turn: one closed disclosure that says how long
// the agent worked and holds everything the turn showed but its response.
import {Disclosure} from './disclosure.jsx';
import {formatDuration} from './duration.js';
import {TranscriptRow} from './transcript-item.jsx';
import './activity-item.css';

export function ActivityItem({row, session, expanded, setOpen}) {
  const open = !!expanded[row.key];
  const summary = row.start && row.until >= row.start ? 'Worked for ' + formatDuration(row.until - row.start) : 'Worked';
  return (
    <Disclosure class="activity-item" open={open} onToggle={open => setOpen(row.key, open)} summary={summary}>
      {open && (
        <div class="activity-item-rows">
          {row.rows.map(inner => <TranscriptRow key={inner.item.id} row={inner} session={session} expanded={expanded} setOpen={setOpen} />)}
        </div>
      )}
    </Disclosure>
  );
}
