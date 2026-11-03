// A header that says how long Thinking or a run of calls has lasted: "Working
// for 5s…" while it goes on, "Worked for 5s" once it ends. One clock ticks for
// every header on the page.
import {createContext} from 'preact';
import {useContext, useEffect, useState} from 'preact/hooks';

const Clock = createContext(0);

export function ClockProvider({children}) {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);
  return <Clock.Provider value={now}>{children}</Clock.Provider>;
}

// formatDuration writes milliseconds as "5s", "2m 32s", or "1h 4m".
export function formatDuration(ms) {
  const seconds = Math.max(0, Math.floor(ms / 1000));
  const minutes = Math.floor(seconds / 60), hours = Math.floor(minutes / 60);
  if (hours) return `${hours}h ${minutes % 60}m`;
  if (minutes) return `${minutes}m ${seconds % 60}s`;
  return `${seconds}s`;
}

// Elapsed is the header text for a span from start to until. A live span runs
// to now; an ended span with unknown times says only its verb.
export function Elapsed({live, present, past, start, until}) {
  const now = useContext(Clock);
  const end = live ? now : until;
  const duration = start && end ? ' for ' + formatDuration(end - start) : '';
  return live ? `${present}${duration}…` : past + duration;
}
