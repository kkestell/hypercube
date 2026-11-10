// The composer's subscription usage: the newest rate limit readings of the
// session's harness, as a bar of the limit used with a marker for how much of
// the window has passed. The shortest window shows first and the longest on
// hover or focus. Both readings fill the same grid row, the hidden one
// invisible, so its columns fit the wider of the two and nothing moves when
// they switch. Harnesses that report no limits show nothing.
import {useEffect, useState} from 'react';
import type {Limit} from '../app/types.ts';
import {Progress} from '../components/ui/progress';
import {tones} from '../shared/tones.ts';
import {useStore} from '../store.ts';

// windowLabel names a window by its length.
function windowLabel(minutes: number) {
  return minutes >= 1440 ? `${Math.round(minutes / 1440)}d` : `${Math.round(minutes / 60)}h`;
}

// timeLeft formats minutes as days and hours, hours and minutes, or minutes.
function timeLeft(minutes: number) {
  const hours = Math.floor(minutes / 60);
  if (minutes >= 1440) return `${Math.floor(hours / 24)}d ${hours % 24}h`;
  return hours > 0 ? `${hours}h ${minutes % 60}m` : `${minutes}m`;
}

function LimitRow({harness, limit, className = ''}: {harness: string; limit: Limit; className?: string}) {
  const [now, setNow] = useState(Date.now);
  // Tick when the displayed minute changes, including the reset itself.
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined;
    const tick = () => {
      const at = Date.now(), remaining = (limit.resets ?? 0) * 1000 - at;
      setNow(at);
      if (remaining > 0) timer = setTimeout(tick, remaining % 60000 || 60000);
    };
    tick();
    return () => clearTimeout(timer);
  }, [limit.resets]);
  const label = windowLabel(limit.minutes);
  const left = limit.resets ? Math.max(0, Math.ceil((limit.resets * 1000 - now) / 60000)) : null;
  const expired = left === 0;
  const elapsed = left === null ? null : Math.min(100, Math.max(0, 100 - left / limit.minutes * 100));
  const waiting = `${limit.used}% last reported; this window reset. Waiting for a new reading from ${harness}.`;
  const cell = `row-start-1 ${expired ? 'opacity-50' : ''} ${className}`;
  const title = expired ? waiting : undefined;
  return <>
    <span className={`col-start-1 ${cell}`} title={title}>{label}</span>
    <span className={`relative col-start-2 w-16 ${cell}`} title={title}>
      <Progress value={limit.used} aria-label={`${harness} ${label} usage`} aria-valuetext={expired ? waiting : undefined} />
      {elapsed !== null && <span className="absolute inset-y-0 w-0.5 bg-muted-foreground" style={{left: `${elapsed}%`}} />}
    </span>
    <span className={`col-start-3 justify-self-end tabular-nums ${limit.used >= 75 ? tones.warning : ''} ${cell}`} title={title}>{limit.used}%</span>
    <span className={`col-start-4 hidden justify-self-end tabular-nums @md/composer:block ${cell}`} title={title}>{expired ? 'Waiting' : left === null ? '' : timeLeft(left)}</span>
  </>;
}

export function Limits({harness}: {harness: string}) {
  const [{limits}] = useStore();
  const own = limits.filter(l => l.harness === harness).sort((a, b) => a.minutes - b.minutes);
  if (own.length === 0) return null;
  const first = own[0], last = own[own.length - 1];
  return <span aria-label="Subscription usage" tabIndex={0} className="group/limits grid shrink-0 items-center gap-x-1.5 whitespace-nowrap text-xs text-muted-foreground">
    {first === last
      ? <LimitRow harness={harness} limit={first} />
      : <>
        <LimitRow harness={harness} limit={first} className="transition-[opacity,visibility] group-hover/limits:invisible group-hover/limits:opacity-0 group-focus/limits:invisible group-focus/limits:opacity-0" />
        <LimitRow harness={harness} limit={last} className="invisible opacity-0 transition-[opacity,visibility] group-hover/limits:visible group-hover/limits:opacity-100 group-focus/limits:visible group-focus/limits:opacity-100" />
      </>}
  </span>;
}
