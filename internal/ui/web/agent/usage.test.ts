import {expect, test} from 'bun:test';
import {cost, tokens} from './usage.tsx';

test('token counts read as 845, 84k, 1.2M', () => {
  expect([845, 1000, 8450, 84500, 999999, 1_234_567, 12_345_678].map(tokens)).toEqual(['845', '1k', '8.4k', '84k', '999k', '1.2M', '12M']);
});

test('costs below a cent read as <$0.01', () => {
  expect(cost(0.004)).toBe('<$0.01');
  expect(cost(1.239)).toBe('$1.24');
});
