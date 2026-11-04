import {Icon} from '../shared/icon.jsx';
// Subscription usage: standard progress bars compare usage to elapsed time,
// with an Ahead badge when usage exceeds that pace. Expired windows retain
// their last reported use until the next harness report.
import {Progress} from '../components/ui/progress';
import {Badge} from '../components/ui/badge';
import {useEffect, useState} from 'react';
import {displayName} from '../agent/profile-label.js';
import {useStore} from '../store.js';

const minute = 60_000;

// windowLabel names a window by its length: 300 minutes is 5h, 10080 is 7d.
export function windowLabel(minutes) {
  if (minutes % 1440 === 0) return `${minutes / 1440}d`;
  if (minutes % 60 === 0) return `${minutes / 60}h`;
  return `${minutes}m`;
}

// timeLeft gives the two largest units of a duration, such as 2d 4h or
// 3h 12m, rounding up to the minute.
export function timeLeft(ms) {
  const total = Math.ceil(ms / minute);
  const days = Math.floor(total / 1440);
  const hours = Math.floor(total % 1440 / 60);
  const minutes = total % 60;
  if (days) return `${days}d ${hours}h`;
  if (hours) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

function LimitWindow({limit, now}) {
  const label = windowLabel(limit.minutes);
  const left = limit.resets * 1000 - now;
  const known = limit.resets > 0;
  const passed = known && left <= 0;
  // The percent of the window elapsed: the usage that keeps exact pace.
  const pace = known && !passed ? Math.min(100, Math.max(0, 100 - left / (limit.minutes * minute) * 100)) : null;
  const ahead = pace !== null && limit.used > pace;
  const classes = ['status-bar-window flex items-center gap-2'];
  if (passed) classes.push('status-bar-window-reset opacity-60');
  if (ahead) classes.push('status-bar-window-ahead');
  let title = `${label} limit: ${limit.used}% used`;
  if (pace !== null) title += `, ${Math.round(pace)}% of the window elapsed, resets in ${timeLeft(left)}`;
  if (passed) title += ', reset';
  return (
    <span className={classes.join(' ')} title={title}>
      {label}
      <span className="status-bar-meter flex w-16 flex-col gap-1">
        <Progress value={Math.min(100, limit.used)} aria-label={`${label} used`} />
        {pace !== null && <Progress value={pace} aria-label={`${label} elapsed`} />}
      </span>
      {limit.used}%
      {ahead && <Badge variant="secondary">Ahead</Badge>}
      {pace !== null && <span className="status-bar-left flex items-center gap-1 text-muted-foreground"><Icon name="history" size="12" />{timeLeft(left)}</span>}
    </span>
  );
}

export function StatusBar() {
  const [state] = useStore();
  // The time left and the pace move with the clock.
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), minute);
    return () => clearInterval(timer);
  }, []);
  if (!state.limits.length) return null;
  const byHarness = Object.groupBy(state.limits, limit => limit.harness);
  return (
    <footer className="status-bar flex shrink-0 flex-wrap items-center justify-end gap-6 border-t px-3 py-2 text-xs">
      {Object.entries(byHarness).map(([harness, limits]) => (
        <span key={harness} className="status-bar-harness flex items-center gap-4">
          {displayName(harness)}
          {limits.map(limit => <LimitWindow key={limit.minutes} limit={limit} now={now} />)}
        </span>
      ))}
    </footer>
  );
}
