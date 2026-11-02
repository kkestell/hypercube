// VS Code's Seti file icons: a glyph of seti.woff in the theme's color. The
// theme maps a file name, then its extensions from longest to shortest, then
// its language to an icon.
import {language} from './language.js';
import theme from './seti/vs-seti-icon-theme.json';
import './file-icon.css';

export function setiIcon(name) {
  const lower = name.toLowerCase();
  let id = theme.fileNames[lower];
  for (let i = lower.indexOf('.'); !id && i >= 0; i = lower.indexOf('.', i + 1)) id = theme.fileExtensions[lower.slice(i + 1)];
  id ??= theme.languageIds[language(name)] ?? theme.file;
  const {fontCharacter, fontColor} = theme.iconDefinitions[id];
  return {glyph: String.fromCodePoint(parseInt(fontCharacter.slice(1), 16)), color: fontColor};
}

export function FileIcon({name, slot}) {
  const {glyph, color} = setiIcon(name);
  return <span slot={slot} class="file-icon" style={{color}} aria-hidden="true">{glyph}</span>;
}
