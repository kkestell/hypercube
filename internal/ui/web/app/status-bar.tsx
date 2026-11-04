import {Icon} from '../shared/icon.tsx';
// Subscription usage bars mark elapsed time and indicate when usage exceeds
// that pace. Expired windows retain their last reported use until the next report.
import {Progress} from '../components/ui/progress';
import {useEffect, useState} from 'react';
import {displayName} from '../agent/profile-label.ts';
import {useStore} from '../store.ts';
import {LayoutToggles} from './layout-toggles.tsx';
import type {Limit} from './types.ts';

const minute = 60_000;

// windowLabel names a window by its length: 300 minutes is 5h, 10080 is 7d.
export function windowLabel(minutes: number) {
  if (minutes % 1440 === 0) return `${minutes / 1440}d`;
  if (minutes % 60 === 0) return `${minutes / 60}h`;
  return `${minutes}m`;
}

// timeLeft gives the two largest units of a duration, such as 2d 4h or
// 3h 12m, rounding up to the minute.
export function timeLeft(ms: number) {
  const total = Math.ceil(ms / minute);
  const days = Math.floor(total / 1440);
  const hours = Math.floor(total % 1440 / 60);
  const minutes = total % 60;
  if (days) return `${days}d ${hours}h`;
  if (hours) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

function LimitWindow({limit, now}: {limit: Limit; now: number}) {
  const label = windowLabel(limit.minutes);
  const resets = limit.resets ?? 0;
  const left = resets * 1000 - now;
  const known = resets > 0;
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
      <span className="status-bar-meter relative w-16">
        <Progress value={Math.min(100, limit.used)} aria-label={`${label} used`} />
        {pace !== null && <span className="status-bar-meter-pace absolute inset-y-0 w-0.5 bg-white mix-blend-difference" style={{left: `${pace}%`, transform: `translateX(-${pace}%)`}} role="img" aria-label={`${label} elapsed: ${Math.round(pace)}%`} />}
      </span>
      {limit.used}%
      {ahead && <span title="Usage ahead of pace"><Icon name="arrow-up" role="img" title="Usage ahead of pace" /></span>}
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
  const byHarness = Object.groupBy(state.limits, limit => limit.harness);
  return (
    <footer className="status-bar flex shrink-0 items-center gap-6 border-t py-1 pr-1 pl-3 text-xs">
      <div className="status-bar-usage flex min-w-0 flex-1 flex-wrap items-center gap-6">
        {Object.entries(byHarness).map(([harness, limits = []]) => (
          <span key={harness} className="status-bar-harness flex items-center gap-4">
            {displayName(harness)}
            {limits.map(limit => <LimitWindow key={limit.minutes} limit={limit} now={now} />)}
          </span>
        ))}
      </div>
      <LayoutToggles />
    </footer>
  );
}
