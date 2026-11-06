const {expect, test} = require('bun:test');
const {isBrowserURL, normalizeAddress} = require('./address.js');

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

test.each([
  'file:///tmp/untrusted.html',
  'FILE:///tmp/untrusted.html',
  'ftp://example.com/file',
  'javascript://example.com/%0aalert(1)',
  'data:text/html,<script>alert(1)</script>',
  'about:srcdoc',
])('refuses %p in the address bar and page navigation', address => {
  expect(normalizeAddress(address)).toBeNull();
  expect(isBrowserURL(address)).toBe(false);
});

test.each(['http://localhost:8080/', 'https://example.com/', 'about:blank'])(
  'allows page navigation to %p', address => expect(isBrowserURL(address)).toBe(true),
);
