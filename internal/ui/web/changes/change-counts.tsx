export function ChangeCounts({added, removed}: {added: number; removed: number}) {
  return <span className="flex shrink-0 gap-1 text-xs text-muted-foreground tabular-nums">
    <span>+{added}</span><span>−{removed}</span>
  </span>;
}
