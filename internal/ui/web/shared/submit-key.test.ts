import {expect, test} from 'bun:test';
import {submitsOnKey} from './submit-key.ts';

test('submitsOnKey sends on Enter alone', () => {
  expect(submitsOnKey({key: 'Enter'})).toBe(true);
  expect(submitsOnKey({key: 'Enter', shiftKey: true})).toBe(false);
  expect(submitsOnKey({key: 'Enter', isComposing: true})).toBe(false);
  expect(submitsOnKey({key: 'Enter', metaKey: true})).toBe(false);
  expect(submitsOnKey({key: 'a'})).toBe(false);
});
