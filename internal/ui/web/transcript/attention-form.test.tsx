import {afterEach, expect, mock, spyOn, test} from 'bun:test';
import {act} from 'react';
import {element, find, findAll, mount, unmountAll} from '../../test-render.tsx';
import {testState} from '../../test-state.ts';
import {draftOf} from '../agent/drafts.ts';
import {createStore} from '../store.ts';
import {AttentionForm, QuestionsForm} from './attention-form.tsx';
import type {AttentionItem} from './types.ts';

afterEach(() => {
  mock.restore();
  unmountAll();
});

const question: AttentionItem = {id: 'q', kind: 'attention', text: 'Choose several', multi: true, choices: ['First', 'With, punctuation', 'Last'], notes: ['First note'], free: true};

test('multiple choices wait for submit, preserve punctuation, and can retry a refused answer', async () => {
  const reply = Promise.withResolvers<Response>();
  const fetches = spyOn(globalThis, 'fetch').mockReturnValue(reply.promise);
  const store = createStore(testState());
  const root = mount(store, <AttentionForm session="s" item={question} />);
  const choices = () => findAll(root, '[role="checkbox"]');
  const submit = () => find(root, '.attention-form-submit');
  expect(submit().hasAttribute('disabled')).toBe(true);
  expect(choices()).toHaveLength(4);
  expect(choices()[0].getAttribute('aria-describedby')).toBeTruthy();
  act(() => choices()[1].click());
  act(() => choices()[0].click());
  expect(fetches).not.toHaveBeenCalled();
  expect(draftOf(store.get(), 's/q').choices).toEqual(['With, punctuation', 'First']);
  await act(async () => submit().click());
  const url = new URL(String(fetches.mock.calls[0][0]), 'http://x/');
  expect(url.searchParams.getAll('choice')).toEqual(['First', 'With, punctuation']);
  expect(choices().every(choice => choice.hasAttribute('disabled'))).toBe(true);
  await act(async () => reply.resolve(new Response('try again', {status: 409})));
  expect(submit().hasAttribute('disabled')).toBe(false);
  expect(choices().map(choice => choice.getAttribute('aria-checked'))).toEqual(['true', 'true', 'false', 'false']);
  fetches.mockResolvedValue(new Response(''));
  await act(async () => submit().click());
  expect(fetches).toHaveBeenCalledTimes(2);
  expect(draftOf(store.get(), 's/q').choices).toBeUndefined();
});

test('question selections have one draft across transcript and notification forms', () => {
  const store = createStore(testState());
  const first = mount(store, <AttentionForm session="s" item={question} />);
  act(() => find(first, '[role="checkbox"]').click());
  const second = mount(store, <AttentionForm session="s" item={question} />);
  expect(find(second, '[role="checkbox"]').getAttribute('aria-checked')).toBe('true');
  act(() => find(second, '[role="checkbox"]').click());
  expect(find(first, '[role="checkbox"]').getAttribute('aria-checked')).toBe('false');
});

test('ended multi-choice questions keep all controls disabled', () => {
  const store = createStore(testState());
  const root = mount(store, <AttentionForm session="s" item={question} ended />);
  expect(findAll(root, 'button').every(button => button.hasAttribute('disabled'))).toBe(true);
  expect(element(root, 'input:not([type="checkbox"])', HTMLInputElement).disabled).toBe(true);
});

test('questions asked together show one at a time and submit together once all are answered', async () => {
  const fetches = spyOn(globalThis, 'fetch');
  fetches.mockImplementation(((() => Promise.resolve(new Response(''))) as unknown) as typeof fetch);
  const store = createStore(testState());
  const items: AttentionItem[] = [
    {id: 'a', kind: 'attention', text: 'Which color?', choices: ['Red', 'Blue'], free: true},
    {id: 'b', kind: 'attention', text: 'Which size?', choices: ['Small', 'Large'], free: true},
  ];
  const root = mount(store, <QuestionsForm session="s" items={items} />);
  const question = () => find(root, '.attention-item-question').textContent;
  expect(question()).toBe('Which color?');
  expect(find(root, '.questions-form-progress').textContent).toBe('Question 1 of 2');
  expect(find(root, '.questions-form-next').hasAttribute('disabled')).toBe(true);
  expect(root.querySelector('.attention-form-submit')).toBeNull();
  act(() => find(root, '[data-choice="Blue"]').click());
  act(() => find(root, '.questions-form-next').click());
  expect(question()).toBe('Which size?');
  expect(find(root, '.attention-form-submit').hasAttribute('disabled')).toBe(true);
  act(() => find(root, '.questions-form-back').click());
  expect(find(root, '[data-choice="Blue"]').getAttribute('aria-checked')).toBe('true');
  act(() => find(root, '.questions-form-next').click());
  act(() => find(root, '[data-choice="Large"]').click());
  await act(async () => find(root, '.attention-form-submit').click());
  const answers = fetches.mock.calls.map(([url]) => new URL(String(url), 'http://x/').searchParams);
  expect(answers.map(params => [params.get('request'), params.get('choice')])).toEqual([['a', 'Blue'], ['b', 'Large']]);
  expect(draftOf(store.get(), 's/a').choices).toBeUndefined();
  expect(draftOf(store.get(), 's/b').choices).toBeUndefined();
});
