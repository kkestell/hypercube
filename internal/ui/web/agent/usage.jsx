// Context and cost use standard progress and text presentation.
import {Progress} from '../components/ui/progress';

export function tokens(n) {
  if (n < 1000) return String(n);
  const [scale, unit] = n < 1e6 ? [1e3, 'k'] : [1e6, 'M'];
  return (n < 10 * scale ? Math.floor(n / scale * 10) / 10 : Math.floor(n / scale)) + unit;
}

export const cost = dollars => dollars < 0.01 ? '<$0.01' : `$${dollars.toFixed(2)}`;

export function Usage({usage}) {
  const {context = 0, window = 0, cost: spent = 0} = usage ?? {};
  if (!window && !spent) return null;
  return <span className="usage flex items-center gap-2 text-xs text-muted-foreground">
    {window > 0 && <span className="usage-context flex items-center gap-2" title={`${Math.round(context / window * 100)}% context used`}>
      <Progress className="w-16" value={Math.min(100, context / window * 100)} aria-label="Context used" />
      <span className="usage-tokens">{tokens(context)} / {tokens(window)}</span>
    </span>}
    {spent > 0 && <span className="usage-cost">{cost(spent)}</span>}
  </span>;
}
