// Prompts stay visible while HTTP waits for the agent, independently of the
// attached transcript and of whatever the user types next.
import type {Image} from '../images.ts';
import type {State} from '../state.ts';

export interface Submission {
  id: string;
  text: string;
  images: Image[];
  status: 'sending' | 'sent' | 'failed';
  error?: string;
}

export const submissionsOf = (state: State, key: string): Submission[] => state.submissions[key] ?? [];

export function withSubmissions(state: State, key: string, entries: Submission[]): State {
  const submissions = {...state.submissions};
  if (entries.length) submissions[key] = entries;
  else delete submissions[key];
  return {...state, submissions};
}

export const addedSubmission = (state: State, key: string, entry: Submission): State =>
  withSubmissions(state, key, [...submissionsOf(state, key), entry]);

// A transcript can acknowledge a submission before its HTTP request returns.
// Updating a completed request must never put the provisional row back.
export const updatedSubmission = (state: State, key: string, id: string, change: Partial<Submission>): State =>
  withSubmissions(state, key, submissionsOf(state, key).map(entry => entry.id === id ? {...entry, ...change} : entry));

export const reconciledSubmissions = (state: State, key: string, ids: Iterable<string>): State => {
  const received = new Set(ids);
  return withSubmissions(state, key, submissionsOf(state, key).filter(entry => !received.has(entry.id)));
};

export function movedSubmissions(state: State, from: string, to: string): State {
  const entries = [...submissionsOf(state, to), ...submissionsOf(state, from)];
  const next = withSubmissions(withSubmissions(state, from, []), to, entries);
  return next.session?.id === to ? reconciledSubmissions(next, to, next.session.transcript.order) : next;
}
