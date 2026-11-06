// The session's context use as a ring; its tooltip gives the token counts and
// cost.
import type {Usage as SessionUsage} from '../sessions/types.ts';

export function tokens(n: number): string {
  if (n < 1000) return String(n);
  const [scale, unit] = n < 1e6 ? [1e3, 'k'] : [1e6, 'M'];
  return (n < 10 * scale ? Math.floor(n / scale * 10) / 10 : Math.floor(n / scale)) + unit;
}

export const cost = (dollars: number): string => dollars < 0.01 ? '<$0.01' : `$${dollars.toFixed(2)}`;

const ring = {size: 14, stroke: 2};
const radius = (ring.size - ring.stroke) / 2;
const circumference = 2 * Math.PI * radius;

function ContextRing({percent}: {percent: number}) {
  const {size, stroke} = ring;
  return <svg className="usage-ring shrink-0 -rotate-90" width={size} height={size} viewBox={`0 0 ${size} ${size}`} role="progressbar" aria-label="Context used" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(percent)}>
    <circle cx={size / 2} cy={size / 2} r={radius} fill="none" strokeWidth={stroke} className="stroke-primary/20" />
    <circle cx={size / 2} cy={size / 2} r={radius} fill="none" strokeWidth={stroke} strokeLinecap="round" className="stroke-primary"
      strokeDasharray={circumference} strokeDashoffset={circumference * (1 - percent / 100)} />
  </svg>;
}

export function Usage({usage}: {usage: SessionUsage}) {
  const {context = 0, window = 0, cost: spent = 0} = usage;
  if (!window) return null;
  const title = `${tokens(context)} / ${tokens(window)} tokens, ${Math.round(context / window * 100)}% context used${spent > 0 ? ', ' + cost(spent) : ''}`;
  return <span className="usage flex shrink-0 text-muted-foreground" title={title}>
    <ContextRing percent={Math.min(100, context / window * 100)} />
  </span>;
}
