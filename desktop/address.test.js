const {expect, test} = require('bun:test');
const {normalizeAddress} = require('./address.js');

test.each([
  ['example.com', 'https://example.com'],
  ['  localhost:8080/page ', 'https://localhost:8080/page'],
  ['http://localhost:8080/', 'http://localhost:8080/'],
  ['about:blank', 'about:blank'],
])('%p is the address %p', (typed, address) => {
  expect(normalizeAddress(typed)).toBe(address);
});

test('text that names no URL has no address', () => {
  expect(normalizeAddress('http://')).toBeNull();
  expect(normalizeAddress('exa mple.com')).toBeNull();
});
