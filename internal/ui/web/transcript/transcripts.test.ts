import {expect, test} from 'bun:test';
import {testState} from '../../test-state.ts';
import type {TranscriptMessage} from '../data-socket.ts';
import {emptySession} from '../sessions/selection.ts';
import type {State} from '../state.ts';
import {receivedTranscript, setExpanded} from './transcripts.ts';
import type {TranscriptItem} from './types.ts';

const state = () => testState({session: emptySession('s')});
const message = (items: TranscriptItem[], reset = false, session = 's'): TranscriptMessage => ({type: 'transcript', session, reset, items});
const text = (id: string, seq: number, text = ''): TranscriptItem => ({id, seq, kind: 'text', text});
const shown = (state: State) => {
  const transcript = state.session?.transcript;
  return transcript?.order.map(id => transcript.items[id]).map(item => item.kind === 'text' ? item.text : undefined);
};

test('messages for another session are ignored', () => {
  const before = state();
  expect(receivedTranscript(before, message([text('1', 1)], true, 'other'))).toBe(before);
  const none = testState();
  expect(receivedTranscript(none, message([]))).toBe(none);
});

test('a reset replaces the items and an update upserts them by ID', () => {
  let next = receivedTranscript(state(), message([text('1', 1, 'one'), text('2', 2, 'tw')], true));
  const one = next.session?.transcript.items['1'];
  next = receivedTranscript(next, message([text('2', 2, 'two'), text('3', 3, 'three')]));
  expect(shown(next)).toEqual(['one', 'two', 'three']);
  expect(next.session?.transcript.items['1']).toBe(one);
  next = receivedTranscript(next, message([text('9', 1, 'fresh')], true));
  expect(shown(next)).toEqual(['fresh']);
});

test('items are ordered by seq, even when an update arrives out of order', () => {
  let next = receivedTranscript(state(), message([text('c', 3, 'three'), text('a', 1, 'one')], true));
  next = receivedTranscript(next, message([text('b', 2, 'two')]));
  expect(shown(next)).toEqual(['one', 'two', 'three']);
});

const turn = (...items: TranscriptItem[]): TranscriptItem[] => [
  {id: 'u', seq: 1, kind: 'user', text: 'Go'},
  {id: 'L1:w', seq: 2, kind: 'state', state: 'working', done: true},
  ...items,
  {id: 'L1:i', seq: 99, kind: 'state', state: 'idle', done: true, final: ['t']},
];

test('an open request leaves activity expansion under the user\'s control', () => {
  const next = receivedTranscript(state(), message(turn(
    {id: 'a', seq: 3, kind: 'tool', tool: 'agent'},
    {id: 'q', seq: 4, parent: 'a', kind: 'attention', text: 'Allow?', choices: ['ok']},
    text('t', 5, 'Waiting'),
  ), true));
  expect(next.session?.expanded).toEqual({});
  const expanded = setExpanded(next, 'activity:L1:w', true);
  expect(expanded.session?.expanded).toEqual({'activity:L1:w': true});
  expect(receivedTranscript(expanded, message([])).session?.expanded).toEqual({'activity:L1:w': true});
  expect(setExpanded(expanded, 'activity:L1:w', false).session?.expanded).toEqual({});
});

test('an open request leaves its activity collapsed when the turn ends later', () => {
  const [opener, work, ...rest] = turn({id: 'q', seq: 3, kind: 'attention', text: 'Allow?'}, text('t', 4, 'Waiting'));
  let next = receivedTranscript(state(), message([opener, work, rest[0], rest[1]], true));
  expect(next.session?.expanded).toEqual({});
  next = receivedTranscript(next, message([rest[2]]));
  expect(next.session?.expanded).toEqual({});
});
