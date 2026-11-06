// A turn's activity: one disclosure that says how long the agent has worked
// and holds everything the turn showed but its response. A finished turn's
// activity starts closed. A running turn's activity counts up each second;
// open, it scrolls within half the transcript's height and keeps its newest
// rows in view.
import {useEffect, useState} from 'react';
import {Disclosure} from './disclosure.tsx';
import {formatDuration} from './duration.ts';
import {TranscriptRow, type RowControls} from './transcript-item.tsx';
import type {ActivityRow} from './transcript-tree.ts';

function useNow(): number {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);
  return now;
}

const Working = ({start}: {start: number}) => {
  const now = useNow();
  return <>{start ? 'Working for ' + formatDuration(now - start) + '…' : 'Working…'}</>;
};

export function ActivityItem({row, session, expanded, setOpen}: RowControls & {row: ActivityRow}) {
  const open = !!expanded[row.key];
  const summary = row.running ? <Working start={row.start} />
    : row.start && row.until >= row.start ? 'Worked for ' + formatDuration(row.until - row.start) : 'Worked';
  const rows = (
    <div className="activity-item-rows ml-2 flex flex-col gap-3 border-l pl-6">
      {row.rows.map(inner => <TranscriptRow key={inner.item.id} row={inner} session={session} expanded={expanded} setOpen={setOpen} />)}
    </div>
  );
  return (
    <Disclosure className="activity-item" open={open} onToggle={open => setOpen(row.key, open)} summary={summary}>
      {open && (row.running
        // A reversed flex column starts scrolled to its end and stays there.
        ? <div className="activity-item-scroll flex max-h-[50cqh] flex-col-reverse overflow-y-auto">{rows}</div>
        : rows)}
    </Disclosure>
  );
}
