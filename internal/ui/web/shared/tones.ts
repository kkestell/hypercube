// The colors that carry meaning, as text colors. Status dots and bars take
// them with bg-current or border-current.
export const tones = {
  error: 'text-destructive',
  // Work is waiting: on the user, or uncommitted on a branch.
  warning: 'text-amber-600 dark:text-amber-500',
  // Something new to read, or people waiting on an answer.
  info: 'text-sky-600 dark:text-sky-500',
  // Ready to merge, running, or added.
  success: 'text-green-600 dark:text-green-500',
  muted: 'text-muted-foreground',
} as const;

export type Tone = keyof typeof tones;
