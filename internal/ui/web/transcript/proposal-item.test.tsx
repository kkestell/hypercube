import {afterEach, expect, test} from 'bun:test';
import {act} from 'react';
import {find, mount, unmountAll} from '../../test-render.tsx';
import {testSession, testState} from '../../test-state.ts';
import {emptySession} from '../sessions/selection.ts';
import {createStore} from '../store.ts';
import {Transcript} from './transcript.tsx';
import {receivedTranscript} from './transcripts.ts';
import type {TranscriptItem} from './types.ts';

afterEach(unmountAll);

test.each(['Started', 'Dismissed'])('a %s child proposal collapses and keeps its brief available', answer => {
  const brief = 'A detailed brief to investigate the parser.';
  const item: TranscriptItem = {
    id: 'p', seq: 1, kind: 'proposal', text: 'Investigate parser',
    input: {name: 'parser', title: 'Parser work', settings: {harness: 'claude', model: 'opus', effort: 'high'}, message: brief},
  };
  const store = createStore(receivedTranscript(testState({session: emptySession('s')}), {type: 'transcript', session: 's', reset: true, items: [item]}));
  const root = mount(store, <Transcript session={testSession({id: 's'})} />);
  expect(root.textContent).toContain(brief);
  expect(root.textContent).toContain('Start');
  act(() => store.update(state => receivedTranscript(state, {type: 'transcript', session: 's', reset: false, items: [{...item, done: true, answer}]})));
  const trigger = find(root, '.proposal-item [data-slot=collapsible-trigger]');
  expect(trigger.textContent).toContain(answer === 'Dismissed' ? 'Dismissed' : 'Child session: parser');
  expect(trigger.textContent).toContain('opushigh');
  expect(find(trigger, '[title]').title).toBe('Claude opus, effort high');
  expect(root.textContent).not.toContain(brief);
  act(() => trigger.click());
  expect(root.textContent).toContain(brief);
  act(() => trigger.click());
  expect(root.textContent).not.toContain(brief);
});
