import {expect, test} from 'bun:test';
import {formatDuration} from './elapsed.jsx';

test.each([
  [0, '0s'],
  [5999, '5s'],
  [59000, '59s'],
  [60000, '1m 0s'],
  [152000, '2m 32s'],
  [3599000, '59m 59s'],
  [3840000, '1h 4m'],
  [-1000, '0s'],
])('%d milliseconds is %s', (ms, text) => {
  expect(formatDuration(ms)).toBe(text);
});
