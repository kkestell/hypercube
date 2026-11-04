// The usage readout at the foot of the sidebar: each harness's newest rate
// limit readings, as a bar of the limit used with a marker for how much of the
// window has passed. The shortest window shows first and the longest on hover.
import {harnessIcon} from '../agent/harness-icon.tsx';
import type {Limit} from '../app/types.ts';
import {Progress} from '../components/ui/progress';
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

const bar = (used: number) => used >= 90 ? '[&_[data-slot=progress-indicator]]:bg-red-500'
  : used >= 75 ? '[&_[data-slot=progress-indicator]]:bg-amber-500' : '[&_[data-slot=progress-indicator]]:bg-neutral-500';

function LimitRow({harness, limit, className}: {harness: string; limit: Limit; className: string}) {
  const label = windowLabel(limit.minutes);
  const left = limit.resets ? Math.max(0, Math.ceil((limit.resets * 1000 - Date.now()) / 60000)) : null;
  const elapsed = left === null ? null : Math.min(100, Math.max(0, 100 - left / limit.minutes * 100));
  return <div className={`min-w-0 flex-1 items-center gap-1 ${className}`}>
    <span className="w-5 shrink-0">{label}</span>
    <span className="relative min-w-0 flex-1">
      <Progress value={limit.used} aria-label={`${harness} ${label} usage`} className={`bg-neutral-500/20 ${bar(limit.used)}`} />
      {elapsed !== null && <span className="absolute inset-y-0 w-0.5 bg-neutral-200" style={{left: `${elapsed}%`}} />}
    </span>
    <span className="w-8 shrink-0 text-right tabular-nums">{limit.used}%</span>
    <span className="w-14 shrink-0 text-right tabular-nums text-muted-foreground @max-[220px]:hidden">{left === null ? '' : timeLeft(left)}</span>
  </div>;
}

export function Usage() {
  const [{limits}] = useStore();
  const now = Date.now() / 1000;
  const live = limits.filter(l => !l.resets || l.resets > now);
  const harnesses = [...new Set(live.map(l => l.harness))];
  if (harnesses.length === 0) return null;
  return <section aria-label="Usage" tabIndex={0} className="group/usage @container shrink-0 space-y-2 border-t p-4 text-xs">
    {harnesses.map(harness => {
      const own = live.filter(l => l.harness === harness).sort((a, b) => a.minutes - b.minutes);
      const first = own[0], last = own[own.length - 1];
      return <div key={harness} className="flex items-center gap-2">
        {harnessIcon(harness)}
        {first === last
          ? <LimitRow harness={harness} limit={first} className="flex" />
          : <>
            <LimitRow harness={harness} limit={first} className="flex group-hover/usage:hidden group-focus/usage:hidden" />
            <LimitRow harness={harness} limit={last} className="hidden group-hover/usage:flex group-focus/usage:flex" />
          </>}
      </div>;
    })}
  </section>;
}
