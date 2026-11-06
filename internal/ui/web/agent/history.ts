import type {Transcript} from '../transcript/types.ts';

// History is read only when navigating, so transcript updates need no extra state.
export function promptHistory(transcript: Transcript | null): string[] {
  if (!transcript) return [];
  return transcript.order.flatMap(id => {
    const item = transcript.items[id];
    return item.kind === 'user' && !item.from && !item.automatic && item.text ? [item.text] : [];
  });
}

// History keeps edits to recalled copies separate from the unsent draft.
export interface History {
  index: number;
  edits: Record<number, string>;
}

export function stepHistory(prompts: string[], history: History | null, direction: 'up' | 'down'): History | null {
  if (!prompts.length) return history;
  if (history === null && direction === 'down') return null;
  const index = history === null ? prompts.length - 1 : direction === 'up' ? Math.max(0, history.index - 1) : history.index + 1;
  return index >= prompts.length ? null : {index, edits: {...prompts, ...history?.edits}};
}
