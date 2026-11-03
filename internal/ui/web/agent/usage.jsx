// An agent's context use, as a ring and tokens used of its window, and its
// cost so far.
import './usage.css';

// tokens reads as 845, 84k, 1.2M.
export function tokens(n) {
  if (n < 1000) return String(n);
  const [scale, unit] = n < 1e6 ? [1e3, 'k'] : [1e6, 'M'];
  return (n < 10 * scale ? Math.floor(n / scale * 10) / 10 : Math.floor(n / scale)) + unit;
}

export const cost = dollars => dollars < 0.01 ? '<$0.01' : `$${dollars.toFixed(2)}`;

const radius = 6, circumference = 2 * Math.PI * radius;

export function Usage({usage}) {
  const {context = 0, window = 0, cost: spent = 0} = usage ?? {};
  if (!window && !spent) return null;
  return (
    <span class="usage">
      {window > 0 && (
        <span class="usage-context">
          <svg viewBox="0 0 16 16" class="usage-ring">
            <circle cx="8" cy="8" r={radius} class="usage-track" />
            <circle cx="8" cy="8" r={radius} class="usage-fill"
              stroke-dasharray={circumference} stroke-dashoffset={circumference * (1 - Math.min(1, context / window))} />
          </svg>
          <span class="usage-tokens">{tokens(context)} / {tokens(window)}</span>
        </span>
      )}
      {spent > 0 && <span class="usage-cost">{cost(spent)}</span>}
    </span>
  );
}
