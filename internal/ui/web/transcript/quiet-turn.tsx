// While a turn runs and its transcript stays unchanged for a few seconds, a
// row at the end of the transcript shows that the agent is still working. An
// agent can reason or write a long tool input without sending anything. A
// running tool shows its own status, so the row waits only on the agent.
import {useEffect, useState} from 'react';
import {BusyDot} from '../shared/busy-dot.tsx';
import type {Transcript} from './types.ts';

// quietDelay is how long, in milliseconds, a running turn sends nothing
// before the row shows.
export const quietDelay = 3000;

// runsTool reports a tool call still running in the latest turn, which
// begins after the transcript's last state item.
export function runsTool({order, items}: Transcript): boolean {
  for (let i = order.length - 1; i >= 0; i--) {
    const item = items[order[i]];
    if (item.kind === 'state') return false;
    if (item.kind === 'tool' && !item.done) return true;
  }
  return false;
}

// useQuiet reports whether transcript has stayed the same for quietDelay
// while waiting.
export function useQuiet(transcript: Transcript, waiting: boolean): boolean {
  const [quietOn, setQuietOn] = useState<Transcript | null>(null);
  useEffect(() => {
    if (!waiting) return;
    const timer = setTimeout(() => setQuietOn(transcript), quietDelay);
    return () => clearTimeout(timer);
  }, [transcript, waiting]);
  return waiting && quietOn === transcript;
}

// QuietTurn lines its dot up with the status column of tool calls.
export function QuietTurn() {
  return <div role="status" className="flex items-center py-1 text-muted-foreground">
    <span aria-hidden className="flex h-5 w-6 shrink-0 items-center"><BusyDot title="Working" /></span>
    Working…
  </div>;
}
