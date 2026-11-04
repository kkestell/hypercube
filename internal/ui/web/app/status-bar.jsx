// The window's status bar, showing the harnesses' subscription usage: for
// each harness that has reported it, each window's length, a meter of the
// percent used, and the time left until the window resets. The meter marks
// how much of the window has elapsed, the pace that would use exactly all of
// it, and turns amber when usage runs ahead of that pace. Past its reset time
// a window keeps its last reported percent, dimmed, until the harness reports
// again.
import {useEffect, useState} from 'preact/hooks';
import {displayName} from '../agent/profile-label.js';
import {useStore} from '../store.js';
import './status-bar.css';

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
  const classes = ['status-bar-window'];
  if (passed) classes.push('status-bar-window-reset');
  if (ahead) classes.push('status-bar-window-ahead');
  let title = `${label} limit: ${limit.used}% used`;
  if (pace !== null) title += `, ${Math.round(pace)}% of the window elapsed, resets in ${timeLeft(left)}`;
  if (passed) title += ', reset';
  return (
    <span class={classes.join(' ')} title={title}>
      {label}
      <span class="status-bar-meter" aria-hidden="true">
        <span class="status-bar-meter-used" style={{width: `${Math.min(100, limit.used)}%`}} />
        {pace !== null && <span class="status-bar-meter-pace" style={{left: `${pace}%`}} />}
      </span>
      {limit.used}%
      {pace !== null && <span class="status-bar-left"><vscode-icon name="history" size="12" />{timeLeft(left)}</span>}
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
    <footer class="status-bar">
      {Object.entries(byHarness).map(([harness, limits]) => (
        <span key={harness} class="status-bar-harness">
          {displayName(harness)}
          {limits.map(limit => <LimitWindow key={limit.minutes} limit={limit} now={now} />)}
        </span>
      ))}
    </footer>
  );
}
