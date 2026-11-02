import {expect, test} from 'bun:test';
import {emptySession} from '../sessions/selection.js';
import {receivedTranscript, setExpanded} from './transcripts.js';

const state = () => ({session: emptySession('s')});
const message = (items, reset = false, session = 's') => ({type: 'transcript', session, agent: 'a', reset, items});
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

test('an open request expands the runs and calls around it', () => {
  const next = receivedTranscript(state(), message([
    {id: 'a', seq: 1, kind: 'tool', tool: 'agent'},
    {id: 'b', seq: 2, parent: 'a', kind: 'tool', tool: 'inner'},
    {id: 'q', seq: 3, parent: 'b', kind: 'attention', text: 'Allow?', choices: ['ok']},
  ], true));
  expect(next.session.expanded).toEqual({'run:a': true, a: true, 'run:b': true, b: true});
  expect(setExpanded(next, 'a', false).session.expanded).toEqual({'run:a': true, 'run:b': true, b: true});
});

test('an open request reveals its parent when the parent arrives later', () => {
  let next = receivedTranscript(state(), message([{id: 'q', seq: 1, parent: 'p', kind: 'attention', text: 'Allow?'}], true));
  next = receivedTranscript(next, message([{id: 'p', seq: 2, kind: 'tool', tool: 'agent'}]));
  expect(next.session.expanded).toEqual({'run:p': true, p: true});
});
