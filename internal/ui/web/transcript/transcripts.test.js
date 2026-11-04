import {expect, test} from 'bun:test';
import {emptySession} from '../sessions/selection.js';
import {receivedTranscript, setExpanded} from './transcripts.js';

const state = () => ({session: emptySession('s')});
const message = (items, reset = false, session = 's') => ({type: 'transcript', session, reset, items});
const shown = state => state.session.transcript.order.map(id => state.session.transcript.items[id].text);

test('messages for another session are ignored', () => {
  const before = state();
  expect(receivedTranscript(before, message([{id: '1', seq: 1}], true, 'other'))).toBe(before);
  expect(receivedTranscript({session: null}, message([]))).toEqual({session: null});
});

test('a reset replaces the items and an update upserts them by ID', () => {
  let next = receivedTranscript(state(), message([{id: '1', seq: 1, text: 'one'}, {id: '2', seq: 2, text: 'tw'}], true));
  const one = next.session.transcript.items['1'];
  next = receivedTranscript(next, message([{id: '2', seq: 2, text: 'two'}, {id: '3', seq: 3, text: 'three'}]));
  expect(shown(next)).toEqual(['one', 'two', 'three']);
  expect(next.session.transcript.items['1']).toBe(one);
  next = receivedTranscript(next, message([{id: '9', seq: 1, text: 'fresh'}], true));
  expect(shown(next)).toEqual(['fresh']);
});

test('items are ordered by seq, even when an update arrives out of order', () => {
  let next = receivedTranscript(state(), message([{id: 'c', seq: 3, text: 'three'}, {id: 'a', seq: 1, text: 'one'}], true));
  next = receivedTranscript(next, message([{id: 'b', seq: 2, text: 'two'}]));
  expect(shown(next)).toEqual(['one', 'two', 'three']);
});

const turn = (...items) => [
  {id: 'u', seq: 1, kind: 'user', text: 'Go'},
  {id: 'L1:w', seq: 2, kind: 'state', state: 'working', done: true},
  ...items,
  {id: 'L1:i', seq: 99, kind: 'state', state: 'idle', done: true, final: ['t']},
];

test('an open request leaves activity expansion under the user\'s control', () => {
  const next = receivedTranscript(state(), message(turn(
    {id: 'a', seq: 3, kind: 'tool', tool: 'agent'},
    {id: 'q', seq: 4, parent: 'a', kind: 'attention', text: 'Allow?', choices: ['ok']},
    {id: 't', seq: 5, kind: 'text', text: 'Waiting'},
  ), true));
  expect(next.session.expanded).toEqual({});
  const expanded = setExpanded(next, 'activity:L1:w', true);
  expect(expanded.session.expanded).toEqual({'activity:L1:w': true});
  expect(receivedTranscript(expanded, message([])).session.expanded).toEqual({'activity:L1:w': true});
  expect(setExpanded(expanded, 'activity:L1:w', false).session.expanded).toEqual({});
});

test('an open request leaves its activity collapsed when the turn ends later', () => {
  const [opener, work, ...rest] = turn({id: 'q', seq: 3, kind: 'attention', text: 'Allow?'}, {id: 't', seq: 4, kind: 'text', text: 'Waiting'});
  let next = receivedTranscript(state(), message([opener, work, rest[0], rest[1]], true));
  expect(next.session.expanded).toEqual({});
  next = receivedTranscript(next, message([rest[2]]));
  expect(next.session.expanded).toEqual({});
});
