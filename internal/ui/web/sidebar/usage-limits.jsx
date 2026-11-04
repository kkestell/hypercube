// The harnesses' subscription usage, one row per harness that has reported
// it: each window's length and the percent used, such as "Claude 5h 42% · 7d
// 18%". A window's title says when it resets. Past that time the window keeps
// its last reported percent, dimmed, until the harness reports again.
import {Fragment} from 'preact';
import {displayName} from '../agent/profile-label.js';
import {useStore} from '../store.js';
import './usage-limits.css';

// windowLabel names a window by its length: 300 minutes is 5h, 10080 is 7d.
export function windowLabel(minutes) {
  if (minutes % 1440 === 0) return `${minutes / 1440}d`;
  if (minutes % 60 === 0) return `${minutes / 60}h`;
  return `${minutes}m`;
}

const resetTime = seconds => new Date(seconds * 1000).toLocaleString(undefined, {weekday: 'short', hour: 'numeric', minute: '2-digit'});

function LimitWindow({limit, now}) {
  const passed = limit.resets > 0 && limit.resets * 1000 <= now;
  const title = limit.resets > 0 ? `${passed ? 'Reset' : 'Resets'} ${resetTime(limit.resets)}` : undefined;
  return (
    <span class={passed ? 'usage-limits-window usage-limits-window-reset' : 'usage-limits-window'} title={title}>
      {windowLabel(limit.minutes)} {limit.used}%
    </span>
  );
}

export function UsageLimits() {
  const [state] = useStore();
  if (!state.limits.length) return null;
  const now = Date.now();
  const byHarness = Object.groupBy(state.limits, limit => limit.harness);
  return (
    <div class="usage-limits">
      {Object.entries(byHarness).map(([harness, limits]) => (
        <div key={harness} class="usage-limits-row">
          <span class="usage-limits-harness">{displayName(harness)}</span>
          {limits.map((limit, i) => (
            <Fragment key={limit.minutes}>
              {i > 0 && ' · '}
              <LimitWindow limit={limit} now={now} />
            </Fragment>
          ))}
        </div>
      ))}
    </div>
  );
}
