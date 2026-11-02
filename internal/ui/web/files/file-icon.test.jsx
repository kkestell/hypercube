import {expect, test} from 'bun:test';
import {render} from 'preact';
import theme from './seti/vs-seti-icon-theme.json';
import {FileIcon, setiIcon} from './file-icon.jsx';

const icon = id => {
  const {fontCharacter, fontColor} = theme.iconDefinitions[id];
  return {glyph: String.fromCodePoint(parseInt(fontCharacter.slice(1), 16)), color: fontColor};
};

test('Seti icons match a file name, then its extensions, then its language', () => {
  expect(setiIcon('README.md')).toEqual(icon(theme.fileNames['readme.md']));
  expect(setiIcon('main.go')).toEqual(icon(theme.languageIds.go));
  expect(setiIcon('Makefile')).toEqual(icon(theme.languageIds.makefile));
  expect(setiIcon('x.d.ts')).toEqual(icon(theme.fileExtensions['d.ts'] ?? theme.languageIds.typescript));
  expect(setiIcon('notes.unknown')).toEqual(icon(theme.file));
});

test('FileIcon draws the glyph in its color', () => {
  const root = document.createElement('div');
  render(<FileIcon name="main.go" />, root);
  const span = root.querySelector('.file-icon');
  expect(span.textContent).toBe(icon(theme.languageIds.go).glyph);
  expect(span.style.color).not.toBe('');
});
