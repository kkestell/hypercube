import type {Transcript} from '../transcript/types.ts';

// History is read only when navigating, so transcript updates need no extra state.
export function promptHistory(transcript: Transcript | null): string[] {
  if (!transcript) return [];
  return transcript.order.flatMap(id => {
    const item = transcript.items[id];
    return item.kind === 'user' && !item.from && item.text ? [item.text] : [];
  });
}
