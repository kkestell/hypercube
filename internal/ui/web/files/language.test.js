import {expect, test} from 'bun:test';
import {language} from './language.js';

test('file names map to VS Code language IDs', () => {
  expect(language('main.go')).toBe('go');
  expect(language('Dockerfile')).toBe('dockerfile');
  expect(language('Dockerfile.dev')).toBe('dockerfile');
  expect(language('tsconfig.json')).toBe('jsonc');
  expect(language('App.TSX')).toBe('typescriptreact');
  expect(language('notes.unknown')).toBe('');
  expect(language('.hidden')).toBe('');
});
