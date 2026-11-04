// History is read only when navigating, so transcript updates need no extra state.
export function promptHistory(transcript) {
  if (!transcript) return [];
  return transcript.order.map(id => transcript.items[id]).filter(item => item.kind === 'user' && !item.from && item.text).map(item => item.text);
}
